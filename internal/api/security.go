package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type SecurityScanRequest struct {
	TarobaseToken string `json:"tarobaseToken"`
	TaskID        string `json:"taskId,omitempty"`
}

// SecurityScanResponse matches the server's async scan initiation response.
type SecurityScanResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"messageId"`
	Message   string `json:"message"`
	TaskID    string `json:"taskId"`
	TaskTitle string `json:"taskTitle"`
}

func (r *SecurityScanResponse) QuietString() string { return r.TaskID }

func (c *Client) SecurityScan(ctx context.Context, projectID string, taskID ...string) (*SecurityScanResponse, error) {
	path := fmt.Sprintf("/api/project/%s/security-scan", projectID)

	body, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		req := SecurityScanRequest{TarobaseToken: token}
		if len(taskID) > 0 && taskID[0] != "" {
			req.TaskID = taskID[0]
		}
		return req, nil
	})
	if err != nil {
		return nil, err
	}

	var resp SecurityScanResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse security scan response: %w", err)
	}
	return &resp, nil
}
