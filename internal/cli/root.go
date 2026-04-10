package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/auth"
	"github.com/poofdotnew/poof-cli/internal/config"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	cfg       *config.Config
	apiClient *api.Client
	authMgr   *auth.Manager

	flagJSON    bool
	flagQuiet   bool
	flagEnv     string
	flagProject string
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
		}
		if flagQuiet {
			output.SetFormat(output.FormatQuiet)
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "Minimal output (IDs and URLs only)")
	rootCmd.PersistentFlags().StringVar(&flagEnv, "env", "", "Environment: production, staging, local")
	rootCmd.PersistentFlags().StringVarP(&flagProject, "project", "p", "", "Project ID")

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
	rootCmd.AddCommand(preferencesCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(iterateCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(shipCmd)
	rootCmd.AddCommand(keygenCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(browserCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
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

	if apiErr.IsCreditsExhausted() {
		return fmt.Errorf("no credits remaining. Run 'poof credits balance' to check, or 'poof credits topup' to buy more")
	} else if apiErr.IsPaymentRequired() {
		return fmt.Errorf("this feature requires a credit purchase. Run 'poof credits topup' first")
	} else if apiErr.IsAuthError() {
		return fmt.Errorf("authentication failed. Run 'poof auth login' to re-authenticate")
	}
	return fmt.Errorf("%s", apiErr.Message)
}
