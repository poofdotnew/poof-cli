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
  poof verify -p <id> -m "Run only the existing tests, do not generate new ones"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		message, _ := cmd.Flags().GetString("message")
		skipProbe, _ := cmd.Flags().GetBool("skip-url-probe")
		if strings.TrimSpace(message) == "" {
			message = defaultVerifyPrompt
		}

		ctx := context.Background()

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
			return poll.Poll(ctx, poll.DefaultConfig(), func(ctx context.Context) (bool, error) {
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

		// Collapse multiple runs of the same test in this verify pass to its
		// most recent result. The server returns results sorted by startedAt
		// desc, so the first occurrence per (source, fileName, testName) wins.
		// This way an early failure that the AI fixed and re-ran doesn't get
		// double-counted as still failing.
		latestFresh := make([]api.TestResult, 0, len(fresh))
		seenKey := make(map[string]struct{}, len(fresh))
		for _, r := range fresh {
			key := r.Source + "|" + r.FileName + "|" + r.TestName
			if _, dup := seenKey[key]; dup {
				continue
			}
			seenKey[key] = struct{}{}
			latestFresh = append(latestFresh, r)
		}
		fresh = latestFresh

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

func init() {
	verifyCmd.Flags().StringP("message", "m", "", "Override the canonical verification prompt")
	verifyCmd.Flags().Bool("skip-url-probe", false, "Skip the draft URL HEAD probe")
}
