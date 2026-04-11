package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/poll"
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

		if !resp.Eligible() {
			if output.GetFormat() == output.FormatJSON {
				output.JSON(resp)
				cmd.SilenceErrors = true
			}
			if resp.Status == "no_membership" {
				return fmt.Errorf("not eligible for deployment: a credit purchase is required. Run 'poof credits topup' first")
			}
			return fmt.Errorf("not eligible for deployment (%s): %s", resp.Status, resp.Message)
		}

		output.Print(resp, func() {
			output.Success("Project is eligible for deployment.")
		})
		return nil
	},
}

var deployPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Deploy to mainnet preview",
	Example: `  poof deploy preview -p <id>
  poof deploy preview -p <id> --dry-run`,
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

		var publishResult *api.PublishResult
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
			publishResult, err = apiClient.PublishProject(ctx, projectID, target, mobileReq)
			if err != nil {
				return handleError(err)
			}

		default:
			opts := &api.PublishOptions{}
			if addrs, _ := cmd.Flags().GetString("allowed-addresses"); addrs != "" {
				opts.AllowedAddresses = strings.Split(addrs, ",")
			}
			if overrides, _ := cmd.Flags().GetString("constants-overrides"); overrides != "" {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(overrides), &parsed); err != nil {
					return fmt.Errorf("--constants-overrides must be valid JSON: %w", err)
				}
				opts.ConstantsOverrides = parsed
			}
			if cfg, _ := cmd.Flags().GetString("config"); cfg != "" {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
					return fmt.Errorf("--config must be valid JSON: %w", err)
				}
				opts.Config = parsed
			}
			publishResult, err = apiClient.PublishProject(ctx, projectID, target, opts)
			if err != nil {
				return handleError(err)
			}
		}

		if publishResult != nil && publishResult.DeploymentTaskID != "" && target != "mobile" {
			if err := waitForDeploy(ctx, projectID, publishResult.DeploymentTaskID, target); err != nil {
				return err
			}
		}

		status, err := apiClient.GetProjectStatus(ctx, projectID)
		if err == nil {
			var targetURL string
			switch target {
			case "preview":
				targetURL = status.URLs["mainnetPreview"]
			case "production":
				targetURL = status.URLs["production"]
			case "mobile":
				targetURL = status.URLs["mainnetPreview"]
			}

			output.Print(map[string]interface{}{
				"target":    target,
				"projectId": projectID,
				"url":       targetURL,
				"urls":      status.URLs,
			}, func() {
				output.Success("Deployed to %s.", target)
				if targetURL != "" {
					output.Info("  %s: %s", target, targetURL)
				}
				if draft := status.URLs["draft"]; draft != "" && draft != targetURL {
					output.Info("  draft: %s", draft)
				}
				if prod := status.URLs["production"]; prod != "" && prod != targetURL && target != "production" {
					output.Info("  production: %s", prod)
				}
			})
		} else {
			output.Success("Deployed to %s. (could not fetch updated URLs)", target)
		}
		return nil
	}
}

var deployStaticCmd = &cobra.Command{
	Use:   "static",
	Short: "Deploy a pre-built static frontend",
	Example: `  poof deploy static -p <id> --archive dist.tar.gz
  poof deploy static -p <id> --archive dist.tar.gz --title "v2.0 release"
  poof deploy static -p <id> --archive dist.tar.gz --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		archivePath, _ := cmd.Flags().GetString("archive")
		if archivePath == "" {
			return fmt.Errorf("--archive is required\n  poof deploy static -p %s --archive dist.tar.gz", projectID)
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")

		// Read and validate the archive
		archive, err := os.ReadFile(archivePath)
		if err != nil {
			return fmt.Errorf("failed to read archive %q: %w", archivePath, err)
		}

		if len(archive) < 2 || archive[0] != 0x1f || archive[1] != 0x8b {
			return fmt.Errorf("file %q is not a valid gzip archive. Create with: tar czf dist.tar.gz -C dist .", archivePath)
		}

		if dryRun {
			output.Info("Would deploy static frontend from %s (%d bytes) to project %s. No changes made.", archivePath, len(archive), projectID)
			return nil
		}

		ctx := context.Background()
		resp, err := apiClient.DeployStatic(ctx, projectID, archive, title, description)
		if err != nil {
			return handleError(err)
		}

		// Match existing deploy pattern: get URLs after deploy
		status, sErr := apiClient.GetProjectStatus(ctx, projectID)
		if sErr == nil {
			output.Print(map[string]interface{}{
				"target":    "static",
				"projectId": projectID,
				"taskId":    resp.TaskID,
				"bundleUrl": resp.BundleURL,
				"urls":      status.URLs,
			}, func() {
				output.Success("Static frontend deployed.")
				for name, url := range status.URLs {
					if url != "" {
						output.Info("  %s: %s", name, url)
					}
				}
			})
		} else {
			output.Print(resp, func() {
				output.Success("Static frontend deployed.")
				if resp.BundleURL != "" {
					output.Info("  URL: %s", resp.BundleURL)
				}
			})
		}
		return nil
	},
}

var deployDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Start code export",
	Example: `  poof deploy download -p <id>
  TASK_ID=$(poof deploy download -p <id> --json | jq -r '.taskId')`,
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

func waitForDeploy(ctx context.Context, projectID, taskID, target string) error {
	err := output.WithSpinner(fmt.Sprintf("Waiting for %s deploy...", target), func() error {
		cfg := poll.Config{
			InitialDelay:      4 * time.Second,
			MaxDelay:          15 * time.Second,
			BackoffFactor:     1.3,
			Timeout:           10 * time.Minute,
			MaxConsecutiveErr: 5,
		}
		return poll.Poll(ctx, cfg, func(ctx context.Context) (bool, error) {
			task, pollErr := apiClient.GetTask(ctx, projectID, taskID)
			if pollErr != nil {
				return false, pollErr
			}
			switch task.Task.Status {
			case "completed":
				return true, nil
			case "failed":
				return false, fmt.Errorf("deploy task %s failed", taskID)
			default:
				return false, nil
			}
		})
	})
	if err != nil {
		task, checkErr := apiClient.GetTask(ctx, projectID, taskID)
		if checkErr == nil && task.Task.Status == "completed" {
			return nil
		}
		return fmt.Errorf("%s deploy did not finish: %w", target, err)
	}
	return nil
}

func init() {
	// Preview flags
	deployPreviewCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployPreviewCmd.Flags().Bool("yes", false, "Skip confirmation")
	deployPreviewCmd.Flags().String("allowed-addresses", "", "Comma-separated wallet addresses allowed to access preview (max 10)")
	deployPreviewCmd.Flags().String("constants-overrides", "", "JSON object of constants overrides for preview")
	deployPreviewCmd.Flags().String("config", "", "JSON object of config overrides for preview (e.g. title, favicon)")

	// Production flags
	deployProductionCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployProductionCmd.Flags().Bool("yes", false, "Skip confirmation (required for production)")
	deployProductionCmd.Flags().String("constants-overrides", "", "JSON object of constants overrides for production")
	deployProductionCmd.Flags().String("config", "", "JSON object of config overrides for production (e.g. title, favicon)")

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

	// Static deploy flags
	deployStaticCmd.Flags().String("archive", "", "Path to tar.gz archive of your dist/ folder (required)")
	deployStaticCmd.Flags().String("title", "", "Checkpoint title")
	deployStaticCmd.Flags().String("description", "", "Checkpoint description")
	deployStaticCmd.Flags().Bool("dry-run", false, "Validate without deploying")

	deployCmd.AddCommand(deployCheckCmd)
	deployCmd.AddCommand(deployPreviewCmd)
	deployCmd.AddCommand(deployProductionCmd)
	deployCmd.AddCommand(deployMobileCmd)
	deployCmd.AddCommand(deployStaticCmd)
	deployCmd.AddCommand(deployDownloadCmd)
	deployCmd.AddCommand(deployDownloadURLCmd)
}
