package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type TasksResponse struct {
	Tasks []map[string]interface{} `json:"tasks"`
}

func (r *TasksResponse) QuietString() string {
	var ids []string
	for _, t := range r.Tasks {
		if id, ok := t["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return strings.Join(ids, "\n")
}

type TaskResponse struct {
	Task TaskDetail `json:"task"`
}

type TaskDetail struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

func (r *TaskResponse) QuietString() string { return r.Task.ID }

type TestResultsResponse struct {
	Results []TestResult `json:"results"`
	Summary TestSummary  `json:"summary"`
	HasMore bool         `json:"hasMore"`
}

func (r *TestResultsResponse) QuietString() string {
	return strconv.Itoa(r.Summary.Passed) + "/" + strconv.Itoa(r.Summary.Total) + " passed"
}

type TestResult struct {
	ID        string          `json:"id"`
	FileName  string          `json:"fileName"`
	TestName  string          `json:"testName"`
	Status    string          `json:"status"`
	Counts    TestCounts      `json:"counts"`
	LastError string          `json:"lastError"`
	Duration  float64         `json:"duration"`
	StartedAt json.RawMessage `json:"startedAt"`
}

// StartedAtString returns the startedAt value as a string, handling both
// numeric (epoch ms) and string (ISO 8601) formats from the server.
func (r *TestResult) StartedAtString() string {
	if len(r.StartedAt) == 0 {
		return ""
	}
	// Try string first
	var s string
	if err := json.Unmarshal(r.StartedAt, &s); err == nil {
		return s
	}
	// Try number (epoch ms)
	var n float64
	if err := json.Unmarshal(r.StartedAt, &n); err == nil {
		return strconv.FormatInt(int64(n), 10)
	}
	return string(r.StartedAt)
}

type TestCounts struct {
	Steps   int `json:"steps"`
	Expects int `json:"expects"`
	Failed  int `json:"failed"`
}

type TestSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Errors  int `json:"errors"`
	Running int `json:"running"`
}

func (c *Client) ListTasks(ctx context.Context, projectID, changeID string) (*TasksResponse, error) {
	path := fmt.Sprintf("/api/project/%s/tasks?changeId=%s", projectID, changeID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp TasksResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetTask(ctx context.Context, projectID, taskID string) (*TaskResponse, error) {
	path := fmt.Sprintf("/api/project/%s/task/%s", projectID, taskID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp TaskResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetTestResults(ctx context.Context, projectID string, limit, offset int) (*TestResultsResponse, error) {
	path := fmt.Sprintf("/api/project/%s/test-results", projectID)
	sep := "?"
	if limit > 0 {
		path += fmt.Sprintf("%slimit=%d", sep, limit)
		sep = "&"
	}
	if offset > 0 {
		path += fmt.Sprintf("%soffset=%d", sep, offset)
	}
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp TestResultsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
