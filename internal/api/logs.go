package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

type LogsResponse struct {
	Logs []LogEntry `json:"logs"`
}

func (c *Client) GetLogs(ctx context.Context, projectID, environment string, limit int) (*LogsResponse, error) {
	params := url.Values{}
	if environment != "" {
		params.Set("environment", environment)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := fmt.Sprintf("/api/project/%s/logs", projectID)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp LogsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
