package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var shipCmd = &cobra.Command{
	Use:   "ship",
	Short: "Run security scan, check eligibility, and deploy",
	Long:  "Composite command: security_scan + check_publish_eligibility + publish_project",
	Example: `  poof ship -p <id>
  poof ship -p <id> -t production --yes
  poof ship -p <id> --dry-run
  poof ship -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		target, _ := cmd.Flags().GetString("target")
		if target == "" {
			target = "preview"
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		yes, _ := cmd.Flags().GetBool("yes")

		if target == "production" && !yes && !dryRun {
			return fmt.Errorf("shipping to production requires --yes to confirm\n  poof ship -p %s -t production --yes", projectID)
		}

		ctx := context.Background()

		// 1. Security scan
		output.Info("Running security scan...")
		var scanResult *api.SecurityScanResponse
		err = output.WithSpinner("Scanning...", func() error {
			var scanErr error
			scanResult, scanErr = apiClient.SecurityScan(ctx, projectID)
			return scanErr
		})
		if err != nil {
			return fmt.Errorf("security scan failed: %w", err)
		}
		if scanResult.Summary.Critical > 0 {
			output.Error("Security scan found %d critical issue(s). Fix them before deploying.", scanResult.Summary.Critical)
			return nil
		}
		output.Success("Security scan passed. (%d total, %d high, %d medium)",
			scanResult.Summary.Total, scanResult.Summary.High, scanResult.Summary.Medium)

		// 2. Check eligibility
		eligibility, err := apiClient.CheckPublishEligibility(ctx, projectID)
		if err != nil {
			return handleError(err)
		}
		if !eligibility.Eligible {
			output.Error("Not eligible for deployment: %s", eligibility.Reason)
			return nil
		}
		output.Success("Eligible for deployment.")

		// 3. Deploy
		if dryRun {
			output.Info("Would deploy project %s to %s. No changes made.", projectID, target)
			return nil
		}
		output.Info("Deploying to %s...", target)
		if err := apiClient.PublishProject(ctx, projectID, target); err != nil {
			return handleError(err)
		}

		output.Success("Deployed to %s!", target)

		// 4. Get updated status for URLs
		status, err := apiClient.GetProjectStatus(ctx, projectID)
		if err == nil {
			output.Print(map[string]interface{}{
				"target":    target,
				"projectId": projectID,
				"urls":      status.URLs,
			}, func() {
				for name, url := range status.URLs {
					if url != "" {
						fmt.Printf("  %s: %s\n", name, url)
					}
				}
			})
		}

		return nil
	},
}

func init() {
	shipCmd.Flags().StringP("target", "t", "preview", "Deploy target: preview, production, mobile")
	shipCmd.Flags().Bool("dry-run", false, "Run scan and check eligibility, but don't deploy")
	shipCmd.Flags().Bool("yes", false, "Skip confirmation (required for production)")
}
