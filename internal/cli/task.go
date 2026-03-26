package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "View tasks and test results",
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks (builds, deployments, downloads)",
	Example: `  poof task list -p <id>
  poof task list -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.ListTasks(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Tasks) == 0 {
				output.Info("No tasks found.")
				return
			}
			rows := make([][]string, len(resp.Tasks))
			for i, t := range resp.Tasks {
				id, _ := t["id"].(string)
				status, _ := t["status"].(string)
				title, _ := t["title"].(string)
				rows[i] = []string{id, status, title}
			}
			output.Table([]string{"ID", "Status", "Title"}, rows)
		})
		return nil
	},
}

var taskGetCmd = &cobra.Command{
	Use:   "get [taskId]",
	Short: "Get task details",
	Example: `  poof task get <taskId> -p <id>`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetTask(context.Background(), projectID, args[0])
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Info("Task:   %s", resp.ID)
			output.Info("Status: %s", resp.Status)
			output.Info("Title:  %s", resp.Title)
		})
		return nil
	},
}

var taskTestResultsCmd = &cobra.Command{
	Use:   "test-results",
	Short: "Get structured test results",
	Example: `  poof task test-results -p <id>
  poof task test-results -p <id> --json | jq '.summary'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetTestResults(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Info("Tests: %d total, %d passed, %d failed, %d errors",
				resp.Summary.Total, resp.Summary.Passed, resp.Summary.Failed, resp.Summary.Errors)

			if resp.Summary.Failed > 0 || resp.Summary.Errors > 0 {
				fmt.Println()
				for _, r := range resp.Results {
					if r.Status == "failed" || r.Status == "error" {
						output.Error("  %s: %s", r.FileName, r.LastError)
					}
				}
			}
		})
		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskGetCmd)
	taskCmd.AddCommand(taskTestResultsCmd)
}
