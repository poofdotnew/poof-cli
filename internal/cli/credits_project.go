package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

// `poof credits project` — manage the per-project credit bank (deposit,
// withdraw, isolation). User-level commands (balance, topup) live on the
// parent credits command and are unchanged.

var creditsProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage the per-project credit bank (owner-only)",
	Long: `Per-project credit bank: deposit / withdraw / isolation.

Buckets: combined (default; either purpose), usage (infra + gas), chat
(AI chat). Owner-only mutations; collaborators and admins are read-only.`,
}

var creditsProjectStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project credit bank balances and isolation state",
	Example: `  poof credits project status -p <id>
  poof credits project status -p <id> --json
  poof credits project status -p <id> --quiet`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetProjectCredits(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			output.Info("Project credit bank — %s", projectID)

			usage := bucketTotal(resp.Usage.Withdrawable, resp.Usage.NonWithdrawable)
			chat := bucketTotal(resp.Chat.Withdrawable, resp.Chat.NonWithdrawable)
			combined := bucketTotal(resp.Combined.Withdrawable, resp.Combined.NonWithdrawable)
			total := usage + chat + combined

			output.Info("  Total deposited: %.2f credits", total)
			output.Info("    Usage:    %.2f  (%.2f withdrawable, %.2f reserved%s)",
				usage,
				clampNN(resp.Usage.Withdrawable),
				clampNN(resp.Usage.NonWithdrawable),
				isolatedSuffix(resp.Usage.Isolated),
			)
			output.Info("    Chat:     %.2f  (%.2f withdrawable, %.2f reserved%s)",
				chat,
				clampNN(resp.Chat.Withdrawable),
				clampNN(resp.Chat.NonWithdrawable),
				isolatedSuffix(resp.Chat.Isolated),
			)
			output.Info("    Combined: %.2f  (%.2f withdrawable, %.2f reserved)",
				combined,
				clampNN(resp.Combined.Withdrawable),
				clampNN(resp.Combined.NonWithdrawable),
			)
			output.Info("  Personal paid balance: %.2f credits", resp.UserPaidCreditsAvailable)
			ownerNote := "owner — can deposit / withdraw / toggle isolation"
			if !resp.IsOwner {
				ownerNote = "read-only access (collaborator or admin)"
			}
			output.Info("  Access: %s", ownerNote)
		})
		return nil
	},
}

var creditsProjectDepositCmd = &cobra.Command{
	Use:   "deposit",
	Short: "Deposit credits from your personal balance into the project bank",
	Long: `Move whole paid credits (subscription + add-on, never daily) into the
project bank. --bucket defaults to combined. 402 if paid balance is short.`,
	Example: `  poof credits project deposit -p <id> --amount 50
  poof credits project deposit -p <id> --amount 100 --bucket usage`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		amount, _ := cmd.Flags().GetInt("amount")
		if amount <= 0 {
			return fmt.Errorf("--amount must be a positive integer (whole credits)")
		}

		bucketStr, _ := cmd.Flags().GetString("bucket")
		if !api.IsValidBucket(bucketStr) {
			return fmt.Errorf("--bucket must be one of: combined, usage, chat (got %q)", bucketStr)
		}
		bucket := api.ProjectBankBucket(bucketStr)

		resp, err := apiClient.DepositProjectCredits(context.Background(), projectID, amount, bucket)
		if err != nil {
			return handleProjectBankError(err)
		}

		output.Print(resp, func() {
			output.Success("Deposited %d credits into the %s bucket.", resp.Deposited, resp.Bucket)
			balance := resp.Balance
			combined := bucketTotal(balance.Combined.Withdrawable, balance.Combined.NonWithdrawable)
			usage := bucketTotal(balance.Usage.Withdrawable, balance.Usage.NonWithdrawable)
			chat := bucketTotal(balance.Chat.Withdrawable, balance.Chat.NonWithdrawable)
			output.Info("  New project bank balances: combined=%.2f usage=%.2f chat=%.2f", combined, usage, chat)
			// Personal paid balance isn't returned by the deposit endpoint —
			// callers who need it should run `poof credits project status`
			// (or `poof credits balance`) after.
		})
		return nil
	},
}

var creditsProjectWithdrawCmd = &cobra.Command{
	Use:   "withdraw",
	Short: "Withdraw credits from the project bank back to your personal balance",
	Long: `Drain a withdrawable bucket back to your personal balance as a fresh
add-on payment record (6-month expiry). Reserved (Poof-granted) credits
stay put. One withdrawal per (user, project) at a time — concurrent
attempts return 402.`,
	Example: `  poof credits project withdraw -p <id> --amount 30
  poof credits project withdraw -p <id> --amount 50 --bucket usage`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		amount, _ := cmd.Flags().GetInt("amount")
		if amount <= 0 {
			return fmt.Errorf("--amount must be a positive integer (whole credits)")
		}

		bucketStr, _ := cmd.Flags().GetString("bucket")
		if !api.IsValidBucket(bucketStr) {
			return fmt.Errorf("--bucket must be one of: combined, usage, chat (got %q)", bucketStr)
		}
		bucket := api.ProjectBankBucket(bucketStr)

		resp, err := apiClient.WithdrawProjectCredits(context.Background(), projectID, amount, bucket)
		if err != nil {
			return handleProjectBankError(err)
		}

		output.Print(resp, func() {
			output.Success("Withdrew %d credits from the %s bucket.", resp.Withdrawn, resp.Bucket)
			output.Info("  Payment record: %s", resp.PaymentRecordID)
			balance := resp.Balance
			combined := bucketTotal(balance.Combined.Withdrawable, balance.Combined.NonWithdrawable)
			usage := bucketTotal(balance.Usage.Withdrawable, balance.Usage.NonWithdrawable)
			chat := bucketTotal(balance.Chat.Withdrawable, balance.Chat.NonWithdrawable)
			output.Info("  New project bank balances: combined=%.2f usage=%.2f chat=%.2f", combined, usage, chat)
			// Personal paid balance isn't returned by the withdraw endpoint;
			// run `poof credits balance` after to see the new pool, or
			// `poof credits project status` for the full picture.
		})
		return nil
	},
}

var creditsProjectIsolationCmd = &cobra.Command{
	Use:   "isolation",
	Short: "Toggle per-purpose project bank isolation (owner-only)",
	Long: `When isolated (true), that purpose only spends from the project bank
and pauses when empty. Off (false, default), it falls back to the owner's
personal balance. Pass at least one of --usage / --chat.`,
	Example: `  poof credits project isolation -p <id> --usage true
  poof credits project isolation -p <id> --chat false --usage true`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		var usagePtr, chatPtr *bool
		if cmd.Flags().Changed("usage") {
			v, err := cmd.Flags().GetBool("usage")
			if err != nil {
				return fmt.Errorf("--usage must be true or false: %w", err)
			}
			usagePtr = &v
		}
		if cmd.Flags().Changed("chat") {
			v, err := cmd.Flags().GetBool("chat")
			if err != nil {
				return fmt.Errorf("--chat must be true or false: %w", err)
			}
			chatPtr = &v
		}
		if usagePtr == nil && chatPtr == nil {
			return fmt.Errorf("pass at least one of --usage / --chat (e.g., --usage=true)")
		}

		if err := apiClient.SetProjectIsolation(context.Background(), projectID, usagePtr, chatPtr); err != nil {
			return handleError(err)
		}

		// Re-read so the user sees the authoritative server state, including
		// any flag they didn't touch.
		state, readErr := apiClient.GetProjectCredits(context.Background(), projectID)
		if readErr != nil {
			// The PUT succeeded — surface the read error but don't fail the
			// command. JSON callers still get a deterministic success
			// payload; text mode shows a hint to retry status.
			output.Print(map[string]interface{}{
				"success":      true,
				"projectId":    projectID,
				"warning":      "isolation updated, but failed to re-read state",
				"warningError": readErr.Error(),
			}, func() {
				output.Success("Isolation updated for %s.", projectID)
				output.Warn("Couldn't re-read state: %v. Run `poof credits project status -p %s` to confirm.", readErr, projectID)
			})
			return nil
		}

		output.Print(state, func() {
			output.Success("Isolation updated for %s.", projectID)
			output.Info("  Usage isolated: %v", state.Usage.Isolated)
			output.Info("  Chat  isolated: %v", state.Chat.Isolated)
		})
		return nil
	},
}

func init() {
	// `-p` / `--project` is declared as a persistent flag on the root command
	// (see root.go), so subcommands inherit it without re-declaring it.

	creditsProjectDepositCmd.Flags().Int("amount", 0, "Whole credits to deposit (required, > 0)")
	creditsProjectDepositCmd.Flags().String("bucket", string(api.BucketCombined), "Target bucket: combined | usage | chat")

	creditsProjectWithdrawCmd.Flags().Int("amount", 0, "Whole credits to withdraw (required, > 0)")
	creditsProjectWithdrawCmd.Flags().String("bucket", string(api.BucketCombined), "Source bucket: combined | usage | chat")

	creditsProjectIsolationCmd.Flags().Bool("usage", false, "Isolate runtime spend (infra + gas) — true / false")
	creditsProjectIsolationCmd.Flags().Bool("chat", false, "Isolate AI chat spend — true / false")

	creditsProjectCmd.AddCommand(creditsProjectStatusCmd)
	creditsProjectCmd.AddCommand(creditsProjectDepositCmd)
	creditsProjectCmd.AddCommand(creditsProjectWithdrawCmd)
	creditsProjectCmd.AddCommand(creditsProjectIsolationCmd)

	creditsCmd.AddCommand(creditsProjectCmd)
}

// Helpers shared with usage.go formatters. Defined here to keep the credits
// project file self-contained.
//
// clampNN returns 0 for any negative input — buckets can drift slightly
// negative under a concurrent-deduct race; we never want to display that.
func clampNN(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

func bucketTotal(w, nw float64) float64 {
	return clampNN(w) + clampNN(nw)
}

func isolatedSuffix(isolated bool) string {
	if isolated {
		return ", ISOLATED"
	}
	return ""
}

// handleProjectBankError preserves the server's 402 message for project-bank
// deposit / withdraw commands, since the shared `handleError` rewrites every
// 402 into the generic "run `poof credits topup`" advice — which is wrong
// for messages like "Insufficient paid credits to deposit N (available M)"
// or "A withdrawal is already in progress for this project". Everything
// other than 402 routes through the standard error handler.
func handleProjectBankError(err error) error {
	if apiErr, ok := api.IsAPIError(err); ok && apiErr.StatusCode == 402 {
		return fmt.Errorf("%s", apiErr.Message)
	}
	return handleError(err)
}
