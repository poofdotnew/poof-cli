package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type SecurityScanRequest struct {
	TarobaseToken string `json:"tarobaseToken"`
}

type SecurityScanResponse struct {
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Summary         ScanSummary     `json:"summary"`
	Status          string          `json:"status"`
}

type Vulnerability struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
}

type ScanSummary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
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
