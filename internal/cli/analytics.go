package cli

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Get client app analytics for a deployed project",
	Example: `  poof analytics -p <id>
  poof analytics -p <id> --environment preview --range 1h
  poof analytics -p <id> --environment production --limit 20
  poof analytics -p <id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		environment, _ := cmd.Flags().GetString("environment")
		timeRange, _ := cmd.Flags().GetString("range")
		limit, _ := cmd.Flags().GetInt("limit")

		resp, err := apiClient.GetClientAppAnalytics(context.Background(), projectID, environment, timeRange, limit)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.Metadata.Message != "" {
				output.Warn("%s", resp.Metadata.Message)
			}

			output.Info("Project:     %s", resp.ProjectID)
			output.Info("Environment: %s", resp.Environment)
			output.Info("Range:       %s (%s to %s)", resp.TimeRange.Range, resp.TimeRange.Start, resp.TimeRange.End)
			output.Info("Dataset:     %s", resp.Dataset)
			if len(resp.SiteIDs) > 0 {
				output.Info("Sites:       %v", resp.SiteIDs)
			}

			output.Info("")
			output.Info("Summary")
			summaryRows := [][]string{
				{"Events", formatCount(resp.Summary.Events)},
				{"Page views", formatCount(resp.Summary.PageViews)},
				{"Route views", formatCount(resp.Summary.RouteViews)},
				{"Visitors", formatCount(resp.Summary.Visitors)},
				{"Sessions", formatCount(resp.Summary.Sessions)},
				{"Errors", formatCount(resp.Summary.Errors)},
				{"API errors", formatCount(resp.Summary.APIErrors)},
				{"Resource errors", formatCount(resp.Summary.ResourceErrors)},
				{"JS errors", formatCount(resp.Summary.JSErrors)},
				{"Avg TTFB", formatMs(resp.Summary.AverageTTFBMs)},
				{"Avg LCP", formatMs(resp.Summary.AverageLCPMs)},
				{"Avg INP", formatMs(resp.Summary.AverageINPMs)},
				{"Avg CLS", fmt.Sprintf("%.3f", resp.Summary.AverageCLS)},
			}
			// Older servers omit the p75 fields; hide zero-value rows so they
			// don't read as a perfect score.
			if resp.Summary.P75TTFBMs > 0 {
				summaryRows = append(summaryRows, []string{"P75 TTFB", formatMs(resp.Summary.P75TTFBMs)})
			}
			if resp.Summary.P75LCPMs > 0 {
				summaryRows = append(summaryRows, []string{"P75 LCP", formatMs(resp.Summary.P75LCPMs)})
			}
			if resp.Summary.P75INPMs > 0 {
				summaryRows = append(summaryRows, []string{"P75 INP", formatMs(resp.Summary.P75INPMs)})
			}
			if resp.Summary.P75CLS > 0 {
				summaryRows = append(summaryRows, []string{"P75 CLS", fmt.Sprintf("%.3f", resp.Summary.P75CLS)})
			}
			if resp.Summary.LCPSampleCount > 0 {
				summaryRows = append(summaryRows, []string{"LCP samples", formatCount(resp.Summary.LCPSampleCount)})
			}
			summaryRows = append(summaryRows, []string{"Engaged seconds", formatCount(resp.Summary.EngagedSeconds)})
			output.Table([]string{"Metric", "Value"}, summaryRows)

			if len(resp.TopPages) > 0 {
				output.Info("")
				output.Info("Top pages")
				rows := make([][]string, len(resp.TopPages))
				for i, page := range resp.TopPages {
					rows[i] = []string{
						page.Path,
						formatCount(page.Events),
						formatCount(page.Visitors),
						formatCount(page.Errors),
					}
				}
				output.Table([]string{"Path", "Events", "Visitors", "Errors"}, rows)
			}

			if len(resp.Errors) > 0 {
				output.Info("")
				output.Info("Failures")
				rows := make([][]string, len(resp.Errors))
				for i, failure := range resp.Errors {
					lastSeen := ""
					if failure.LastSeen != nil {
						lastSeen = *failure.LastSeen
					}
					rows[i] = []string{
						failure.Event,
						failure.FailureClass,
						failure.Path,
						formatCount(failure.Count),
						lastSeen,
					}
				}
				output.Table([]string{"Event", "Class", "Path", "Count", "Last seen"}, rows)
			}
		})
		return nil
	},
}

func formatCount(value float64) string {
	return fmt.Sprintf("%.0f", value)
}

func formatMs(value float64) string {
	if value <= 0 {
		return "0ms"
	}
	return fmt.Sprintf("%.1fms", value)
}

func init() {
	analyticsCmd.Flags().String("environment", "", "Client app environment: draft, preview, production")
	analyticsCmd.Flags().String("range", "24h", "Time range: 1h, 6h, 24h, 3d, 7d")
	analyticsCmd.Flags().Int("limit", 10, "Max rows for top pages, failures, devices, countries, and referrers (server max: 50)")
}
