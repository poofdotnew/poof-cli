package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

// CLI mirror of the in-app Usage panel + /api/project/{id}/infra-usage.
// Owner-and-collaborator read; owner-only mutations.

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Read project compute usage and manage overuse limits",
}

var usageStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show this month's compute / storage / cost summary",
	Long: `Current-month requests / CPU / storage / cost, free vs paid breakdown,
overuse limit, pause state. Honour summaryStale / blockedStatusStale —
when true, the corresponding fields are best-effort and shouldn't be
acted on.`,
	Example: `  poof usage status -p <id>
  poof usage status -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetInfraUsage(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Info("Usage — %s — %s", projectID, resp.Period)
			if resp.SummaryStale {
				output.Warn("Summary fields are stale (upstream usage pipeline failed) — values may be zero placeholders.")
			}
			if resp.BlockedStatusStale {
				output.Warn("Blocked-status check failed — isBlocked / canResume / blockedReason should not be trusted on this response.")
			}

			output.Info("  Cost this month: %.4g credits (free %.4g, paid %.4g)",
				resp.CostCredits, resp.FreeCreditsApplied, resp.ChargedCredits,
			)
			output.Info("  Free tier used:  %.1f%%", resp.PercentUsed)
			if resp.InfraOveruseLimit != nil {
				output.Info("  Overuse limit:   %.4g credits", *resp.InfraOveruseLimit)
			} else {
				output.Info("  Overuse limit:   not set (app pauses at free-tier exhaustion)")
			}
			output.Info("  Available paid:  %.4g credits (personal balance + project bank when not isolated)",
				resp.PaidCreditsRemaining,
			)
			output.Info("  Status:          %s", resp.Status)

			if resp.IsBlocked {
				reason := string(resp.BlockedReason)
				if reason == "" {
					reason = "(reason unavailable)"
				}
				resumable := "owner action required"
				if resp.CanResume {
					resumable = "resumable — run `poof usage resume -p <id>` once you've fixed the cause"
				}
				output.Warn("App is paused: %s — %s", reason, resumable)
			}

			output.Info("  Requests:        %d", resp.TotalRequests)
			output.Info("  CPU time:        %d ms", resp.TotalCpuTimeMs)
			output.Info("  Wall time:       %d ms", resp.TotalWallTimeMs)
			output.Info("  Storage:         %d bytes (%d docs, %d files)",
				resp.TotalStorageBytes, resp.TotalDocumentCount, resp.TotalFileCount,
			)

			if resp.LastUpdated != nil && *resp.LastUpdated != "" {
				output.Info("  Last updated:    %s", *resp.LastUpdated)
			}
		})
		return nil
	},
}

var usageLimitCmd = &cobra.Command{
	Use:   "limit",
	Short: "Set or clear the monthly overuse limit",
	Long: `Credit ceiling beyond the free tier. Without a limit, the app pauses at
free-tier exhaustion. With one, paid credits cover the overage (project
bank first, then personal balance, subject to isolation).`,
	Example: `  poof usage limit -p <id> --credits 50
  poof usage limit -p <id> --clear`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		clear, _ := cmd.Flags().GetBool("clear")
		creditsSet := cmd.Flags().Changed("credits")
		if clear && creditsSet {
			return fmt.Errorf("pass either --credits N or --clear, not both")
		}
		if !clear && !creditsSet {
			return fmt.Errorf("pass --credits N to set, or --clear to remove the limit")
		}

		var limit *float64
		if !clear {
			v, err := cmd.Flags().GetFloat64("credits")
			if err != nil {
				return fmt.Errorf("--credits must be a number: %w", err)
			}
			if !(v > 0) {
				return fmt.Errorf("--credits must be > 0 (use --clear to remove the limit)")
			}
			limit = &v
		}

		if err := apiClient.SetInfraOveruseLimit(context.Background(), projectID, limit); err != nil {
			return handleError(err)
		}

		// Re-read so the user sees the new limit. As with the isolation
		// command, a read failure here doesn't roll back the PUT — surface
		// the warning but treat the command as successful.
		state, readErr := apiClient.GetInfraUsage(context.Background(), projectID)
		if readErr != nil {
			output.Print(map[string]interface{}{
				"success":      true,
				"projectId":    projectID,
				"clear":        clear,
				"credits":      limit,
				"warning":      "limit updated, but failed to re-read state",
				"warningError": readErr.Error(),
			}, func() {
				output.Success("Overuse limit updated for %s.", projectID)
				output.Warn("Couldn't re-read state: %v. Run `poof usage status -p %s` to confirm.", readErr, projectID)
			})
			return nil
		}

		output.Print(state, func() {
			if clear {
				output.Success("Overuse limit cleared for %s.", projectID)
			} else {
				output.Success("Overuse limit set to %.4g credits for %s.", *limit, projectID)
			}
			if state.InfraOveruseLimit != nil {
				output.Info("  Current limit: %.4g credits", *state.InfraOveruseLimit)
			} else {
				output.Info("  Current limit: not set")
			}
		})
		return nil
	},
}

var usageResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a paused project (owner-only)",
	Long: `Resume only when preconditions are met (limit set, threshold not
reached, paid credits available). Otherwise the server returns 400 with
the typed blockedReason — fix the cause and retry.`,
	Example: `  poof usage resume -p <id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.ResumeProject(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.Resumed {
				output.Success("Project %s resumed.", projectID)
			} else {
				output.Warn("Server reported resumed=false for %s.", projectID)
			}
		})
		return nil
	},
}

func init() {
	usageLimitCmd.Flags().Float64("credits", 0, "New overuse limit, in credits (must be > 0)")
	usageLimitCmd.Flags().Bool("clear", false, "Remove the overuse limit (app will pause at free-tier exhaustion)")

	usageCmd.AddCommand(usageStatusCmd)
	usageCmd.AddCommand(usageLimitCmd)
	usageCmd.AddCommand(usageResumeCmd)
}
