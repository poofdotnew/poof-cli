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
	Use:   "get",
	Short: "Get secret names and requirements",
	Example: `  poof secrets get -p <id>
  poof secrets get -p <id> --environment production`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		environment, _ := cmd.Flags().GetString("environment")
		resp, err := apiClient.GetSecrets(context.Background(), projectID, environment)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			reqs := resp.SecretRequirements
			if len(reqs.Required) == 0 && len(reqs.Optional) == 0 {
				output.Info("No secrets configured.")
				return
			}

			if len(reqs.Required) > 0 {
				output.Info("Required:")
				for _, s := range reqs.Required {
					status := "missing"
					if s.HasValue {
						status = "set"
					}
					output.Info("  %s (%s) [%s]", s.Key, s.Label, status)
				}
			}
			if len(reqs.Optional) > 0 {
				output.Info("Optional:")
				for _, s := range reqs.Optional {
					status := "missing"
					if s.HasValue {
						status = "set"
					}
					output.Info("  %s (%s) [%s]", s.Key, s.Label, status)
				}
			}

			output.Info("\nSummary: %d/%d required set, %d/%d optional set",
				resp.Summary.RequiredWithValues, resp.Summary.TotalRequired,
				resp.Summary.OptionalWithValues, resp.Summary.TotalOptional)
		})
		return nil
	},
}

var secretsSetCmd = &cobra.Command{
	Use:     "set KEY=VALUE [KEY=VALUE...]",
	Short:   "Set secret values",
	Example: `  poof secrets set -p <id> API_KEY=sk-123 DB_URL=postgres://...`,
	Args:    cobra.MinimumNArgs(1),
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
			if parts[0] == "" {
				return fmt.Errorf("key cannot be empty in %q", arg)
			}
			secrets[parts[0]] = parts[1]
		}

		if err := apiClient.SetSecrets(context.Background(), projectID, secrets); err != nil {
			return handleError(err)
		}

		output.Success("Set %d secret(s).", len(secrets))
		return nil
	},
}

func init() {
	secretsGetCmd.Flags().String("environment", "", "Environment: development, mainnet-preview, production")

	secretsCmd.AddCommand(secretsGetCmd)
	secretsCmd.AddCommand(secretsSetCmd)
}
