package cli

import (
	"context"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security tools",
}

var securityScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Initiate a security audit",
	Example: `  poof security scan -p <id>
  poof security scan -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		var resp *api.SecurityScanResponse
		err = output.WithSpinner("Initiating security scan...", func() error {
			var scanErr error
			resp, scanErr = apiClient.SecurityScan(context.Background(), projectID)
			return scanErr
		})
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Success("Security scan initiated.")
			if resp.TaskID != "" {
				output.Info("  Task:    %s", resp.TaskID)
			}
			if resp.TaskTitle != "" {
				output.Info("  Scanning: %s", resp.TaskTitle)
			}
			output.Info("  Message: %s", resp.Message)
		})
		return nil
	},
}

func init() {
	securityCmd.AddCommand(securityScanCmd)
}
