package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var preferencesCmd = &cobra.Command{
	Use:   "preferences",
	Short: "Manage AI model preferences",
}

var preferencesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get current AI model tiers",
	Example: `  poof preferences get
  poof preferences get --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		resp, err := apiClient.GetPreferences(context.Background())
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Preferences) == 0 {
				output.Info("No preferences set.")
				return
			}
			rows := make([][]string, 0, len(resp.Preferences))
			for k, v := range resp.Preferences {
				// Skip nested objects (like modelOverrides) in table display
				if s, ok := v.(string); ok {
					rows = append(rows, []string{k, s})
				}
			}
			output.Table([]string{"Use Case", "Tier"}, rows)
		})
		return nil
	},
}

var preferencesSetCmd = &cobra.Command{
	Use:   "set KEY=VALUE [KEY=VALUE...]",
	Short: "Set AI model tiers (average, smart, genius)",
	Example: `  poof preferences set mainChat=genius
  poof preferences set mainChat=genius codingAgent=smart`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		prefs := make(map[string]interface{})
		for _, arg := range args {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid format %q — use KEY=VALUE (e.g., mainChat=genius)", arg)
			}
			if parts[0] == "" {
				return fmt.Errorf("key cannot be empty in %q", arg)
			}
			tier := parts[1]
			if tier != "average" && tier != "smart" && tier != "genius" {
				return fmt.Errorf("invalid tier %q — use average, smart, or genius", tier)
			}
			prefs[parts[0]] = tier
		}

		if err := apiClient.SetPreferences(context.Background(), prefs); err != nil {
			return handleError(err)
		}

		output.Success("Preferences updated.")
		return nil
	},
}

func init() {
	preferencesCmd.AddCommand(preferencesGetCmd)
	preferencesCmd.AddCommand(preferencesSetCmd)
}
