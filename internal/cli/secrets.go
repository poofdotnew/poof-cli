package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Manage project secrets",
}

var secretsGetCmd = &cobra.Command{
	Use:     "get",
	Short:   "Get secret names and requirements",
	Example: `  poof secrets get -p <id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetSecrets(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Secrets.Required) > 0 {
				output.Info("Required: %s", strings.Join(resp.Secrets.Required, ", "))
			}
			if len(resp.Secrets.Optional) > 0 {
				output.Info("Optional: %s", strings.Join(resp.Secrets.Optional, ", "))
			}
			if len(resp.Secrets.Required) == 0 && len(resp.Secrets.Optional) == 0 {
				output.Info("No secrets configured.")
			}
		})
		return nil
	},
}

var secretsSetCmd = &cobra.Command{
	Use:   "set KEY=VALUE [KEY=VALUE...]",
	Short: "Set secret values",
	Example: `  poof secrets set -p <id> API_KEY=sk-123 DB_URL=postgres://...
  poof secrets set -p <id> --environment preview API_KEY=sk-456`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		secrets := make(map[string]string)
		for _, arg := range args {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid format %q — use KEY=VALUE", arg)
			}
			secrets[parts[0]] = parts[1]
		}

		environment, _ := cmd.Flags().GetString("environment")

		if err := apiClient.SetSecrets(context.Background(), projectID, secrets, environment); err != nil {
			return handleError(err)
		}

		output.Success("Set %d secret(s).", len(secrets))
		return nil
	},
}

func init() {
	secretsSetCmd.Flags().String("environment", "", "Target environment")

	secretsCmd.AddCommand(secretsGetCmd)
	secretsCmd.AddCommand(secretsSetCmd)
}
