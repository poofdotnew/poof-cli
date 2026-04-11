package cli

import (
	"context"
	"fmt"
	"time"

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

		if err := validateMode(mode); err != nil {
			return err
		}

		ctx := context.Background()

		// 1. Create project
		if output.GetFormat() == output.FormatText {
			output.Info("Creating project...")
		}
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
		if output.GetFormat() == output.FormatText {
			output.Success("Project created: %s", projectID)
		}

		// 2. Poll until AI finishes
		// Track whether we've seen the AI become active to avoid declaring
		// "done" before it has started (race between project creation and
		// the server activating the AI).
		seenActive := false
		pollStart := time.Now()
		const activationGrace = 30 * time.Second

		err = output.WithSpinner("AI is building...", func() error {
			return poll.Poll(ctx, poll.LongAIConfig(), func(ctx context.Context) (bool, error) {
				status, err := apiClient.CheckAIActive(ctx, projectID)
				if err != nil {
					return false, err
				}
				if status.Status == "error" {
					return false, fmt.Errorf("AI processing failed with error status")
				}
				if status.Active {
					seenActive = true
					return false, nil
				}
				// AI is not active — only consider done if we've seen it
				// active at least once, or the grace period has elapsed
				// (handles the unlikely case where AI starts and finishes
				// before our first poll).
				if seenActive || time.Since(pollStart) > activationGrace {
					return true, nil
				}
				return false, nil
			})
		})
		if err != nil {
			if output.GetFormat() == output.FormatText {
				output.Info("Project ID: %s (you can check status with 'poof project status -p %s')", projectID, projectID)
			}
			return fmt.Errorf("build timed out or failed: %w", err)
		}

		// 3. Wait for draft deploy to become ready (up to 3 minutes)
		status, err := apiClient.GetProjectStatus(ctx, projectID)
		if err != nil {
			return handleError(err)
		}

		if !status.IsTargetDeployed("draft") {
			err = output.WithSpinner("Waiting for draft deploy...", func() error {
				return poll.Poll(ctx, poll.Config{
					InitialDelay:      5 * time.Second,
					MaxDelay:          10 * time.Second,
					BackoffFactor:     1.0,
					Timeout:           3 * time.Minute,
					MaxConsecutiveErr: 5,
				}, func(ctx context.Context) (bool, error) {
					s, err := apiClient.GetProjectStatus(ctx, projectID)
					if err != nil {
						return false, err
					}
					if s.IsTargetDeployed("draft") {
						status = s
						return true, nil
					}
					return false, nil
				})
			})
			if err != nil {
				// Not fatal — build succeeded, draft just isn't ready yet
				status, _ = apiClient.GetProjectStatus(ctx, projectID)
			}
		}

		// 4. Report results
		type buildResult struct {
			ProjectID    string                 `json:"projectId"`
			URLs         map[string]string      `json:"urls"`
			Project      api.Project            `json:"project"`
			PublishState map[string]interface{} `json:"publishState,omitempty"`
			DraftReady   bool                   `json:"draftReady"`
		}
		result := &buildResult{
			ProjectID:    projectID,
			URLs:         status.URLs,
			Project:      status.Project,
			PublishState: status.PublishState,
			DraftReady:   status.IsTargetDeployed("draft"),
		}

		if output.GetFormat() == output.FormatQuiet {
			output.Quiet(projectID)
		} else {
			output.Print(result, func() {
				if result.DraftReady {
					output.Success("Build complete!")
				} else {
					output.Warn("Build finished, but the draft deploy is not ready yet.")
				}
				output.Info("Project ID: %s", projectID)
				if draft, ok := status.URLs["draft"]; ok && draft != "" {
					output.Info("Draft: %s", draft)
				}
				if !result.DraftReady {
					output.Info("Draft deploy state is still pending. Re-check with 'poof project status -p %s' before treating the draft URL as live.", projectID)
				}
			})
		}
		return nil
	},
}

func init() {
	buildCmd.Flags().StringP("message", "m", "", "What to build (required)")
	buildCmd.Flags().Bool("public", true, "Make project public")
	buildCmd.Flags().Bool("stdin", false, "Read message from stdin")
	buildCmd.Flags().String("mode", "full", "Generation mode: full, policy, ui,policy, backend,policy")
}
