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

		// 1. Security scan (async — initiates scan)
		output.Info("Initiating security scan...")
		var scanResult *api.SecurityScanResponse
		err = output.WithSpinner("Scanning...", func() error {
			var scanErr error
			scanResult, scanErr = apiClient.SecurityScan(ctx, projectID)
			return scanErr
		})
		if err != nil {
			return fmt.Errorf("security scan failed: %w", err)
		}
		output.Success("Security scan initiated (task: %s).", scanResult.TaskID)

		// 2. Check eligibility
		eligibility, err := apiClient.CheckPublishEligibility(ctx, projectID)
		if err != nil {
			return handleError(err)
		}
		if !eligibility.Eligible() {
			return fmt.Errorf("not eligible for deployment (%s): %s", eligibility.Status, eligibility.Message)
		}
		output.Success("Eligible for deployment.")

		// 3. Deploy
		if dryRun {
			output.Info("Would deploy project %s to %s. No changes made.", projectID, target)
			return nil
		}
		output.Info("Deploying to %s...", target)

		switch target {
		case "preview":
			signedPermit, _ := cmd.Flags().GetString("signed-permit")
			if signedPermit == "" {
				return fmt.Errorf("--signed-permit is required for preview deploy\n  poof ship -p %s -t preview --signed-permit <transaction>", projectID)
			}
			if err := apiClient.PublishProject(ctx, projectID, target, signedPermit); err != nil {
				return handleError(err)
			}
		case "mobile":
			platform, _ := cmd.Flags().GetString("platform")
			appName, _ := cmd.Flags().GetString("app-name")
			appIconUrl, _ := cmd.Flags().GetString("app-icon-url")
			if platform == "" || appName == "" || appIconUrl == "" {
				return fmt.Errorf("mobile deploy requires --platform, --app-name, and --app-icon-url")
			}
			mobileReq := &api.MobilePublishRequest{
				Platform:   platform,
				AppName:    appName,
				AppIconUrl: appIconUrl,
			}
			if err := apiClient.PublishProject(ctx, projectID, target, mobileReq); err != nil {
				return handleError(err)
			}
		default:
			if err := apiClient.PublishProject(ctx, projectID, target); err != nil {
				return handleError(err)
			}
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
						output.Info("  %s: %s", name, url)
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
	shipCmd.Flags().String("signed-permit", "", "Signed permit transaction (required for preview)")
	shipCmd.Flags().String("platform", "", "Mobile platform: ios, android, seeker")
	shipCmd.Flags().String("app-name", "", "Mobile app name")
	shipCmd.Flags().String("app-icon-url", "", "Mobile app icon URL")
}
