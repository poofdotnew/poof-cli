package cli

import (
	"context"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Get runtime logs for a deployed project",
	Example: `  poof logs -p <id>
  poof logs -p <id> --environment preview --limit 50
  poof logs -p <id> --offset 50
  poof logs -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		environment, _ := cmd.Flags().GetString("environment")
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		resp, err := apiClient.GetLogs(context.Background(), projectID, environment, limit, offset)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Logs) == 0 {
				output.Info("No logs found.")
				return
			}
			for _, l := range resp.Logs {
				output.Info("[%s] %s: %s", l.Timestamp, l.Level, l.Message)
			}
			if resp.TotalCount > len(resp.Logs) {
				output.Info("\nShowing %d of %d total log entries.", len(resp.Logs), resp.TotalCount)
			}
			if resp.HasMore {
				output.Info("(more logs available — use --offset %d to see next page)", offset+limit)
			}
		})
		return nil
	},
}

func init() {
	logsCmd.Flags().String("environment", "", "Filter by environment: development, mainnet-preview, production")
	logsCmd.Flags().Int("limit", 50, "Max log entries (server max: 50)")
	logsCmd.Flags().Int("offset", 0, "Offset for pagination")
}
