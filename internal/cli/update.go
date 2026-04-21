package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/poofdotnew/poof-cli/internal/output"
	selfupdate "github.com/poofdotnew/poof-cli/internal/update"
	"github.com/poofdotnew/poof-cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	updateCheckOnly bool
	updateForce     bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update poof to the latest release",
	Example: `  poof update
  poof update --check
  poof update --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		client := selfupdate.NewClient()
		rel, err := client.LatestRelease(ctx)
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}
		check, err := selfupdate.CheckRelease(version.Version, rel)
		if err != nil {
			return fmt.Errorf("failed to inspect latest release: %w", err)
		}

		if updateCheckOnly {
			renderUpdateCheck(check)
			return nil
		}
		if !check.Comparable && !updateForce {
			return fmt.Errorf("current version %q is not a release build; rerun with --force to install latest release %s", version.Version, check.LatestVersion)
		}
		if !check.UpdateAvailable && !updateForce {
			output.Print(check, func() {
				output.Success("poof is already up to date (%s).", version.Version)
			})
			return nil
		}

		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to locate current executable: %w", err)
		}

		if output.GetFormat() == output.FormatText {
			if updateForce && !check.UpdateAvailable {
				output.Info("Installing latest poof release %s...", check.LatestVersion)
			} else {
				output.Info("Updating poof %s -> %s...", version.Version, check.LatestVersion)
			}
		}

		result, err := client.InstallRelease(ctx, rel, exePath)
		if errors.Is(err, selfupdate.ErrManagedInstall) {
			return fmt.Errorf("this poof binary appears to be managed by Homebrew; run 'brew upgrade poofdotnew/tap/poof'")
		}
		if err != nil {
			return fmt.Errorf("failed to update poof: %w", err)
		}
		result.PreviousVersion = version.Version

		output.Print(result, func() {
			output.Success("Updated poof to %s.", result.Version)
			output.Info("Installed: %s", result.Path)
		})
		return nil
	},
}

func renderUpdateCheck(check *selfupdate.CheckResult) {
	output.Print(check, func() {
		switch {
		case check.UpdateAvailable:
			output.Warn("A new poof version is available: %s (current %s).", check.LatestVersion, check.CurrentVersion)
			output.Info("Run 'poof update' to upgrade.")
		case !check.Comparable:
			output.Info("Latest poof release: %s", check.LatestVersion)
			output.Info("Current build: %s", check.CurrentVersion)
		default:
			output.Success("poof is up to date (%s).", check.CurrentVersion)
		}
	})
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Check for updates without installing")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Install the latest release even when the current version cannot be compared")
}
