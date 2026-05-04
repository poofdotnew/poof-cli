package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/auth"
	"github.com/poofdotnew/poof-cli/internal/config"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/poll"
	selfupdate "github.com/poofdotnew/poof-cli/internal/update"
	"github.com/poofdotnew/poof-cli/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	cfg       *config.Config
	apiClient *api.Client
	authMgr   *auth.Manager

	flagJSON          bool
	flagQuiet         bool
	flagEnv           string
	flagProject       string
	flagNoUpdateCheck bool
)

// rootCmd is the top-level poof command.
var rootCmd = &cobra.Command{
	Use:          "poof",
	Short:        "Poof CLI — build, deploy, and manage Solana dApps on poof.new",
	Version:      version.Short(),
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load config
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Override from flags
		if flagEnv != "" {
			cfg.PoofEnv = flagEnv
		}
		if flagJSON {
			output.SetFormat(output.FormatJSON)
			// Suppress cobra's "Error: ..." line; Execute() emits errors as JSON.
			cmd.Root().SilenceErrors = true
		}
		if flagQuiet {
			output.SetFormat(output.FormatQuiet)
		}

		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		maybeNotifyUpdate(cmd)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "Minimal output (IDs and URLs only)")
	rootCmd.PersistentFlags().StringVar(&flagEnv, "env", "", "Environment: production, staging, local")
	rootCmd.PersistentFlags().StringVarP(&flagProject, "project", "p", "", "Project ID")
	rootCmd.PersistentFlags().BoolVar(&flagNoUpdateCheck, "no-update-check", false, "Skip the automatic update check")

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(filesCmd)
	rootCmd.AddCommand(taskCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(creditsCmd)
	rootCmd.AddCommand(securityCmd)
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(secretsCmd)
	rootCmd.AddCommand(domainCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(analyticsCmd)
	rootCmd.AddCommand(preferencesCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(iterateCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(shipCmd)
	rootCmd.AddCommand(keygenCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(browserCmd)
	rootCmd.AddCommand(dataCmd)
	rootCmd.AddCommand(usageCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if output.GetFormat() == output.FormatJSON {
			// Emit the error as structured JSON so agents parsing stdout
			// with 2>&1 get valid JSON instead of a bare "Error: ..." line
			// that corrupts the stream.
			output.JSON(map[string]string{"error": err.Error()})
		}
		os.Exit(1)
	}
}

// requireAuth initializes the auth manager and API client.
// Call this in commands that need authentication.
func requireAuth() error {
	if cfg.SolanaPrivateKey == "" {
		return fmt.Errorf("SOLANA_PRIVATE_KEY not set. Run 'poof keygen' to create a keypair or set it in your .env file")
	}

	env, err := cfg.GetEnvironment()
	if err != nil {
		return err
	}

	authMgr, err = auth.NewManager(cfg.SolanaPrivateKey, env.AuthURL, env.AppID, cfg.PoofEnv)
	if err != nil {
		return err
	}

	apiClient, err = api.NewClient(cfg, authMgr)
	if err != nil {
		return err
	}

	return nil
}

// getProjectID returns the project ID from flag, config, or error.
func getProjectID() (string, error) {
	id := flagProject
	if id == "" {
		id = cfg.DefaultProject
	}
	if id == "" {
		return "", fmt.Errorf("project ID required: use --project flag or set default_project_id in ~/.poof/config.yaml")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid project ID: %q", id)
	}
	return id, nil
}

// validModes lists all supported generation modes for project creation.
var validModes = map[string]bool{
	"full":           true,
	"policy":         true,
	"ui,policy":      true,
	"backend,policy": true,
	"ui":             true,
	"backend":        true,
}

// validateMode checks if a generation mode is valid.
func validateMode(mode string) error {
	if !validModes[mode] {
		return fmt.Errorf("invalid mode %q (valid: full, policy, ui,policy, backend,policy)", mode)
	}
	return nil
}

// handleError formats an API error with context-aware messaging and returns it.
func handleError(err error) error {
	apiErr, ok := api.IsAPIError(err)
	if !ok {
		return err
	}

	switch {
	case apiErr.IsCreditsExhausted():
		return fmt.Errorf("no credits remaining. Run 'poof credits balance' to check, or 'poof credits topup' to buy more")
	case apiErr.IsPaymentRequired():
		return fmt.Errorf("this feature requires a credit purchase. Run 'poof credits topup' first")
	case apiErr.IsAuthError():
		return fmt.Errorf("authentication failed. Run 'poof auth login' to re-authenticate")
	case apiErr.StatusCode == 403:
		msg := apiErr.Message
		lower := strings.ToLower(msg)
		if msg == "" || lower == "forbidden" || lower == "not authorized" {
			return fmt.Errorf("not authorized for this project. Check: 1) project ID is correct (poof project list), 2) wallet matches the owner (poof auth status), 3) --env / POOF_ENV matches the environment the project was created in")
		}
		return fmt.Errorf("forbidden: %s", msg)
	case apiErr.StatusCode == 404:
		return fmt.Errorf("not found: %s", apiErr.Message)
	}
	return fmt.Errorf("%s", apiErr.Message)
}

func maybeNotifyUpdate(cmd *cobra.Command) {
	if shouldSkipUpdateCheck(cmd) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cachePath := filepath.Join(config.PoofDir(), "update_cache.json")
	result, err := selfupdate.CheckWithCache(ctx, selfupdate.NewClient(), version.Version, cachePath, 24*time.Hour)
	if err != nil || !result.UpdateAvailable {
		return
	}
	if !selfupdate.NotificationDue(cachePath, result, 24*time.Hour) {
		return
	}

	output.Warn("A new poof version is available: %s (current %s). Run 'poof update' to upgrade.", result.LatestVersion, result.CurrentVersion)
	_ = selfupdate.MarkNotified(cachePath, result)
}

func shouldSkipUpdateCheck(cmd *cobra.Command) bool {
	if flagNoUpdateCheck || output.GetFormat() != output.FormatText {
		return true
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return true
	}
	if envDisablesUpdateCheck(os.Getenv("POOF_NO_UPDATE_CHECK")) {
		return true
	}
	if cmd != nil {
		path := cmd.CommandPath()
		if path == "poof update" || strings.HasPrefix(path, "poof completion") {
			return true
		}
	}
	return false
}

func envDisablesUpdateCheck(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// pollAIUntilIdle polls the AI active endpoint until the AI finishes. It uses
// an activation grace period to avoid declaring "done" before the server has
// even started the AI. If the poll times out, the function cancels the AI
// session so subsequent commands (e.g. poof ship) aren't blocked by a stale
// active session.
func pollAIUntilIdle(ctx context.Context, projectID, spinnerMsg string) error {
	seenActive := false
	pollStart := time.Now()
	const activationGrace = 30 * time.Second

	err := output.WithSpinner(spinnerMsg, func() error {
		return poll.Poll(ctx, poll.LongAIConfig(), func(ctx context.Context) (bool, error) {
			status, err := apiClient.CheckAIActive(ctx, projectID)
			if err != nil {
				return false, err
			}
			if status.Status == "error" {
				return false, fmt.Errorf("AI processing failed with error status")
			}
			if status.Active {
				seenActive = true
				return false, nil
			}
			if seenActive || time.Since(pollStart) > activationGrace {
				return true, nil
			}
			return false, nil
		})
	})
	if err != nil {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = apiClient.CancelAI(cancelCtx, projectID)
	}
	return err
}
