package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/poll"
	"github.com/spf13/cobra"
)

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security tools",
}

var securityScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Initiate a security audit",
	Long: `Initiate a security scan. By default this returns as soon as the scan is
queued (so agents can do other work and poll later). Pass --wait to block
until the scan finishes and surface the findings inline.`,
	Example: `  poof security scan -p <id>
  poof security scan -p <id> --wait
  poof security scan -p <id> --task <taskId>
  poof security scan -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		taskID, _ := cmd.Flags().GetString("task")
		wait, _ := cmd.Flags().GetBool("wait")

		ctx := context.Background()

		var resp *api.SecurityScanResponse
		err = output.WithSpinner("Initiating security scan...", func() error {
			var scanErr error
			resp, scanErr = apiClient.SecurityScan(ctx, projectID, taskID)
			return scanErr
		})
		if err != nil {
			return handleError(err)
		}

		if !wait || resp.ScanID == "" {
			output.Print(resp, func() {
				output.Success("Security scan initiated.")
				if resp.ScanID != "" {
					output.Info("  Scan:     %s", resp.ScanID)
				}
				if resp.TaskID != "" {
					output.Info("  Task:     %s", resp.TaskID)
				}
				if resp.TaskTitle != "" {
					output.Info("  Scanning: %s", resp.TaskTitle)
				}
				if resp.Message != "" {
					output.Info("  Message:  %s", resp.Message)
				}
			})
			return nil
		}

		// --wait mode: poll the scan record until it completes, then surface
		// findings. Mirrors the polling loop inside `poof ship`.
		var scan *api.SecurityScanStatus
		err = output.WithSpinner("Waiting for security scan to complete...", func() error {
			scanPollCfg := poll.Config{
				InitialDelay:      3 * time.Second,
				MaxDelay:          10 * time.Second,
				BackoffFactor:     1.3,
				Timeout:           10 * time.Minute,
				MaxConsecutiveErr: 5,
			}
			return poll.Poll(ctx, scanPollCfg, func(ctx context.Context) (bool, error) {
				s, err := apiClient.GetSecurityScan(ctx, projectID, resp.ScanID)
				if err != nil {
					return false, err
				}
				scan = s
				switch s.Status {
				case "completed":
					return true, nil
				case "failed":
					msg := s.ErrorMessage
					if msg == "" {
						msg = "security scan failed"
					}
					return false, fmt.Errorf("security scan failed: %s", msg)
				default:
					return false, nil
				}
			})
		})
		if err != nil {
			return fmt.Errorf("security scan failed: %w", err)
		}

		blocking := scan != nil && (scan.CriticalSeverity > 0 || scan.HighSeverity > 0)

		type scanResult struct {
			ScanID   string                    `json:"scanId"`
			Blocking bool                      `json:"blocking"`
			Task     *api.SecurityScanResponse `json:"initiation,omitempty"`
			Scan     *api.SecurityScanStatus   `json:"scan,omitempty"`
		}
		result := &scanResult{ScanID: resp.ScanID, Blocking: blocking, Task: resp, Scan: scan}

		output.Print(result, func() {
			if scan == nil {
				output.Warn("Scan completed but no status record returned.")
				return
			}
			total := scan.TotalFindings
			switch {
			case blocking:
				output.Warn("Scan completed with blocking findings: %d critical, %d high (of %d total).",
					scan.CriticalSeverity, scan.HighSeverity, total)
			case total > 0:
				output.Warn("Scan completed with non-blocking findings: %d medium, %d low (of %d total).",
					scan.MediumSeverity, scan.LowSeverity, total)
			default:
				output.Success("Scan completed. No findings.")
			}
			output.Info("  Scan ID:  %s", scan.ID)
			if resp.TaskTitle != "" {
				output.Info("  Target:   %s", resp.TaskTitle)
			}
			// Surface blocking findings inline so agents can fix them
			// without a second API roundtrip. Only show critical/high by
			// default — medium/low go in the JSON but not the text view.
			for _, f := range scan.Findings {
				sev := strings.ToLower(f.Severity)
				if sev != "critical" && sev != "high" {
					continue
				}
				title := f.Title
				if title == "" {
					title = f.Category
				}
				if title == "" {
					title = "(no title)"
				}
				if f.File != "" {
					output.Warn("  [%s] %s — %s", strings.ToUpper(sev), title, f.File)
				} else {
					output.Warn("  [%s] %s", strings.ToUpper(sev), title)
				}
				if f.Description != "" {
					output.Info("    %s", truncate(f.Description, 240))
				}
			}
		})

		// Exit non-zero on blocking findings so agents can gate deploys on
		// the exit code. Medium/low findings don't block deploys server-side,
		// so we stay silent on exit code for those.
		if blocking {
			return fmt.Errorf("security scan blocked by %d critical + %d high findings",
				scan.CriticalSeverity, scan.HighSeverity)
		}
		return nil
	},
}

func init() {
	securityScanCmd.Flags().String("task", "", "Task ID to check status of a previous scan")
	securityScanCmd.Flags().Bool("wait", false, "Block until the scan finishes and surface findings inline")
	securityCmd.AddCommand(securityScanCmd)
}
