package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/poll"
	"github.com/spf13/cobra"
)

const defaultVerifyPrompt = `Please verify everything you just built end-to-end. Do all of the following in one turn:

1. Generate lifecycle action tests under lifecycle-actions/test-*.json that exercise every policy you created. Cover both success and failure cases — verify that authorized callers can perform operations and unauthorized callers are denied. Run them and report results.

2. Generate UI functional tests under lifecycle-actions/ui-test-*.json that exercise the real frontend pages. Cover form submissions, navigation, CRUD operations, and any onchain interactions. Fund the mock test user if the app has onchain features. Run them with the browser test runner against the draft app.

3. Fix any failures you uncover and re-run the relevant tests until they pass. Do not skip failing tests.

When everything passes, end your turn with a short summary of what you tested and the final pass counts.`

// backendOnlyVerifyPrompt is used when the project's generationMode excludes a
// Poof-generated UI (e.g. "policy" or "backend,policy"). The draft URL for these
// projects serves a placeholder shell until a local static frontend is uploaded
// via `poof deploy static`, so asking the AI to "run UI functional tests against
// the draft app" produces vacuous passes against the placeholder. This prompt
// sticks to lifecycle/policy tests only.
const backendOnlyVerifyPrompt = `Please verify the backend you just built end-to-end. This project is backend-only (generationMode excludes ui), so do NOT generate or run any UI functional tests — there is no Poof-generated frontend to test against. Do the following in one turn:

1. Generate lifecycle action tests under lifecycle-actions/test-*.json that exercise every policy you created. Cover both success and failure cases — verify that authorized callers can perform operations and unauthorized callers are denied. Run them and report results.

2. Fix any failures you uncover and re-run the tests until they pass. Do not skip failing tests.

3. Do NOT create any lifecycle-actions/ui-test-*.json files and do NOT run the browser UI test runner. This project has no Poof-generated UI. If the user has deployed a local static frontend via 'poof deploy static' and wants UI tests, they will ask explicitly.

When everything passes, end your turn with a short summary of what policies you tested and the final pass counts.`

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Run the canonical post-build verification flow",
	Long: `Send the canonical lifecycle + UI verification prompt, wait for the AI to finish, and
verify that fresh test results were produced and all of them passed.

Unlike 'poof iterate', this command is strict about evidence:
- It snapshots existing test result IDs before sending the prompt.
- After the AI finishes it only counts results created during this run.
- Exits non-zero if no fresh results appear or if any fresh result failed.
- Optionally probes the draft URL to confirm the app is reachable.`,
	Example: `  poof verify -p <id>
  poof verify -p <id> --json
  poof verify -p <id> --skip-url-probe
  poof verify -p <id> --ui-tests=false      # force lifecycle-only prompt
  poof verify -p <id> --ui-tests=true       # force full prompt even for backend-only projects
  poof verify -p <id> -m "Run only the existing tests, do not generate new ones"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		messageOverride, _ := cmd.Flags().GetString("message")
		skipProbe, _ := cmd.Flags().GetBool("skip-url-probe")
		uiTestsFlag, _ := cmd.Flags().GetString("ui-tests")

		ctx := context.Background()

		// Decide which prompt to send based on generationMode and the --ui-tests
		// flag. An explicit --message always wins.
		var message string
		var uiTestsEnabled bool
		switch strings.ToLower(strings.TrimSpace(uiTestsFlag)) {
		case "true", "yes", "on", "1":
			message = defaultVerifyPrompt
			uiTestsEnabled = true
		case "false", "no", "off", "0":
			message = backendOnlyVerifyPrompt
			uiTestsEnabled = false
		default: // auto / empty
			// Two independent signals can route us to the lifecycle-only prompt:
			//   1. generationMode excludes `ui` (backend,policy or policy) —
			//      the project was created as backend-only, so Poof never had
			//      the UI source and any UI test prompt would be vacuous.
			//   2. The project has a prior static_deploy task — the agent
			//      uploaded a local dist, so Poof's AI only sees the minified
			//      bundle. UI tests against it are unreliable regardless of
			//      generationMode.
			// Either signal is sufficient. We fall back to the default
			// (UI-tests-enabled) prompt only when BOTH signals say "UI is
			// Poof-owned and in-source."
			genMode, gmErr := fetchGenerationMode(ctx, projectID)
			hasStaticDeploy, sdErr := projectHasStaticDeployTask(ctx, projectID)

			switch {
			case gmErr == nil && generationModeExcludesUI(genMode):
				message = backendOnlyVerifyPrompt
				uiTestsEnabled = false
				if output.GetFormat() == output.FormatText {
					output.Info("Detected generationMode=%q — running lifecycle tests only. Use --ui-tests=true to force UI tests.", genMode)
				}
			case sdErr == nil && hasStaticDeploy:
				message = backendOnlyVerifyPrompt
				uiTestsEnabled = false
				if output.GetFormat() == output.FormatText {
					output.Info("Detected a prior static_deploy task — the draft URL is serving your locally-built dist, which Poof's AI cannot read. Running lifecycle tests only. Use --ui-tests=true to force UI tests (not recommended for static deploys).")
				}
			default:
				// Both signals absent or errored. Use the full prompt; worst
				// case the user is on an older server without generationMode
				// in the schema and gets redundant UI test generation.
				message = defaultVerifyPrompt
				uiTestsEnabled = true
			}
		}
		if strings.TrimSpace(messageOverride) != "" {
			message = messageOverride
		}
		_ = uiTestsEnabled // reserved for future use in JSON output

		// 1. Snapshot existing test result IDs so we can detect what is fresh.
		baselineIDs := map[string]struct{}{}
		baseline, err := apiClient.GetTestResults(ctx, projectID, 100, 0)
		if err != nil {
			if apiErr, ok := api.IsAPIError(err); !ok || !apiErr.IsNotFound() {
				return handleError(err)
			}
		} else {
			for _, r := range baseline.Results {
				if r.ID != "" {
					baselineIDs[r.ID] = struct{}{}
				}
			}
		}

		if output.GetFormat() == output.FormatText {
			output.Info("Sending verification prompt...")
		}

		// 2. Send the prompt as a chat message.
		if _, err := apiClient.Chat(ctx, projectID, message, nil); err != nil {
			return handleError(err)
		}

		// 3. Poll until AI is done. Mirror build/iterate semantics with a grace
		// period so we never call it done before the server activates the AI.
		seenActive := false
		pollStart := time.Now()
		const activationGrace = 30 * time.Second

		err = output.WithSpinner("AI is verifying...", func() error {
			return poll.Poll(ctx, poll.LongAIConfig(), func(ctx context.Context) (bool, error) {
				status, err := apiClient.CheckAIActive(ctx, projectID)
				if err != nil {
					return false, err
				}
				if status.Status == "error" {
					return false, fmt.Errorf("AI processing failed with error status")
				}
				if status.Active {
					seenActive = true
					return false, nil
				}
				if seenActive || time.Since(pollStart) > activationGrace {
					return true, nil
				}
				return false, nil
			})
		})
		if err != nil {
			return fmt.Errorf("verify timed out or failed: %w", err)
		}

		// 4. Re-fetch test results and filter to fresh ones.
		results, err := apiClient.GetTestResults(ctx, projectID, 100, 0)
		if err != nil {
			if apiErr, ok := api.IsAPIError(err); ok && apiErr.IsNotFound() {
				results = &api.TestResultsResponse{}
			} else {
				return handleError(err)
			}
		}

		fresh := make([]api.TestResult, 0, len(results.Results))
		for _, r := range results.Results {
			if r.ID == "" {
				continue
			}
			if _, seen := baselineIDs[r.ID]; seen {
				continue
			}
			fresh = append(fresh, r)
		}

		// Collapse multiple runs of the same test file in this verify pass
		// to its most recent result. An early failure that the AI fixed
		// and re-ran shouldn't get double-counted as still failing.
		fresh = collapseResultsToLatest(fresh)

		freshSummary := summarizeResults(fresh)

		// 5. Optional draft URL smoke probe.
		probe := map[string]interface{}{}
		if !skipProbe {
			status, statusErr := apiClient.GetProjectStatus(ctx, projectID)
			if statusErr == nil {
				if draft, ok := status.URLs["draft"]; ok && draft != "" {
					code, perr := probeDraftURL(ctx, draft)
					probe["url"] = draft
					if perr != nil {
						probe["error"] = perr.Error()
					} else {
						probe["statusCode"] = code
						probe["reachable"] = code >= 200 && code < 400
					}
				}
				if status.PublishState != nil {
					probe["draftDeployedFlag"] = status.IsTargetDeployed("draft")
				}
			}
		}

		passed := freshSummary.Total > 0 && freshSummary.Failed == 0 && freshSummary.Errors == 0

		type verifyResult struct {
			ProjectID    string                 `json:"projectId"`
			Passed       bool                   `json:"passed"`
			FreshSummary api.TestSummary        `json:"freshSummary"`
			FreshResults []api.TestResult       `json:"freshResults"`
			BaselineSize int                    `json:"baselineSize"`
			Probe        map[string]interface{} `json:"probe,omitempty"`
		}
		result := &verifyResult{
			ProjectID:    projectID,
			Passed:       passed,
			FreshSummary: freshSummary,
			FreshResults: fresh,
			BaselineSize: len(baselineIDs),
			Probe:        probe,
		}

		output.Print(result, func() {
			if freshSummary.Total == 0 {
				output.Warn("Verify finished, but no fresh test results were produced.")
				output.Info("Treat this as missing test artifacts, not a passing run.")
				output.Info("Inspect: poof task list -p %s --json", projectID)
				output.Info("Inspect: poof chat active -p %s --json", projectID)
				output.Info("Inspect: poof project messages -p %s --limit 100 --json", projectID)
			} else if !passed {
				output.Warn("Verify finished with failures.")
				output.Info("Fresh tests: %d passed, %d failed, %d errors (of %d)",
					freshSummary.Passed, freshSummary.Failed, freshSummary.Errors, freshSummary.Total)
				for _, r := range fresh {
					if r.Status == "failed" || r.Status == "error" {
						if r.Source != "" {
							output.Error("  [%s] %s: %s", r.Source, r.FileName, r.LastError)
						} else {
							output.Error("  %s: %s", r.FileName, r.LastError)
						}
					}
				}
			} else {
				output.Success("Verify passed.")
				output.Info("Fresh tests: %d passed (of %d)", freshSummary.Passed, freshSummary.Total)
			}

			if len(probe) > 0 {
				if reachable, ok := probe["reachable"].(bool); ok {
					if reachable {
						output.Info("Draft probe: %v (HTTP %v)", probe["url"], probe["statusCode"])
					} else {
						output.Warn("Draft probe: %v (HTTP %v)", probe["url"], probe["statusCode"])
					}
				} else if errMsg, ok := probe["error"].(string); ok {
					output.Warn("Draft probe failed: %s", errMsg)
				}
			}
		})

		if !passed {
			return fmt.Errorf("verify failed: %d fresh results, %d failed, %d errors",
				freshSummary.Total, freshSummary.Failed, freshSummary.Errors)
		}
		return nil
	},
}

// summarizeResults computes counts identical to the server's test summary, but
// only over the slice of results we pass in (so we can summarize "fresh only").
// The server's status enum is: completed (= passed), failed, error, running.
func summarizeResults(rs []api.TestResult) api.TestSummary {
	var s api.TestSummary
	s.Total = len(rs)
	for i := range rs {
		switch strings.ToLower(rs[i].Status) {
		case "completed", "passed", "pass", "success":
			s.Passed++
		case "failed", "fail":
			s.Failed++
		case "error", "errored":
			s.Errors++
		case "running", "in_progress", "queued":
			s.Running++
		}
	}
	return s
}

// probeDraftURL issues a HEAD request against the draft URL with a short
// timeout. It returns the HTTP status code or an error if the request itself
// failed (timeout, DNS, connection refused, etc.).
func probeDraftURL(ctx context.Context, url string) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// fetchGenerationMode reads the project's generationMode via the status API.
// Returns "" if the field is missing (older server) — callers fall back to the
// full prompt in that case.
func fetchGenerationMode(ctx context.Context, projectID string) (string, error) {
	status, err := apiClient.GetProjectStatus(ctx, projectID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(status.Project.GenerationMode), nil
}

// generationModeExcludesUI returns true when the project was built without a
// Poof-generated UI — i.e. the draft URL is a placeholder shell until the user
// deploys their own static frontend. The server accepts values like "full",
// "policy", "ui,policy", "backend,policy". "full" and anything containing "ui"
// includes a Poof-generated UI.
func generationModeExcludesUI(mode string) bool {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" || m == "full" {
		return false
	}
	for _, part := range strings.Split(m, ",") {
		if strings.TrimSpace(part) == "ui" {
			return false
		}
	}
	return true
}

// projectHasStaticDeployTask scans the most recent 50 tasks for any with
// focusArea == "static_deploy". When present, the draft URL is serving a
// locally-built dist that Poof's AI can't read — so asking it to generate UI
// tests produces vacuous DOM-shape assertions. This is a belt-and-suspenders
// check that works even when generationMode is missing from the project record
// (older servers / schemas that silently drop the attribute).
func projectHasStaticDeployTask(ctx context.Context, projectID string) (bool, error) {
	resp, err := apiClient.ListTasks(ctx, projectID, "", 50, 0)
	if err != nil {
		return false, err
	}
	for _, t := range resp.Tasks {
		if fa, ok := t["focusArea"].(string); ok && fa == "static_deploy" {
			return true, nil
		}
		// focusAreas is a JSON-stringified array on the server — treat the
		// substring "static_deploy" as a match to avoid double JSON parsing.
		if fas, ok := t["focusAreas"].(string); ok && strings.Contains(fas, "static_deploy") {
			return true, nil
		}
	}
	return false, nil
}

func init() {
	verifyCmd.Flags().StringP("message", "m", "", "Override the canonical verification prompt")
	verifyCmd.Flags().Bool("skip-url-probe", false, "Skip the draft URL HEAD probe")
	verifyCmd.Flags().String("ui-tests", "auto", "Include browser UI functional tests: auto (infer from generationMode) | true | false")
}
