package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

var filesUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload an image to project storage",
	Example: `  poof files upload -p <id> --file logo.png
  poof files upload -p <id> --file screenshot.jpg --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			return fmt.Errorf("--file is required\n  poof files upload -p %s --file image.png", projectID)
		}

		ext := strings.ToLower(filepath.Ext(filePath))
		contentType, ok := imageExtToMIME[ext]
		if !ok {
			return fmt.Errorf("unsupported image type %q (supported: .png, .jpg, .jpeg, .gif, .webp)", ext)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		sizeLimitMB := maxImageSizeMB
		maxBytes := int(sizeLimitMB * 1024 * 1024)
		if len(data) > maxBytes {
			return fmt.Errorf("%s exceeds %.1fMB limit (%d bytes)", filePath, maxImageSizeMB, len(data))
		}

		encoded := base64.StdEncoding.EncodeToString(data)
		fileName := filepath.Base(filePath)

		resp, err := apiClient.UploadImage(context.Background(), projectID, encoded, contentType, fileName)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Success("Uploaded: %s", resp.URL)
		})
		return nil
	},
}

func init() {
	filesUpdateCmd.Flags().String("from-json", "", "JSON file mapping paths to contents")
	filesUpdateCmd.Flags().String("file", "", "Single file path to update")
	filesUpdateCmd.Flags().String("content", "", "Content for the single file")

	filesUploadCmd.Flags().String("file", "", "Path to image file (PNG, JPEG, GIF, WebP)")

	filesCmd.AddCommand(filesGetCmd)
	filesCmd.AddCommand(filesUpdateCmd)
	filesCmd.AddCommand(filesUploadCmd)
}
