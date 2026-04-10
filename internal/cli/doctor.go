package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Print overall project health: status, deploy state, tests, AI state",
	Long: `Read-only health check that aggregates everything an agent typically needs to
decide what to do next:

- Project status (slug, urls, publish state for every target)
- Whether the AI is currently active
- The most recent project tasks
- Latest structured test results summary
- A quick HEAD probe of the draft URL

doctor never sends chat messages and never modifies project state.`,
	Example: `  poof doctor -p <id>
  poof doctor -p <id> --json
  poof doctor -p <id> --skip-url-probe`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		skipProbe, _ := cmd.Flags().GetBool("skip-url-probe")
		taskLimit, _ := cmd.Flags().GetInt("task-limit")
		if taskLimit <= 0 {
			taskLimit = 10
		}

		ctx := context.Background()

		report := &doctorReport{ProjectID: projectID}

		// Project status (urls + publish state)
		status, statusErr := apiClient.GetProjectStatus(ctx, projectID)
		if statusErr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("status: %v", statusErr))
		} else {
			report.Project = &status.Project
			report.URLs = status.URLs
			report.PublishState = status.PublishState
			report.DraftDeployedFlag = status.IsTargetDeployed("draft")
			report.PreviewDeployedFlag = status.IsTargetDeployed("preview")
			report.LiveDeployedFlag = status.IsTargetDeployed("live")
		}

		// AI active state
		if active, err := apiClient.CheckAIActive(ctx, projectID); err == nil {
			report.AI = active
		} else {
			report.Errors = append(report.Errors, fmt.Sprintf("ai/active: %v", err))
		}

		// Recent tasks
		if tasks, err := apiClient.ListTasks(ctx, projectID, "", taskLimit, 0); err == nil {
			report.RecentTasks = tasks.Tasks
		} else {
			report.Errors = append(report.Errors, fmt.Sprintf("tasks: %v", err))
		}

		// Test results summary. The server returns raw history — including
		// older failed runs that the AI already fixed — so collapse to the
		// most recent result per (source, fileName, testName) before
		// summarizing. This mirrors verify's freshness logic and prevents
		// doctor from claiming test failures that no longer apply.
		if tests, err := apiClient.GetTestResults(ctx, projectID, 100, 0); err == nil {
			latest := collapseResultsToLatest(tests.Results)
			summary := summarizeResults(latest)
			report.TestSummary = &summary
			report.RawTestSummary = &tests.Summary
		} else if apiErr, ok := api.IsAPIError(err); ok && apiErr.IsNotFound() {
			report.TestSummary = &api.TestSummary{}
		} else {
			report.Errors = append(report.Errors, fmt.Sprintf("test-results: %v", err))
		}

		// Draft probe
		if !skipProbe && status != nil {
			if draft, ok := status.URLs["draft"]; ok && draft != "" {
				code, perr := doctorProbe(ctx, draft)
				report.Probe = &doctorProbeResult{URL: draft}
				if perr != nil {
					report.Probe.Error = perr.Error()
				} else {
					report.Probe.StatusCode = code
					report.Probe.Reachable = code >= 200 && code < 400
				}
			}
		}

		// Derive an overall verdict to make agent decisions easier.
		report.Verdict = computeVerdict(report)

		output.Print(report, func() { renderDoctorText(report) })
		return nil
	},
}

type doctorReport struct {
	ProjectID           string                   `json:"projectId"`
	Project             *api.Project             `json:"project,omitempty"`
	URLs                map[string]string        `json:"urls,omitempty"`
	PublishState        map[string]interface{}   `json:"publishState,omitempty"`
	DraftDeployedFlag   bool                     `json:"draftDeployedFlag"`
	PreviewDeployedFlag bool                     `json:"previewDeployedFlag"`
	LiveDeployedFlag    bool                     `json:"liveDeployedFlag"`
	AI                  *api.AIActiveResponse    `json:"ai,omitempty"`
	RecentTasks         []map[string]interface{} `json:"recentTasks,omitempty"`
	TestSummary         *api.TestSummary         `json:"testSummary,omitempty"`
	RawTestSummary      *api.TestSummary         `json:"rawTestSummary,omitempty"`
	Probe               *doctorProbeResult       `json:"probe,omitempty"`
	Verdict             string                   `json:"verdict"`
	Errors              []string                 `json:"errors,omitempty"`
}

// collapseResultsToLatest returns the most recent result per
// (source, fileName). The server returns results sorted by startedAt desc,
// so the first occurrence wins. We intentionally collapse on file name
// rather than (file, testName) because the AI often renames the test
// inside a file between runs and we want the latest file state to win.
func collapseResultsToLatest(rs []api.TestResult) []api.TestResult {
	out := make([]api.TestResult, 0, len(rs))
	seen := make(map[string]struct{}, len(rs))
	for i := range rs {
		key := rs[i].Source + "|" + rs[i].FileName
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, rs[i])
	}
	return out
}

func (r *doctorReport) QuietString() string {
	parts := []string{r.ProjectID, r.Verdict}
	if r.URLs != nil {
		if u, ok := r.URLs["draft"]; ok && u != "" {
			parts = append(parts, u)
		}
	}
	return strings.Join(parts, "\n")
}

type doctorProbeResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode,omitempty"`
	Reachable  bool   `json:"reachable"`
	Error      string `json:"error,omitempty"`
}

// computeVerdict folds the report into a single keyword agents can branch on.
func computeVerdict(r *doctorReport) string {
	if len(r.Errors) > 0 {
		return "incomplete"
	}
	if r.AI != nil && r.AI.Active {
		return "ai_running"
	}
	probeReachable := r.Probe != nil && r.Probe.Reachable
	if r.DraftDeployedFlag || probeReachable {
		if r.TestSummary != nil && (r.TestSummary.Failed > 0 || r.TestSummary.Errors > 0) {
			return "deployed_with_test_failures"
		}
		if r.TestSummary == nil || r.TestSummary.Total == 0 {
			return "deployed_without_tests"
		}
		return "healthy"
	}
	if r.Project != nil {
		return "deploy_pending"
	}
	return "unknown"
}

func renderDoctorText(r *doctorReport) {
	if r.Project != nil {
		output.Info("Project: %s", r.Project.Title)
		output.Info("ID:      %s", r.Project.ID)
	}
	if r.URLs != nil {
		if u, ok := r.URLs["draft"]; ok && u != "" {
			output.Info("Draft:   %s", u)
		}
		if u, ok := r.URLs["mainnetPreview"]; ok && u != "" {
			output.Info("Preview: %s", u)
		}
		if u, ok := r.URLs["production"]; ok && u != "" {
			output.Info("Prod:    %s", u)
		}
	}
	output.Info("Deploy flags: draft=%v preview=%v live=%v",
		r.DraftDeployedFlag, r.PreviewDeployedFlag, r.LiveDeployedFlag)

	if r.AI != nil {
		state := r.AI.State
		if state == "" {
			if r.AI.Active {
				state = "running"
			} else {
				state = "idle"
			}
		}
		output.Info("AI: %s (active=%v)", state, r.AI.Active)
	}

	if r.TestSummary != nil {
		output.Info("Tests: %d total, %d passed, %d failed, %d errors",
			r.TestSummary.Total, r.TestSummary.Passed,
			r.TestSummary.Failed, r.TestSummary.Errors)
	}

	if len(r.RecentTasks) > 0 {
		output.Info("Recent tasks:")
		for i, t := range r.RecentTasks {
			if i >= 5 {
				output.Info("  ... (%d more)", len(r.RecentTasks)-5)
				break
			}
			id, _ := t["id"].(string)
			st, _ := t["status"].(string)
			title, _ := t["title"].(string)
			output.Info("  - [%s] %s — %s", st, id, title)
		}
	}

	if r.Probe != nil {
		if r.Probe.Error != "" {
			output.Warn("Draft probe: %s -> error: %s", r.Probe.URL, r.Probe.Error)
		} else if r.Probe.Reachable {
			output.Info("Draft probe: %s -> HTTP %d (reachable)", r.Probe.URL, r.Probe.StatusCode)
		} else {
			output.Warn("Draft probe: %s -> HTTP %d (unreachable)", r.Probe.URL, r.Probe.StatusCode)
		}
	}

	switch r.Verdict {
	case "healthy":
		output.Success("Verdict: %s", r.Verdict)
	case "deployed_without_tests", "deployed_with_test_failures", "ai_running", "deploy_pending":
		output.Warn("Verdict: %s", r.Verdict)
	default:
		output.Error("Verdict: %s", r.Verdict)
	}

	for _, e := range r.Errors {
		output.Error("  %s", e)
	}
}

func doctorProbe(ctx context.Context, url string) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func init() {
	doctorCmd.Flags().Bool("skip-url-probe", false, "Skip the draft URL HEAD probe")
	doctorCmd.Flags().Int("task-limit", 10, "How many recent tasks to include")
}
