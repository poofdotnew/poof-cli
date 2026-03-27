package cli

import (
	"context"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage custom domains (requires credit purchase)",
}

var domainListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List custom domains",
	Example: `  poof domain list -p <id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetDomains(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Domains) == 0 {
				output.Info("No custom domains configured.")
				return
			}
			rows := make([][]string, len(resp.Domains))
			for i, d := range resp.Domains {
				def := ""
				if d.IsDefault {
					def = "yes"
				}
				rows[i] = []string{d.Domain, d.Status, def}
			}
			output.Table([]string{"Domain", "Status", "Default"}, rows)
		})
		return nil
	},
}

var domainAddCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Add a custom domain",
	Example: `  poof domain add myapp.com -p <id>
  poof domain add myapp.com -p <id> --default`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		isDefault, _ := cmd.Flags().GetBool("default")

		if err := apiClient.AddDomain(context.Background(), projectID, args[0], isDefault); err != nil {
			return handleError(err)
		}

		output.Print(map[string]interface{}{
			"success":   true,
			"domain":    args[0],
			"isDefault": isDefault,
		}, func() {
			output.Success("Domain %s added.", args[0])
			if isDefault {
				output.Info("Set as default domain.")
			}
		})
		return nil
	},
}

func init() {
	domainAddCmd.Flags().Bool("default", false, "Set as default domain")

	domainCmd.AddCommand(domainListCmd)
	domainCmd.AddCommand(domainAddCmd)
}
