package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type SecurityScanRequest struct {
	TarobaseToken string `json:"tarobaseToken"`
}

// SecurityScanResponse matches the server's async scan initiation response.
type SecurityScanResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"messageId"`
	Message   string `json:"message"`
	TaskID    string `json:"taskId"`
	TaskTitle string `json:"taskTitle"`
}

func (c *Client) SecurityScan(ctx context.Context, projectID string) (*SecurityScanResponse, error) {
	path := fmt.Sprintf("/api/project/%s/security-scan", projectID)

	body, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return SecurityScanRequest{TarobaseToken: token}, nil
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
