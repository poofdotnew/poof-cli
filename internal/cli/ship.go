package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/poll"
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
		validTargets := map[string]bool{"preview": true, "production": true, "mobile": true}
		if !validTargets[target] {
			return fmt.Errorf("invalid target %q (valid: preview, production, mobile)", target)
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		yes, _ := cmd.Flags().GetBool("yes")

		if target == "production" && !yes && !dryRun {
			return fmt.Errorf("shipping to production requires --yes to confirm\n  poof ship -p %s -t production --yes", projectID)
		}

		ctx := context.Background()

		// 1. Security scan — initiate and wait for completion
		if output.GetFormat() == output.FormatText {
			output.Info("Initiating security scan...")
		}
		var scanResult *api.SecurityScanResponse
		err = output.WithSpinner("Scanning...", func() error {
			var scanErr error
			scanResult, scanErr = apiClient.SecurityScan(ctx, projectID)
			return scanErr
		})
		if err != nil {
			return fmt.Errorf("security scan failed: %w", err)
		}
		if output.GetFormat() == output.FormatText {
			output.Success("Security scan initiated (task: %s).", scanResult.TaskID)
		}

		// Wait for the security scan task to complete
		if scanResult.TaskID != "" {
			err = output.WithSpinner("Waiting for security scan to complete...", func() error {
				scanPollCfg := poll.Config{
					InitialDelay:      3 * time.Second,
					MaxDelay:          10 * time.Second,
					BackoffFactor:     1.3,
					Timeout:           5 * time.Minute,
					MaxConsecutiveErr: 5,
				}
				return poll.Poll(ctx, scanPollCfg, func(ctx context.Context) (bool, error) {
					task, err := apiClient.GetTask(ctx, projectID, scanResult.TaskID)
					if err != nil {
						return false, err
					}
					switch task.Task.Status {
					case "completed":
						return true, nil
					case "failed":
						return false, fmt.Errorf("security scan task failed")
					default:
						return false, nil
					}
				})
			})
			if err != nil {
				return fmt.Errorf("security scan failed: %w", err)
			}
			if output.GetFormat() == output.FormatText {
				output.Success("Security scan completed.")
			}
		}

		// 2. Check eligibility
		eligibility, err := apiClient.CheckPublishEligibility(ctx, projectID)
		if err != nil {
			return handleError(err)
		}
		if !eligibility.Eligible() {
			if eligibility.Status == "no_membership" {
				return fmt.Errorf("not eligible for deployment: a credit purchase is required. Run 'poof credits topup' first")
			}
			return fmt.Errorf("not eligible for deployment (%s): %s", eligibility.Status, eligibility.Message)
		}
		if output.GetFormat() == output.FormatText {
			output.Success("Eligible for deployment.")
		}

		// 3. Deploy
		if dryRun {
			output.Print(map[string]interface{}{
				"dryRun":    true,
				"projectId": projectID,
				"target":    target,
			}, func() {
				output.Info("Would deploy project %s to %s. No changes made.", projectID, target)
			})
			return nil
		}
		if output.GetFormat() == output.FormatText {
			output.Info("Deploying to %s...", target)
		}

		switch target {
		case "mobile":
			platform, _ := cmd.Flags().GetString("platform")
			appName, _ := cmd.Flags().GetString("app-name")
			appIconUrl, _ := cmd.Flags().GetString("app-icon-url")
			if platform == "" || appName == "" || appIconUrl == "" {
				return fmt.Errorf("mobile deploy requires --platform, --app-name, and --app-icon-url")
			}
			appDesc, _ := cmd.Flags().GetString("app-description")
			themeColor, _ := cmd.Flags().GetString("theme-color")
			isDraft, _ := cmd.Flags().GetBool("draft")
			targetEnv, _ := cmd.Flags().GetString("target-environment")
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
		default:
			// preview and production — permit signing is handled automatically
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
			if err := apiClient.PublishProject(ctx, projectID, target, opts); err != nil {
				return handleError(err)
			}
		}

		// 4. Get updated status for URLs
		status, err := apiClient.GetProjectStatus(ctx, projectID)
		if err == nil {
			if output.GetFormat() == output.FormatQuiet {
				output.Quiet(projectID)
			} else {
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
		}

		return nil
	},
}

func init() {
	shipCmd.Flags().StringP("target", "t", "preview", "Deploy target: preview, production, mobile")
	shipCmd.Flags().Bool("dry-run", false, "Run scan and check eligibility, but don't deploy")
	shipCmd.Flags().Bool("yes", false, "Skip confirmation (required for production)")
	shipCmd.Flags().String("allowed-addresses", "", "Comma-separated wallet addresses allowed to access preview (max 10)")
	shipCmd.Flags().String("constants-overrides", "", "JSON object of constants overrides")
	shipCmd.Flags().String("config", "", "JSON object of config overrides (e.g. title, favicon)")
	shipCmd.Flags().String("platform", "", "Mobile platform: ios, android, seeker")
	shipCmd.Flags().String("app-name", "", "Mobile app name")
	shipCmd.Flags().String("app-icon-url", "", "Mobile app icon URL")
	shipCmd.Flags().String("app-description", "", "Mobile app description")
	shipCmd.Flags().String("theme-color", "#0a0a0a", "Mobile theme color (hex, e.g. #0a0a0a)")
	shipCmd.Flags().Bool("draft", false, "Publish mobile app as draft")
	shipCmd.Flags().String("target-environment", "", "Mobile target environment: draft, mainnet-preview")
}
