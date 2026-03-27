package cli

import (
	"context"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Browse project templates",
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available templates",
	Example: `  poof template list
  poof template list --category defi
  poof template list --search "nft" --json
  poof template list --limit 10 --skip 20`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		category, _ := cmd.Flags().GetString("category")
		search, _ := cmd.Flags().GetString("search")
		sortBy, _ := cmd.Flags().GetString("sort")
		limit, _ := cmd.Flags().GetInt("limit")
		skip, _ := cmd.Flags().GetInt("skip")

		resp, err := apiClient.ListTemplates(context.Background(), category, search, sortBy, limit, skip)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Templates) == 0 {
				output.Info("No templates found.")
				return
			}
			rows := make([][]string, len(resp.Templates))
			for i, t := range resp.Templates {
				rows[i] = []string{t.Name, t.Category, t.Description}
			}
			output.Table([]string{"Name", "Category", "Description"}, rows)
			if resp.Pagination.HasMore {
				output.Info("(more templates available — use --skip %d to see next page)", skip+limit)
			}
		})
		return nil
	},
}

func init() {
	templateListCmd.Flags().String("category", "", "Filter by category")
	templateListCmd.Flags().String("search", "", "Search query")
	templateListCmd.Flags().String("sort", "", "Sort by: newest, most_used, staff_picks")
	templateListCmd.Flags().Int("limit", 20, "Max templates to return")
	templateListCmd.Flags().Int("skip", 0, "Number of templates to skip (pagination)")

	templateCmd.AddCommand(templateListCmd)
}
