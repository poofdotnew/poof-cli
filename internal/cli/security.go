package cli

import (
	"context"
	"fmt"

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
	Short: "Run a security audit",
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
		err = output.WithSpinner("Running security scan...", func() error {
			var scanErr error
			resp, scanErr = apiClient.SecurityScan(context.Background(), projectID)
			return scanErr
		})
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.Status != "" && len(resp.Vulnerabilities) == 0 {
				output.Success("Security scan complete: %s", resp.Status)
				return
			}

			s := resp.Summary
			output.Info("Security scan results:")
			output.Info("  Total: %d  Critical: %d  High: %d  Medium: %d  Low: %d",
				s.Total, s.Critical, s.High, s.Medium, s.Low)

			if s.Critical == 0 && s.High == 0 {
				output.Success("No critical or high severity issues found.")
			}

			if len(resp.Vulnerabilities) > 0 {
				fmt.Println()
				for _, v := range resp.Vulnerabilities {
					prefix := "  "
					if v.Severity == "critical" || v.Severity == "high" {
						prefix = "! "
					}
					loc := ""
					if v.File != "" {
						loc = fmt.Sprintf(" (%s", v.File)
						if v.Line > 0 {
							loc += fmt.Sprintf(":%d", v.Line)
						}
						loc += ")"
					}
					fmt.Printf("%s[%s] %s: %s%s\n", prefix, v.Severity, v.Category, v.Description, loc)
				}
			}
		})
		return nil
	},
}

func init() {
	securityCmd.AddCommand(securityScanCmd)
}
