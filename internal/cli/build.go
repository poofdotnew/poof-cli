package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/poll"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Create a project, wait for AI to finish, and show the result",
	Long:  "Composite command: create_project + poll until done + get_project_status",
	Example: `  poof build -m "Build a token-gated voting app"
  poof build -m "NFT marketplace" --mode policy
  poof build -m "Staking dashboard" --public=false
  echo "Build a chat app" | poof build --stdin
  poof build -m "DEX" --json | jq '.urls.draft'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")

		if useStdin {
			message = readStdin()
		}
		if message == "" {
			return fmt.Errorf("--message is required\n  poof build -m \"Build a todo app\"")
		}

		isPublic, _ := cmd.Flags().GetBool("public")
		mode, _ := cmd.Flags().GetString("mode")

		ctx := context.Background()

		// 1. Create project
		output.Info("Creating project...")
		req := api.CreateProjectRequest{
			FirstMessage:   message,
			IsPublic:       isPublic,
			GenerationMode: mode,
		}
		createResp, err := apiClient.CreateProject(ctx, req)
		if err != nil {
			return handleError(err)
		}
		projectID := createResp.ProjectID
		output.Success("Project created: %s", projectID)

		// 2. Poll until AI finishes
		err = output.WithSpinner("AI is building...", func() error {
			return poll.Poll(ctx, poll.DefaultConfig(), func(ctx context.Context) (bool, error) {
				status, err := apiClient.CheckAIActive(ctx, projectID)
				if err != nil {
					return false, err
				}
				if status.Status == "error" {
					return false, fmt.Errorf("AI processing failed with error status")
				}
				return !status.Active, nil
			})
		})
		if err != nil {
			output.Info("Project ID: %s (you can check status with 'poof project status -p %s')", projectID, projectID)
			return fmt.Errorf("build timed out or failed: %w", err)
		}

		// 3. Get project status
		status, err := apiClient.GetProjectStatus(ctx, projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(map[string]interface{}{
			"projectId": projectID,
			"urls":      status.URLs,
			"project":   status.Project,
		}, func() {
			output.Success("Build complete!")
			output.Info("Project ID: %s", projectID)
			if draft, ok := status.URLs["draft"]; ok && draft != "" {
				output.Info("Draft URL:  %s", draft)
			}
			if preview, ok := status.URLs["preview"]; ok && preview != "" {
				output.Info("Preview:    %s", preview)
			}
		})
		return nil
	},
}

func init() {
	buildCmd.Flags().StringP("message", "m", "", "What to build (required)")
	buildCmd.Flags().Bool("public", true, "Make project public")
	buildCmd.Flags().Bool("stdin", false, "Read message from stdin")
	buildCmd.Flags().String("mode", "full", "Generation mode: full, policy, ui,policy, backend,policy")
}
