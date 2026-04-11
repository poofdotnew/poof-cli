package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var filesCmd = &cobra.Command{
	Use:   "files",
	Short: "Manage project files",
}

var filesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get project source files (requires credit purchase)",
	Long: `Fetch project source files.

By default this returns the full project. Use --path to filter to a single
file or glob (e.g. --path "src/**/*.tsx"), --list to show only file paths
without contents, or --stat to show path + byte count per file. Agents
should prefer --list or --path over a full dump to keep output small.`,
	Example: `  poof files get -p <id> --list
  poof files get -p <id> --path "UI/components/HomePage.tsx"
  poof files get -p <id> --path "lifecycle-actions/*.json" --json
  poof files get -p <id> --stat | head -20`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		pathFilter, _ := cmd.Flags().GetString("path")
		listOnly, _ := cmd.Flags().GetBool("list")
		statMode, _ := cmd.Flags().GetBool("stat")

		resp, err := apiClient.GetFiles(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		// Apply path glob filter if requested. Use doublestar semantics via
		// filepath.Match with manual ** support: first try exact match, then
		// filepath.Match, then a simple ** prefix/suffix expansion.
		filtered := resp.Files
		if pathFilter != "" {
			filtered = filterFilesByGlob(resp.Files, pathFilter)
			if len(filtered) == 0 {
				return fmt.Errorf("no files matched --path %q (total files in project: %d)", pathFilter, len(resp.Files))
			}
		}

		if listOnly {
			type listResp struct {
				Files []string `json:"files"`
				Total int      `json:"total"`
			}
			paths := make([]string, 0, len(filtered))
			for p := range filtered {
				paths = append(paths, p)
			}
			sort.Strings(paths)
			view := &listResp{Files: paths, Total: len(paths)}
			output.Print(view, func() {
				for _, p := range paths {
					output.Info("%s", p)
				}
				output.Info("\n%d file(s)", len(paths))
			})
			return nil
		}

		if statMode {
			type statEntry struct {
				Path  string `json:"path"`
				Bytes int    `json:"bytes"`
			}
			type statResp struct {
				Files []statEntry `json:"files"`
				Total int         `json:"total"`
				Bytes int         `json:"bytes"`
			}
			entries := make([]statEntry, 0, len(filtered))
			totalBytes := 0
			for p, c := range filtered {
				entries = append(entries, statEntry{Path: p, Bytes: len(c)})
				totalBytes += len(c)
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
			view := &statResp{Files: entries, Total: len(entries), Bytes: totalBytes}
			output.Print(view, func() {
				for _, e := range entries {
					output.Info("%8d  %s", e.Bytes, e.Path)
				}
				output.Info("\n%d file(s), %d bytes total", len(entries), totalBytes)
			})
			return nil
		}

		view := &api.FilesResponse{Files: filtered}
		output.Print(view, func() {
			for path := range filtered {
				output.Info("%s", path)
			}
			output.Info("\n%d file(s) total", len(filtered))
		})
		return nil
	},
}

// filterFilesByGlob matches file paths against a pattern. Supports:
//   - exact match: "src/config.ts"
//   - bare filename: "HomePage.tsx" (matches any directory)
//   - single-level glob: "src/*.ts" (no slashes in the * segment)
//   - doublestar: "**/*.tsx", "src/**/*.tsx"
//
// The pattern is translated to a regex for doublestar support so we don't
// hand-roll matching logic.
func filterFilesByGlob(files map[string]string, pattern string) map[string]string {
	out := map[string]string{}
	if content, ok := files[pattern]; ok {
		out[pattern] = content
		return out
	}
	re, err := globToRegex(pattern)
	if err != nil {
		return out
	}
	baseOnly := !strings.ContainsAny(pattern, "/*?[")
	for path, content := range files {
		if re.MatchString(path) {
			out[path] = content
			continue
		}
		if baseOnly && filepath.Base(path) == pattern {
			out[path] = content
		}
	}
	return out
}

// globToRegex converts a glob pattern into a Go regular expression. Handles
// **, *, ?, and escapes regex metacharacters. Anchored at both ends.
func globToRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// ** matches across path separators
				b.WriteString(".*")
				i++
				// Skip a following slash if present so "**/foo" matches "foo"
				// directly (no leading dir).
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
				}
			} else {
				// single * matches within one path segment
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '{', '}', '|', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
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
	filesGetCmd.Flags().String("path", "", "Glob pattern filter (e.g. \"src/**/*.tsx\" or \"HomePage.tsx\")")
	filesGetCmd.Flags().Bool("list", false, "List file paths only (no contents)")
	filesGetCmd.Flags().Bool("stat", false, "Show byte counts per file (no contents)")

	filesUpdateCmd.Flags().String("from-json", "", "JSON file mapping paths to contents")
	filesUpdateCmd.Flags().String("file", "", "Single file path to update")
	filesUpdateCmd.Flags().String("content", "", "Content for the single file")

	filesUploadCmd.Flags().String("file", "", "Path to image file (PNG, JPEG, GIF, WebP)")

	filesCmd.AddCommand(filesGetCmd)
	filesCmd.AddCommand(filesUpdateCmd)
	filesCmd.AddCommand(filesUploadCmd)
}
