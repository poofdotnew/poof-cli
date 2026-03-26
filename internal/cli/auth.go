package cli

import (
	"fmt"
	"time"

	"github.com/poofdotnew/poof-cli/internal/auth"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate and cache token",
	Example: `  poof auth login
  poof auth login --env staging`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		token, err := authMgr.Login()
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		_ = token // token is cached; don't print it
		output.Print(map[string]string{
			"wallet": authMgr.WalletAddress(),
			"env":    cfg.PoofEnv,
		}, func() {
			output.Success("Logged in as %s (%s)", authMgr.WalletAddress(), cfg.PoofEnv)
		})
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current auth status",
	Example: `  poof auth status
  poof auth status --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cached, err := auth.LoadCachedTokens()
		if err != nil {
			output.Print(map[string]string{"status": "not authenticated"}, func() {
				output.Info("Not authenticated. Run 'poof auth login' to authenticate.")
			})
			return nil
		}

		valid := time.Now().Before(cached.ExpiresAt)
		status := "valid"
		if !valid {
			status = "expired"
		}

		output.Print(map[string]interface{}{
			"wallet":    cached.Wallet,
			"env":       cached.Environment,
			"status":    status,
			"expiresAt": cached.ExpiresAt.Format(time.RFC3339),
		}, func() {
			output.Info("Wallet:      %s", cached.Wallet)
			output.Info("Environment: %s", cached.Environment)
			if valid {
				output.Success("Token valid (expires %s)", cached.ExpiresAt.Format("15:04:05"))
			} else {
				output.Error("Token expired at %s. Run 'poof auth login' to refresh.", cached.ExpiresAt.Format("15:04:05"))
			}
		})
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear cached credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := auth.ClearCachedTokens(); err != nil {
			// If the file doesn't exist, that's fine
			output.Info("No cached credentials to clear.")
			return nil
		}
		output.Success("Logged out. Cached credentials cleared.")
		return nil
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}
