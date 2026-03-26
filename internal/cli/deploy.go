package cli

import (
	"context"
	"fmt"

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
			if resp.Eligible {
				output.Success("Project is eligible for deployment.")
			} else {
				output.Error("Not eligible: %s", resp.Reason)
			}
		})
		return nil
	},
}

var deployPreviewCmd = &cobra.Command{
	Use:     "preview",
	Short:   "Deploy to mainnet preview",
	Example: `  poof deploy preview -p <id>`,
	RunE:    deployTarget("preview"),
}

var deployProductionCmd = &cobra.Command{
	Use:   "production",
	Short: "Deploy to production",
	Example: `  poof deploy production -p <id> --yes
  poof deploy production -p <id> --dry-run`,
	RunE: deployTarget("production"),
}

var deployMobileCmd = &cobra.Command{
	Use:     "mobile",
	Short:   "Publish mobile app",
	Example: `  poof deploy mobile -p <id>`,
	RunE:    deployTarget("mobile"),
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

		if err := apiClient.PublishProject(context.Background(), projectID, target); err != nil {
			return handleError(err)
		}

		// Get URLs after deploy
		status, err := apiClient.GetProjectStatus(context.Background(), projectID)
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
			output.Info("Download URL (expires in 5 min):")
			output.Quiet(resp.URL)
		})
		return nil
	},
}

func init() {
	deployPreviewCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployPreviewCmd.Flags().Bool("yes", false, "Skip confirmation")
	deployProductionCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployProductionCmd.Flags().Bool("yes", false, "Skip confirmation (required for production)")
	deployMobileCmd.Flags().Bool("dry-run", false, "Preview what would happen without deploying")
	deployMobileCmd.Flags().Bool("yes", false, "Skip confirmation")
	deployDownloadURLCmd.Flags().String("task", "", "Task ID from download command (required)")

	deployCmd.AddCommand(deployCheckCmd)
	deployCmd.AddCommand(deployPreviewCmd)
	deployCmd.AddCommand(deployProductionCmd)
	deployCmd.AddCommand(deployMobileCmd)
	deployCmd.AddCommand(deployDownloadCmd)
	deployCmd.AddCommand(deployDownloadURLCmd)
}
