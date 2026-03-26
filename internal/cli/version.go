package cli

import (
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Example: `  poof version
  poof version --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Print(map[string]string{
			"version": version.Version,
			"commit":  version.Commit,
			"date":    version.Date,
		}, func() {
			fmt.Println(version.Info())
		})
		return nil
	},
}
