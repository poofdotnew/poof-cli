package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/poll"
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
			return fmt.Errorf("--message is required\n  poof iterate -p %s -m \"Add a feature\"", projectID)
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

		// 1. Send chat message
		_, err = apiClient.Chat(ctx, projectID, message, attachedFiles)
		if err != nil {
			return handleError(err)
		}
		if output.GetFormat() == output.FormatText {
			output.Info("Message sent. AI is building...")
		}

		// 2. Poll until AI finishes
		// Track whether we've seen the AI become active to avoid declaring
		// "done" before it has started (race between sending the message
		// and the server activating the AI).
		seenActive := false
		pollStart := time.Now()
		const activationGrace = 30 * time.Second

		err = output.WithSpinner("AI is working...", func() error {
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
				// AI is not active — only consider done if we've seen it
				// active at least once, or the grace period has elapsed.
				if seenActive || time.Since(pollStart) > activationGrace {
					return true, nil
				}
				return false, nil
			})
		})
		if err != nil {
			return fmt.Errorf("timed out or failed: %w", err)
		}

		// 3. Check test results (may not exist for all projects)
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

		output.Print(results, func() {
			if results.Summary.Total == 0 {
				printNoTestResultsGuidance(projectID)
			} else if results.Summary.Failed > 0 || results.Summary.Errors > 0 {
				output.Warn("Done with test failures.")
				output.Info("Tests: %d passed, %d failed, %d errors (of %d)",
					results.Summary.Passed, results.Summary.Failed,
					results.Summary.Errors, results.Summary.Total)
			} else {
				output.Success("Done! All tests passed.")
				output.Info("Tests: %d passed (of %d)",
					results.Summary.Passed, results.Summary.Total)
			}
		})
		return nil
	},
}

func printNoTestResultsGuidance(projectID string) {
	output.Warn("Done, but no test results were found.")
	output.Info("Treat this as missing test artifacts, not a passing run.")
	output.Info("Inspect: poof task list -p %s --json", projectID)
	output.Info("Inspect: poof chat active -p %s --json", projectID)
	output.Info("Inspect: poof logs -p %s", projectID)
	output.Info("Inspect: poof project messages -p %s --limit 100 --json", projectID)
	output.Info("If chat is still active with no new task ids or logs, cancel once and do one targeted retry.")
}

func init() {
	iterateCmd.Flags().StringP("message", "m", "", "Message to send (required)")
	iterateCmd.Flags().Bool("stdin", false, "Read message from stdin")
	iterateCmd.Flags().StringSlice("file", nil, "Image file(s) to attach (PNG, JPEG, GIF, WebP, max 3.4MB each)")
}
