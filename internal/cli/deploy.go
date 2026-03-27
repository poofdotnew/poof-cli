package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy and download projects",
}

var deployCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check publish eligibility",
	Example: `  poof deploy check -p <id>
  poof deploy check -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.CheckPublishEligibility(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.Eligible() {
				output.Success("Project is eligible for deployment.")
			}
		})

		if !resp.Eligible() {
			return fmt.Errorf("not eligible for deployment (%s): %s", resp.Status, resp.Message)
		}
		return nil
	},
}

var deployPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Deploy to mainnet preview",
	Example: `  poof deploy preview -p <id> --signed-permit <tx>
  poof deploy preview -p <id> --signed-permit <tx> --dry-run`,
	RunE: deployTarget("preview"),
}

var deployProductionCmd = &cobra.Command{
	Use:   "production",
	Short: "Deploy to production",
	Example: `  poof deploy production -p <id> --yes
  poof deploy production -p <id> --dry-run`,
	RunE: deployTarget("production"),
}

var deployMobileCmd = &cobra.Command{
	Use:   "mobile",
	Short: "Publish mobile app",
	Example: `  poof deploy mobile -p <id> --platform ios --app-name "My App" --app-icon-url https://...
  poof deploy mobile -p <id> --platform android --app-name "My App" --app-icon-url https://...`,
	RunE: deployTarget("mobile"),
}

func deployTarget(target string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		yes, _ := cmd.Flags().GetBool("yes")

		if dryRun {
			output.Info("Would deploy project %s to %s. No changes made.", projectID, target)
			return nil
		}

		// Production deploy requires --yes
		if target == "production" && !yes {
			return fmt.Errorf("deploying to production requires --yes to confirm\n  poof deploy production -p %s --yes", projectID)
		}

		ctx := context.Background()

		switch target {
		case "mobile":
			platform, _ := cmd.Flags().GetString("platform")
			appName, _ := cmd.Flags().GetString("app-name")
			appIconUrl, _ := cmd.Flags().GetString("app-icon-url")
			appDesc, _ := cmd.Flags().GetString("app-description")
			themeColor, _ := cmd.Flags().GetString("theme-color")
			isDraft, _ := cmd.Flags().GetBool("draft")
			targetEnv, _ := cmd.Flags().GetString("target-environment")

			if platform == "" {
				return fmt.Errorf("--platform is required for mobile deploy (ios, android, seeker)")
			}
			if appName == "" {
				return fmt.Errorf("--app-name is required for mobile deploy")
			}
			if appIconUrl == "" {
				return fmt.Errorf("--app-icon-url is required for mobile deploy")
			}

			mobileReq := &api.MobilePublishRequest{
				Platform:          platform,
				AppName:           appName,
				AppIconUrl:        appIconUrl,
				AppDescription:    appDesc,
				ThemeColor:        themeColor,
				IsDraft:           isDraft,
				TargetEnvironment: targetEnv,
			}
			if err := apiClient.PublishProject(ctx, projectID, target, mobileReq); err != nil {
				return handleError(err)
			}

		case "preview":
			signedPermit, _ := cmd.Flags().GetString("signed-permit")
			if signedPermit == "" {
				return fmt.Errorf("--signed-permit is required for preview deploy\n  poof deploy preview -p %s --signed-permit <transaction>", projectID)
			}
			if err := apiClient.PublishProject(ctx, projectID, target, signedPermit); err != nil {
				return handleError(err)
			}

		default:
			signedPermit, _ := cmd.Flags().GetString("signed-permit")
			if signedPermit != "" {
				if err := apiClient.PublishProject(ctx, projectID, target, signedPermit); err != nil {
					return handleError(err)
				}
			} else {
				if err := apiClient.PublishProject(ctx, projectID, target); err != nil {
					return handleError(err)
				}
			}
		}

		// Get URLs after deploy
		status, err := apiClient.GetProjectStatus(ctx, projectID)
		if err == nil {
			output.Print(map[string]interface{}{
				"target":    target,
				"projectId": projectID,
				"urls":      status.URLs,
			}, func() {
				output.Success("Deployed to %s.", target)
				for name, url := range status.URLs {
					if url != "" {
						output.Info("  %s: %s", name, url)
					}
				}
			})
		} else {
			output.Success("Deployed to %s.", target)
		}
		return nil
	}
}

var deployDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Start code export",
	Example: `  poof deploy download -p <id>
  TASK_ID=$(poof deploy download -p <id> --json | jq -r '.TaskID')`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.DownloadCode(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Success("Download started. Task ID: %s", resp.TaskID)
			output.Info("Run 'poof deploy download-url -p %s --task %s' to get the download link.", projectID, resp.TaskID)
		})
		return nil
	},
}

var deployDownloadURLCmd = &cobra.Command{
	Use:   "download-url",
	Short: "Get signed download URL",
	Example: `  poof deploy download-url -p <id> --task <taskId>
  poof deploy download-url -p <id> --task <taskId> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		taskID, _ := cmd.Flags().GetString("task")
		if taskID == "" {
			return fmt.Errorf("--task is required\n  poof deploy download-url -p %s --task <taskId>", projectID)
		}

		resp, err := apiClient.GetDownloadURL(context.Background(), projectID, taskID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Info("Download URL (expires %s):", resp.ExpiresAt)
			output.Quiet(resp.URL)
		})
		return nil
	},
}

func init() {
	// Preview flags
	deployPreviewCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployPreviewCmd.Flags().Bool("yes", false, "Skip confirmation")
	deployPreviewCmd.Flags().String("signed-permit", "", "Signed permit transaction (required for preview)")

	// Production flags
	deployProductionCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployProductionCmd.Flags().Bool("yes", false, "Skip confirmation (required for production)")
	deployProductionCmd.Flags().String("signed-permit", "", "Signed permit transaction (required for subsequent deploys)")

	// Mobile flags
	deployMobileCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployMobileCmd.Flags().Bool("yes", false, "Skip confirmation")
	deployMobileCmd.Flags().String("platform", "", "Target platform: ios, android, seeker (required)")
	deployMobileCmd.Flags().String("app-name", "", "App name (required)")
	deployMobileCmd.Flags().String("app-icon-url", "", "App icon URL (required)")
	deployMobileCmd.Flags().String("app-description", "", "App description")
	deployMobileCmd.Flags().String("theme-color", "#0a0a0a", "Theme color (hex, e.g. #0a0a0a)")
	deployMobileCmd.Flags().Bool("draft", false, "Publish as draft")
	deployMobileCmd.Flags().String("target-environment", "", "Target environment: draft, mainnet-preview")

	deployDownloadURLCmd.Flags().String("task", "", "Task ID from download command (required)")

	deployCmd.AddCommand(deployCheckCmd)
	deployCmd.AddCommand(deployPreviewCmd)
	deployCmd.AddCommand(deployProductionCmd)
	deployCmd.AddCommand(deployMobileCmd)
	deployCmd.AddCommand(deployDownloadCmd)
	deployCmd.AddCommand(deployDownloadURLCmd)
}
