package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var filesCmd = &cobra.Command{
	Use:   "files",
	Short: "Manage project files",
}

var filesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get all source files (requires credit purchase)",
	Example: `  poof files get -p <id>
  poof files get -p <id> --json | jq 'keys'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetFiles(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			for path := range resp.Files {
				output.Info("%s", path)
			}
			output.Info("\n%d files total", len(resp.Files))
		})
		return nil
	},
}

var filesUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update project files from a JSON file",
	Long:  "Update project files. Pass a JSON file mapping paths to contents, or use --file and --content for a single file.",
	Example: `  poof files update -p <id> --file src/config.ts --content "export const X = 1;"
  poof files update -p <id> --from-json files.json
  cat files.json | poof files update -p <id> --from-json /dev/stdin`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		var files map[string]string

		jsonFile, _ := cmd.Flags().GetString("from-json")
		filePath, _ := cmd.Flags().GetString("file")
		content, _ := cmd.Flags().GetString("content")

		if jsonFile != "" {
			data, err := os.ReadFile(jsonFile)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", jsonFile, err)
			}
			if err := json.Unmarshal(data, &files); err != nil {
				return fmt.Errorf("invalid JSON in %s: %w", jsonFile, err)
			}
		} else if filePath != "" && content != "" {
			files = map[string]string{filePath: content}
		} else {
			return fmt.Errorf("use --from-json <file> or --file <path> --content <text>")
		}

		if err := apiClient.UpdateFiles(context.Background(), projectID, files); err != nil {
			return handleError(err)
		}

		output.Print(map[string]interface{}{
			"success": true,
			"count":   len(files),
		}, func() {
			output.Success("Updated %d file(s).", len(files))
		})
		return nil
	},
}

func init() {
	filesUpdateCmd.Flags().String("from-json", "", "JSON file mapping paths to contents")
	filesUpdateCmd.Flags().String("file", "", "Single file path to update")
	filesUpdateCmd.Flags().String("content", "", "Content for the single file")

	filesCmd.AddCommand(filesGetCmd)
	filesCmd.AddCommand(filesUpdateCmd)
}
