package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type TasksResponse struct {
	Tasks []map[string]interface{} `json:"tasks"`
}

type TaskResponse struct {
	Task TaskDetail `json:"task"`
}

type TaskDetail struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

type TestResultsResponse struct {
	Results []TestResult `json:"results"`
	Summary TestSummary  `json:"summary"`
	HasMore bool         `json:"hasMore"`
}

type TestResult struct {
	ID        string     `json:"id"`
	FileName  string     `json:"fileName"`
	TestName  string     `json:"testName"`
	Status    string     `json:"status"`
	Counts    TestCounts `json:"counts"`
	LastError string     `json:"lastError"`
	Duration  float64    `json:"duration"`
	StartedAt string     `json:"startedAt"`
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
