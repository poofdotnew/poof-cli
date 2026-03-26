package cli

import (
	"context"
	"fmt"

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

		if useStdin {
			message = readStdin()
		}
		if message == "" {
			return fmt.Errorf("--message is required\n  poof iterate -p %s -m \"Add a feature\"", projectID)
		}

		ctx := context.Background()

		// 1. Send chat message
		chatResp, err := apiClient.Chat(ctx, projectID, message)
		if err != nil {
			return handleError(err)
		}

		if chatResp.Queued {
			output.Info("Message queued (AI was active). Waiting...")
		} else {
			output.Info("Message sent. AI is building...")
		}

		// 2. Poll until AI finishes
		err = output.WithSpinner("AI is working...", func() error {
			return poll.Poll(ctx, poll.DefaultConfig(), func(ctx context.Context) (bool, error) {
				status, err := apiClient.CheckAIActive(ctx, projectID)
				if err != nil {
					return false, err
				}
				if status.Status == "error" {
					return false, fmt.Errorf("AI processing failed with error status")
				}
				return !status.Active, nil
			})
		})
		if err != nil {
			return fmt.Errorf("timed out or failed: %w", err)
		}

		// 3. Check test results
		results, err := apiClient.GetTestResults(ctx, projectID)
		if err != nil {
			// Test results may not exist; that's OK
			output.Success("Done.")
			return nil
		}

		output.Print(results, func() {
			output.Success("Done!")
			if results.Summary.Total > 0 {
				output.Info("Tests: %d passed, %d failed, %d errors (of %d)",
					results.Summary.Passed, results.Summary.Failed,
					results.Summary.Errors, results.Summary.Total)
			}
		})
		return nil
	},
}

func init() {
	iterateCmd.Flags().StringP("message", "m", "", "Message to send (required)")
	iterateCmd.Flags().Bool("stdin", false, "Read message from stdin")
}
