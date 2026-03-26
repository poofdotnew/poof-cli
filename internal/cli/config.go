package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/poofdotnew/poof-cli/internal/config"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or set CLI configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Example: `  poof config show
  poof config show --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Print(map[string]string{
			"environment":    cfg.PoofEnv,
			"walletAddress":  cfg.WalletAddress,
			"defaultProject": cfg.DefaultProject,
			"outputFormat":   cfg.OutputFormat,
		}, func() {
			output.Info("Environment:     %s", cfg.PoofEnv)
			if cfg.WalletAddress != "" {
				output.Info("Wallet:          %s", cfg.WalletAddress)
			} else {
				output.Info("Wallet:          (not set)")
			}
			if cfg.DefaultProject != "" {
				output.Info("Default project: %s", cfg.DefaultProject)
			}
			output.Info("Config dir:      %s", config.PoofDir())
		})
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set KEY VALUE",
	Short: "Set a config value (saved to ~/.poof/config.yaml)",
	Example: `  poof config set default_project_id <id>
  poof config set environment staging
  poof config set output_format json`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		validKeys := map[string]bool{
			"environment":        true,
			"default_project_id": true,
			"output_format":      true,
		}
		if !validKeys[key] {
			return fmt.Errorf("unknown config key %q (valid: environment, default_project_id, output_format)", key)
		}

		configPath := filepath.Join(config.PoofDir(), "config.yaml")

		// Read existing config
		existing := make(map[string]string)
		data, err := os.ReadFile(configPath)
		if err == nil {
			if err := yaml.Unmarshal(data, &existing); err != nil {
				return fmt.Errorf("failed to parse existing config at %s: %w", configPath, err)
			}
		}

		existing[key] = value

		out, err := yaml.Marshal(existing)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}

		if err := os.WriteFile(configPath, out, 0600); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}

		output.Success("Set %s = %s", key, value)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
}
