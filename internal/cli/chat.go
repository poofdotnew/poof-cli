package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

// Supported image MIME types
var imageExtToMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

const maxImageSizeMB = 3.4

// uploadAndPrepareFiles uploads image files and returns the message suffix and CDN URLs.
func uploadAndPrepareFiles(ctx context.Context, projectID string, filePaths []string) (string, []string, error) {
	return uploadFilesWith(ctx, filePaths, func(ctx context.Context, encoded, contentType, fileName string) (string, error) {
		resp, err := apiClient.UploadImage(ctx, projectID, encoded, contentType, fileName)
		if err != nil {
			return "", err
		}
		return resp.URL, nil
	})
}

// uploadAndPrepareFilesGlobal uploads image files to the global (non-project)
// Tarobase app. Used by `poof build --file`, where the project doesn't exist
// yet so the project-scoped upload endpoint can't be used.
func uploadAndPrepareFilesGlobal(ctx context.Context, filePaths []string) (string, []string, error) {
	return uploadFilesWith(ctx, filePaths, func(ctx context.Context, encoded, contentType, fileName string) (string, error) {
		resp, err := apiClient.UploadImageGlobal(ctx, encoded, contentType, fileName)
		if err != nil {
			return "", err
		}
		return resp.URL, nil
	})
}

func uploadFilesWith(ctx context.Context, filePaths []string, upload func(ctx context.Context, encoded, contentType, fileName string) (string, error)) (string, []string, error) {
	var messageAppend string
	var urls []string

	for _, fp := range filePaths {
		ext := strings.ToLower(filepath.Ext(fp))
		contentType, ok := imageExtToMIME[ext]
		if !ok {
			return "", nil, fmt.Errorf("unsupported image type %q (supported: .png, .jpg, .jpeg, .gif, .webp)", ext)
		}

		data, err := os.ReadFile(fp)
		if err != nil {
			return "", nil, fmt.Errorf("failed to read %s: %w", fp, err)
		}

		sizeLimit := maxImageSizeMB
		maxBytes := int(sizeLimit * 1024 * 1024)
		if len(data) > maxBytes {
			return "", nil, fmt.Errorf("%s exceeds %.1fMB limit (%d bytes)", fp, maxImageSizeMB, len(data))
		}

		encoded := base64.StdEncoding.EncodeToString(data)
		fileName := filepath.Base(fp)

		output.Info("Uploading %s...", fileName)
		url, err := upload(ctx, encoded, contentType, fileName)
		if err != nil {
			return "", nil, fmt.Errorf("failed to upload %s: %w", fileName, err)
		}

		urls = append(urls, url)
		messageAppend += fmt.Sprintf(` <userUploadedFile type="image">%s</userUploadedFile>`, url)
	}

	return messageAppend, urls, nil
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Chat with the AI builder",
}

var chatSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to the AI",
	Example: `  poof chat send -p <id> -m "Add a leaderboard page"
  poof chat send -p <id> -m "Fix the login button"
  echo "Add dark mode" | poof chat send -p <id> --stdin`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		filePaths, _ := cmd.Flags().GetStringSlice("file")

		if useStdin {
			message = readStdin()
		}
		if message == "" && len(filePaths) == 0 {
			return fmt.Errorf("--message is required\n  poof chat send -p %s -m \"Add a feature\"", projectID)
		}

		ctx := context.Background()
		var attachedFiles []string

		if len(filePaths) > 0 {
			suffix, urls, err := uploadAndPrepareFiles(ctx, projectID, filePaths)
			if err != nil {
				return err
			}
			message += suffix
			attachedFiles = urls
		}

		resp, err := apiClient.Chat(ctx, projectID, message, attachedFiles)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Success("Message sent. AI is building...")
			output.Info("Message ID: %s", resp.MessageID)
		})
		return nil
	},
}

var chatActiveCmd = &cobra.Command{
	Use:   "active",
	Short: "Check if AI is currently processing",
	Example: `  poof chat active -p <id>
  poof chat active -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.CheckAIActive(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			state := resp.State
			if state == "" {
				if resp.Active {
					state = "running"
				} else {
					state = "idle"
				}
			}
			if resp.Active {
				output.Info("AI is active (state: %s, status: %s)", state, resp.Status)
			} else {
				output.Info("AI is idle (state: %s, status: %s)", state, resp.Status)
			}
		})
		return nil
	},
}

var chatCancelCmd = &cobra.Command{
	Use:     "cancel",
	Short:   "Cancel in-progress AI execution",
	Example: `  poof chat cancel -p <id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		if err := apiClient.CancelAI(context.Background(), projectID); err != nil {
			return handleError(err)
		}

		output.Print(map[string]bool{"success": true}, func() {
			output.Success("AI execution canceled.")
		})
		return nil
	},
}

var chatClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear saved AI session context",
	Example: `  poof chat clear -p <id>
  poof chat clear -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.ClearAISession(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.Message != "" {
				output.Success(resp.Message)
				return
			}
			output.Success("Session cleared.")
		})
		return nil
	},
}

var chatSteerCmd = &cobra.Command{
	Use:   "steer",
	Short: "Redirect AI mid-execution without canceling",
	Example: `  poof chat steer -p <id> -m "Focus on the backend first"
  poof chat steer -p <id> -m "Skip the UI, just do policies"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")

		if useStdin {
			message = readStdin()
		}
		if message == "" {
			return fmt.Errorf("--message is required\n  poof chat steer -p %s -m \"Focus on backend\"", projectID)
		}

		if err := apiClient.SteerAI(context.Background(), projectID, message, ""); err != nil {
			return handleError(err)
		}

		output.Print(map[string]bool{"success": true}, func() {
			output.Success("Steering message sent.")
		})
		return nil
	},
}

func init() {
	chatSendCmd.Flags().StringP("message", "m", "", "Message to send (required)")
	chatSendCmd.Flags().Bool("stdin", false, "Read message from stdin")
	chatSendCmd.Flags().StringSlice("file", nil, "Image file(s) to attach (PNG, JPEG, GIF, WebP, max 3.4MB each)")
	chatSteerCmd.Flags().StringP("message", "m", "", "Steering message (required)")
	chatSteerCmd.Flags().Bool("stdin", false, "Read message from stdin")

	chatCmd.AddCommand(chatSendCmd)
	chatCmd.AddCommand(chatActiveCmd)
	chatCmd.AddCommand(chatCancelCmd)
	chatCmd.AddCommand(chatClearCmd)
	chatCmd.AddCommand(chatSteerCmd)
}
