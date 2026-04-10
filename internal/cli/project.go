package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	Example: `  poof project list
  poof project list --limit 20 --offset 10
  poof project list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		resp, err := apiClient.ListProjects(context.Background(), limit, offset)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Projects) == 0 {
				output.Info("No projects found.")
				return
			}
			rows := make([][]string, len(resp.Projects))
			for i, p := range resp.Projects {
				rows[i] = []string{p.ID, p.Title, p.Slug}
			}
			output.Table([]string{"ID", "Title", "Slug"}, rows)
			if resp.HasMore {
				output.Info("(more projects available — use --offset %d to see next page)", offset+limit)
			}
		})
		return nil
	},
}

var projectCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project",
	Example: `  poof project create -m "Build a token-gated voting app"
  poof project create -m "NFT marketplace" --mode policy
  poof project create -m "Staking dashboard" --public=false
  echo "Build a chat app" | poof project create --stdin`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		isPublic, _ := cmd.Flags().GetBool("public")
		mode, _ := cmd.Flags().GetString("mode")

		if useStdin {
			message = readStdin()
		}
		if message == "" {
			return fmt.Errorf("--message is required\n  poof project create -m \"Build a todo app\"")
		}

		if err := validateMode(mode); err != nil {
			return err
		}

		req := api.CreateProjectRequest{
			FirstMessage:   message,
			IsPublic:       isPublic,
			GenerationMode: mode,
		}

		resp, err := apiClient.CreateProject(context.Background(), req)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Success("Project created: %s", resp.ProjectID)
		})
		return nil
	},
}

var projectUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update project metadata",
	Example: `  poof project update -p <id> --title "My App"
  poof project update -p <id> --slug my-app --description "A cool app"
  poof project update -p <id> --generation-mode policy
  poof project update -p <id> --public=false --network mainnet-beta`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		req := api.UpdateProjectRequest{}
		if v, _ := cmd.Flags().GetString("title"); v != "" {
			req.Title = v
		}
		if v, _ := cmd.Flags().GetString("description"); v != "" {
			req.Description = v
		}
		if v, _ := cmd.Flags().GetString("slug"); v != "" {
			req.Slug = v
		}
		if cmd.Flags().Changed("public") {
			v, _ := cmd.Flags().GetBool("public")
			req.IsPublic = &v
		}
		if v, _ := cmd.Flags().GetString("generation-mode"); v != "" {
			req.GenerationMode = v
		}
		if v, _ := cmd.Flags().GetString("network"); v != "" {
			req.Network = v
		}

		if err := apiClient.UpdateProject(context.Background(), projectID, &req); err != nil {
			return handleError(err)
		}

		output.Print(map[string]interface{}{
			"success":   true,
			"projectId": projectID,
		}, func() {
			output.Success("Project %s updated.", projectID)
		})
		return nil
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a project (irreversible)",
	Example: `  poof project delete -p <id> --yes
  poof project delete -p <id> --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			output.Info("Would delete project %s. No changes made.", projectID)
			return nil
		}

		if !yes {
			return fmt.Errorf("deleting project %s is irreversible. Pass --yes to confirm\n  poof project delete -p %s --yes", projectID, projectID)
		}

		if err := apiClient.DeleteProject(context.Background(), projectID); err != nil {
			return handleError(err)
		}

		output.Print(map[string]interface{}{
			"success":   true,
			"projectId": projectID,
		}, func() {
			output.Success("Project %s deleted.", projectID)
		})
		return nil
	},
}

var projectStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get project status, URLs, and deployment info",
	Example: `  poof project status -p <id>
  poof project status -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetProjectStatus(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Info("Project: %s", resp.Project.Title)
			output.Info("ID:      %s", resp.Project.ID)
			draftFlag := resp.IsTargetDeployed("draft")
			previewFlag := resp.IsTargetDeployed("preview")
			liveFlag := resp.IsTargetDeployed("live")
			if draft, ok := resp.URLs["draft"]; ok && draft != "" {
				output.Info("Draft:   %s %s", draft, deployedMarker(draftFlag))
			}
			if preview, ok := resp.URLs["mainnetPreview"]; ok && preview != "" {
				output.Info("Preview: %s %s", preview, deployedMarker(previewFlag))
			}
			if prod, ok := resp.URLs["production"]; ok && prod != "" {
				output.Info("Prod:    %s %s", prod, deployedMarker(liveFlag))
			}
			output.Info("Deploy flags: draft=%v preview=%v live=%v", draftFlag, previewFlag, liveFlag)
		})
		return nil
	},
}

// deployedMarker returns a compact tag used in text output to make it
// obvious whether a target has actually been deployed yet.
func deployedMarker(deployed bool) string {
	if deployed {
		return "(deployed)"
	}
	return "(not deployed)"
}

var projectMessagesCmd = &cobra.Command{
	Use:   "messages",
	Short: "Get conversation history",
	Example: `  poof project messages -p <id>
  poof project messages -p <id> --limit 100
  poof project messages -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		resp, err := apiClient.GetMessages(context.Background(), projectID, limit, offset)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			for _, m := range resp.Messages {
				role := m.Role
				if role == "assistant" {
					role = "AI"
				}
				output.Info("[%s] %s: %s", m.Status, role, truncate(m.Content, 200))
			}
			if resp.HasMore {
				output.Info("(more messages available — use --offset %d to see next page)", offset+limit)
			}
		})
		return nil
	},
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// readStdin reads all of stdin into a string (for piped input).
// Returns empty string if stdin is a terminal (not piped).
func readStdin() string {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return ""
	}
	scanner := bufio.NewScanner(os.Stdin)
	var result string
	for scanner.Scan() {
		if result != "" {
			result += "\n"
		}
		result += scanner.Text()
	}
	if scanner.Err() != nil {
		fmt.Fprintf(os.Stderr, "Warning: error reading stdin: %v\n", scanner.Err())
		return ""
	}
	return result
}

func init() {
	projectListCmd.Flags().Int("limit", 10, "Max projects to return")
	projectListCmd.Flags().Int("offset", 0, "Offset for pagination")

	projectCreateCmd.Flags().StringP("message", "m", "", "First message describing what to build (required)")
	projectCreateCmd.Flags().Bool("public", true, "Make project public")
	projectCreateCmd.Flags().Bool("stdin", false, "Read message from stdin")
	projectCreateCmd.Flags().String("mode", "full", "Generation mode: full, policy, ui,policy, backend,policy")

	projectUpdateCmd.Flags().String("title", "", "New title")
	projectUpdateCmd.Flags().String("description", "", "New description")
	projectUpdateCmd.Flags().String("slug", "", "New slug")
	projectUpdateCmd.Flags().Bool("public", true, "Project visibility")
	projectUpdateCmd.Flags().String("generation-mode", "", "Generation mode: full, policy, ui,policy, backend,policy")
	projectUpdateCmd.Flags().String("network", "", "Solana network")

	projectDeleteCmd.Flags().Bool("dry-run", false, "Preview what would be deleted without making changes")
	projectDeleteCmd.Flags().Bool("yes", false, "Skip confirmation (required for delete)")

	projectMessagesCmd.Flags().Int("limit", 50, "Max messages to return (1-200)")
	projectMessagesCmd.Flags().Int("offset", 0, "Offset for pagination")

	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectUpdateCmd)
	projectCmd.AddCommand(projectDeleteCmd)
	projectCmd.AddCommand(projectStatusCmd)
	projectCmd.AddCommand(projectMessagesCmd)
}
