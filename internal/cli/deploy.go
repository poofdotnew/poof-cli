package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
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
	Short: "Deploy a pre-built static frontend (draft tier only)",
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

		archive, err := os.ReadFile(archivePath)
		if err != nil {
			return fmt.Errorf("failed to read archive %q: %w", archivePath, err)
		}

		if len(archive) < 2 || archive[0] != 0x1f || archive[1] != 0x8b {
			return fmt.Errorf("file %q is not a valid gzip archive. Create with: tar czf dist.tar.gz -C dist .", archivePath)
		}

		if dryRun {
			output.Print(map[string]interface{}{
				"success":     true,
				"dryRun":      true,
				"target":      "static",
				"projectId":   projectID,
				"archivePath": archivePath,
				"bytes":       len(archive),
			}, func() {
				output.Info("Would deploy static frontend from %s (%d bytes) to project %s (draft). No changes made.", archivePath, len(archive), projectID)
			})
			return nil
		}

		ctx := context.Background()
		resp, err := apiClient.DeployStatic(ctx, projectID, archive, title, description)
		if err != nil {
			return handleError(err)
		}

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

type backendArtifactManifest struct {
	Entrypoint      string `json:"entrypoint"`
	Main            string `json:"main"`
	WranglerVersion string `json:"wranglerVersion"`
	APISpecPath     string `json:"apiSpecPath"`
	QueuesPath      string `json:"queuesPath"`
	HeartbeatPath   string `json:"heartbeatPath"`
}

func cleanArchivePath(value, field string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("poof-backend-artifact.json must include %s", field)
	}
	if strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", fmt.Errorf("%s must not contain control characters", field)
	}
	if strings.Contains(value, "\\") {
		return "", fmt.Errorf("%s must use POSIX archive paths", field)
	}
	cleaned := path.Clean(value)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("%s must be a safe relative archive path", field)
	}
	return cleaned, nil
}

func cleanArchiveEntryPath(value string) (string, bool, error) {
	if strings.TrimSpace(value) == "" {
		return "", false, fmt.Errorf("archive entry must not be empty")
	}
	if strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", false, fmt.Errorf("archive entry must not contain control characters")
	}
	if strings.Contains(value, "\\") {
		return "", false, fmt.Errorf("archive entry must use POSIX archive paths")
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return "", true, nil
	}
	if strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") || strings.HasPrefix(cleaned, "/") {
		return "", false, fmt.Errorf("archive entry must be a safe relative archive path")
	}
	return cleaned, false, nil
}

func validateBackendArchive(archive []byte) error {
	if len(archive) < 2 || archive[0] != 0x1f || archive[1] != 0x8b {
		return fmt.Errorf("archive is not a gzip-compressed tar file. Create with: tar czf backend-worker.tar.gz -C .poof-backend-bundle .")
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("failed to read gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	entries := map[string]byte{}
	var manifestBytes []byte

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar archive: %w", err)
		}
		name, skipEntry, err := cleanArchiveEntryPath(hdr.Name)
		if err != nil {
			return err
		}
		if skipEntry {
			continue
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			return fmt.Errorf("archive must not contain links: %s", name)
		}
		if !isTarRegularFile(hdr.Typeflag) && hdr.Typeflag != tar.TypeDir {
			return fmt.Errorf("archive must not contain special entries: %s", name)
		}
		entries[name] = hdr.Typeflag
		if name == "poof-backend-artifact.json" {
			manifestBytes, err = io.ReadAll(io.LimitReader(tr, 1024*1024))
			if err != nil {
				return fmt.Errorf("failed to read poof-backend-artifact.json: %w", err)
			}
		}
	}

	if len(manifestBytes) == 0 {
		return fmt.Errorf("backend archive must include poof-backend-artifact.json")
	}

	var manifest backendArtifactManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("invalid poof-backend-artifact.json: %w", err)
	}

	entrypoint := manifest.Entrypoint
	if entrypoint == "" {
		entrypoint = manifest.Main
	}
	entrypoint, err = cleanArchivePath(entrypoint, "entrypoint")
	if err != nil {
		return err
	}
	if strings.TrimSpace(manifest.WranglerVersion) == "" {
		return fmt.Errorf("poof-backend-artifact.json must include wranglerVersion")
	}
	if typ, ok := entries[entrypoint]; !ok || !isTarRegularFile(typ) {
		return fmt.Errorf("backend archive entrypoint %q was not found as a regular file", entrypoint)
	}

	for field, value := range map[string]string{
		"apiSpecPath":   manifest.APISpecPath,
		"queuesPath":    manifest.QueuesPath,
		"heartbeatPath": manifest.HeartbeatPath,
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		cleaned, err := cleanArchivePath(value, field)
		if err != nil {
			return err
		}
		if typ, ok := entries[cleaned]; !ok || !isTarRegularFile(typ) {
			return fmt.Errorf("backend archive %s %q was not found as a regular file", field, cleaned)
		}
	}

	return nil
}

func isTarRegularFile(typeflag byte) bool {
	return typeflag == tar.TypeReg || typeflag == 0
}

var deployBackendCmd = &cobra.Command{
	Use:   "backend",
	Short: "Deploy a pre-built PartyServer backend bundle (draft tier only)",
	Example: `  bunx wrangler deploy --dry-run --outdir .poof-backend-bundle
  # add poof-backend-artifact.json to .poof-backend-bundle, then:
  tar czf backend-worker.tar.gz -C .poof-backend-bundle .
  poof deploy backend -p <id> --archive backend-worker.tar.gz
  poof deploy backend -p <id> --archive backend-worker.tar.gz --title "backend v2"
  poof deploy backend -p <id> --archive backend-worker.tar.gz --dry-run`,
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
			return fmt.Errorf("--archive is required\n  poof deploy backend -p %s --archive backend-worker.tar.gz", projectID)
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")

		archive, err := os.ReadFile(archivePath)
		if err != nil {
			return fmt.Errorf("failed to read archive %q: %w", archivePath, err)
		}

		if err := validateBackendArchive(archive); err != nil {
			return fmt.Errorf("invalid backend archive %q: %w", archivePath, err)
		}

		if dryRun {
			output.Print(map[string]interface{}{
				"success":     true,
				"dryRun":      true,
				"target":      "backend",
				"projectId":   projectID,
				"archivePath": archivePath,
				"bytes":       len(archive),
			}, func() {
				output.Info("Would deploy backend artifact from %s (%d bytes) to project %s (draft). No changes made.", archivePath, len(archive), projectID)
			})
			return nil
		}

		ctx := context.Background()
		resp, err := apiClient.DeployBackend(ctx, projectID, archive, title, description)
		if err != nil {
			return handleError(err)
		}

		status, sErr := apiClient.GetProjectStatus(ctx, projectID)
		if sErr == nil {
			output.Print(map[string]interface{}{
				"target":     "backend",
				"projectId":  projectID,
				"taskId":     resp.TaskID,
				"backendUrl": resp.BackendURL,
				"urls":       status.URLs,
			}, func() {
				output.Success("Backend artifact deployed.")
				if resp.BackendURL != "" {
					output.Info("  backend: %s", resp.BackendURL)
				}
				for name, url := range status.URLs {
					if url != "" {
						output.Info("  %s: %s", name, url)
					}
				}
			})
		} else {
			output.Print(resp, func() {
				output.Success("Backend artifact deployed.")
				if resp.BackendURL != "" {
					output.Info("  backend: %s", resp.BackendURL)
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

	deployBackendCmd.Flags().String("archive", "", "Path to tar.gz archive of Wrangler bundled backend output (required)")
	deployBackendCmd.Flags().String("title", "", "Checkpoint title")
	deployBackendCmd.Flags().String("description", "", "Checkpoint description")
	deployBackendCmd.Flags().Bool("dry-run", false, "Validate without deploying")

	deployCmd.AddCommand(deployCheckCmd)
	deployCmd.AddCommand(deployPreviewCmd)
	deployCmd.AddCommand(deployProductionCmd)
	deployCmd.AddCommand(deployMobileCmd)
	deployCmd.AddCommand(deployStaticCmd)
	deployCmd.AddCommand(deployBackendCmd)
	deployCmd.AddCommand(deployDownloadCmd)
	deployCmd.AddCommand(deployDownloadURLCmd)
}
