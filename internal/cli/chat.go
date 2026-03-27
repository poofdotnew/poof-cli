package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Chat with the AI builder",
}

var chatSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to the AI",
	Example: `  poof chat send -p <id> -m "Add a leaderboard page"
  poof chat send -p <id> -m "Fix the login button"
  echo "Add dark mode" | poof chat send -p <id> --stdin`,
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
			return fmt.Errorf("--message is required\n  poof chat send -p %s -m \"Add a feature\"", projectID)
		}

		resp, err := apiClient.Chat(context.Background(), projectID, message)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Success("Message sent. AI is building...")
			output.Info("Message ID: %s", resp.MessageID)
		})
		return nil
	},
}

var chatActiveCmd = &cobra.Command{
	Use:   "active",
	Short: "Check if AI is currently processing",
	Example: `  poof chat active -p <id>
  poof chat active -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.CheckAIActive(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.Active {
				output.Info("AI is active (status: %s)", resp.Status)
			} else {
				output.Info("AI is idle (status: %s)", resp.Status)
			}
		})
		return nil
	},
}

var chatCancelCmd = &cobra.Command{
	Use:     "cancel",
	Short:   "Cancel in-progress AI execution",
	Example: `  poof chat cancel -p <id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		if err := apiClient.CancelAI(context.Background(), projectID); err != nil {
			return handleError(err)
		}

		output.Success("AI execution canceled.")
		return nil
	},
}

var chatSteerCmd = &cobra.Command{
	Use:   "steer",
	Short: "Redirect AI mid-execution without canceling",
	Example: `  poof chat steer -p <id> -m "Focus on the backend first"
  poof chat steer -p <id> -m "Skip the UI, just do policies"`,
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
			return fmt.Errorf("--message is required\n  poof chat steer -p %s -m \"Focus on backend\"", projectID)
		}

		if err := apiClient.SteerAI(context.Background(), projectID, message); err != nil {
			return handleError(err)
		}

		output.Success("Steering message sent.")
		return nil
	},
}

func init() {
	chatSendCmd.Flags().StringP("message", "m", "", "Message to send (required)")
	chatSendCmd.Flags().Bool("stdin", false, "Read message from stdin")
	chatSteerCmd.Flags().StringP("message", "m", "", "Steering message (required)")
	chatSteerCmd.Flags().Bool("stdin", false, "Read message from stdin")

	chatCmd.AddCommand(chatSendCmd)
	chatCmd.AddCommand(chatActiveCmd)
	chatCmd.AddCommand(chatCancelCmd)
	chatCmd.AddCommand(chatSteerCmd)
}
