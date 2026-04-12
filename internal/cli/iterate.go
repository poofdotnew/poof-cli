package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var iterateCmd = &cobra.Command{
	Use:   "iterate",
	Short: "Send a chat message, wait for AI, and show test results",
	Long:  "Composite command: chat + poll until done + get_test_results",
	Example: `  poof iterate -p <id> -m "Add a leaderboard page"
  poof iterate -p <id> -m "Generate and run lifecycle tests"
  echo "Add dark mode" | poof iterate -p <id> --stdin
  poof iterate -p <id> -m "Fix login" --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		filePaths, _ := cmd.Flags().GetStringSlice("file")

		if useStdin {
			message = readStdin()
		}
		if message == "" && len(filePaths) == 0 {
			return fmt.Errorf("--message or --file is required\n  poof iterate -p %s -m \"Add a feature\"\n  poof iterate -p %s --file screenshot.png -m \"Match this design\"", projectID, projectID)
		}

		ctx := context.Background()

		var attachedFiles []string
		if len(filePaths) > 0 {
			suffix, urls, err := uploadAndPrepareFiles(ctx, projectID, filePaths)
			if err != nil {
				return err
			}
			message += suffix
			attachedFiles = urls
		}

		// 0. Snapshot existing test result IDs so we can tell agents whether
		// *new* tests actually ran during this iterate turn. Without this,
		// iterate would report "all tests passed" from stale history even
		// when the AI never touched the test suite.
		baselineIDs := map[string]struct{}{}
		if baseline, bErr := apiClient.GetTestResults(ctx, projectID, 100, 0); bErr == nil {
			for _, r := range baseline.Results {
				if r.ID != "" {
					baselineIDs[r.ID] = struct{}{}
				}
			}
		} else if apiErr, ok := api.IsAPIError(bErr); !ok || !apiErr.IsNotFound() {
			return handleError(bErr)
		}

		// 1. Send chat message
		_, err = apiClient.Chat(ctx, projectID, message, attachedFiles)
		if err != nil {
			return handleError(err)
		}
		if output.GetFormat() == output.FormatText {
			output.Info("Message sent. AI is building...")
		}

		// 2. Poll until AI finishes (auto-cancels on timeout so ship isn't blocked)
		err = pollAIUntilIdle(ctx, projectID, "AI is working...")
		if err != nil {
			return fmt.Errorf("timed out or failed: %w", err)
		}

		// 3. Check test results (may not exist for all projects). The server
		// returns raw history — including older failed runs that the AI
		// already fixed — so collapse to the most recent result per test
		// file before summarizing. Prevents iterate from reporting stale
		// failures the AI has already corrected.
		results, err := apiClient.GetTestResults(ctx, projectID, 100, 0)
		if err != nil {
			if apiErr, ok := api.IsAPIError(err); ok && apiErr.IsNotFound() {
				output.Print(map[string]interface{}{
					"success": true,
					"results": []interface{}{},
					"summary": map[string]int{"total": 0, "passed": 0, "failed": 0, "errors": 0, "running": 0},
				}, func() {
					printNoTestResultsGuidance(projectID)
				})
				return nil
			}
			return handleError(err)
		}

		// Split into fresh (created during this iterate turn) vs latest-per-file
		// views. Agents care about whether tests *actually ran*, so surface both.
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
		fresh = collapseResultsToLatest(fresh)
		freshSummary := summarizeResults(fresh)

		latest := collapseResultsToLatest(results.Results)
		latestSummary := summarizeResults(latest)

		type iterateResult struct {
			Results      []api.TestResult `json:"results"`
			Summary      api.TestSummary  `json:"summary"`
			FreshResults []api.TestResult `json:"freshResults"`
			FreshSummary api.TestSummary  `json:"freshSummary"`
			HasMore      bool             `json:"hasMore"`
		}
		viewResults := &iterateResult{
			Results:      latest,
			Summary:      latestSummary,
			FreshResults: fresh,
			FreshSummary: freshSummary,
			HasMore:      results.HasMore,
		}

		output.Print(viewResults, func() {
			if freshSummary.Total == 0 {
				if latestSummary.Total == 0 {
					printNoTestResultsGuidance(projectID)
					return
				}
				// Previous tests exist but none re-ran this turn. Don't
				// claim "passed" — be explicit that this iterate didn't
				// execute the suite.
				output.Warn("Done, but no tests ran during this turn.")
				output.Info("Existing suite (latest per file): %d passed, %d failed, %d errors (of %d)",
					latestSummary.Passed, latestSummary.Failed,
					latestSummary.Errors, latestSummary.Total)
				output.Info("If tests should have run, use: poof verify -p %s", projectID)
				return
			}
			if freshSummary.Failed > 0 || freshSummary.Errors > 0 {
				output.Warn("Done with test failures.")
				output.Info("Fresh this turn: %d passed, %d failed, %d errors (of %d)",
					freshSummary.Passed, freshSummary.Failed,
					freshSummary.Errors, freshSummary.Total)
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
				output.Success("Done! All fresh tests passed.")
				output.Info("Fresh this turn: %d passed (of %d)",
					freshSummary.Passed, freshSummary.Total)
			}
		})
		return nil
	},
}

func printNoTestResultsGuidance(projectID string) {
	output.Warn("Done, but no test results were found.")
	output.Info("If you sent a non-test prompt, this is expected — iterate is a general chat command.")
	output.Info("For strict pass/fail test verification, use: poof verify -p %s", projectID)
	output.Info("To inspect run state: poof doctor -p %s", projectID)
	output.Info("If you expected tests to run, also check:")
	output.Info("  poof task list -p %s --json", projectID)
	output.Info("  poof chat active -p %s --json", projectID)
	output.Info("  poof project messages -p %s --limit 100 --json", projectID)
}

func init() {
	iterateCmd.Flags().StringP("message", "m", "", "Message to send (required)")
	iterateCmd.Flags().Bool("stdin", false, "Read message from stdin")
	iterateCmd.Flags().StringSlice("file", nil, "Image file(s) to attach (PNG, JPEG, GIF, WebP, max 3.4MB each)")
}
