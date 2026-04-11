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
// `TaskID` is the *target* code checkpoint being scanned and is already in
// 'completed' status, so it must NOT be polled as an indicator of scan
// progress. Use ScanID with GetSecurityScan() to track the actual scan.
type SecurityScanResponse struct {
	Success   bool   `json:"success"`
	ScanID    string `json:"scanId"`
	MessageID string `json:"messageId"`
	Message   string `json:"message"`
	TaskID    string `json:"taskId"`
	TaskTitle string `json:"taskTitle"`
}

func (r *SecurityScanResponse) QuietString() string { return r.ScanID }

// SecurityScanStatus is a single security scan record returned by GET
// /api/project/[projectId]/security-scan/[scanId]. startedAt and completedAt
// are stored as InstantDB date fields, which serialize as either ISO strings
// or epoch milliseconds, so we use Timestamp to absorb both.
type SecurityScanStatus struct {
	ID                 string            `json:"id"`
	ScanNumber         int               `json:"scanNumber"`
	Status             string            `json:"status"`
	ScannedTaskID      string            `json:"scannedTaskId"`
	StartedAt          Timestamp         `json:"startedAt"`
	CompletedAt        Timestamp         `json:"completedAt"`
	TotalFindings      int               `json:"totalFindings"`
	CriticalSeverity   int               `json:"criticalSeverity"`
	HighSeverity       int               `json:"highSeverity"`
	MediumSeverity     int               `json:"mediumSeverity"`
	LowSeverity        int               `json:"lowSeverity"`
	InformationalCount int               `json:"informationalCount,omitempty"`
	DeployEligible     *bool             `json:"deployEligible,omitempty"`
	ErrorMessage       string            `json:"errorMessage"`
	Findings           []SecurityFinding `json:"findings,omitempty"`
}

// SecurityFinding is one entry from a scan's findings array. The server
// stores findings as a JSON string with a flexible schema — we surface the
// common fields agents need to triage, and keep the rest opaque via
// RawMessage for callers that want the full record.
type SecurityFinding struct {
	ID          string          `json:"id,omitempty"`
	Severity    string          `json:"severity,omitempty"`
	Category    string          `json:"category,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	File        string          `json:"file,omitempty"`
	Location    string          `json:"location,omitempty"`
	Raw         json.RawMessage `json:"-"`
}

type securityScanGetResponse struct {
	Scan SecurityScanStatus `json:"scan"`
}

// GetSecurityScan fetches a single security scan record by id.
func (c *Client) GetSecurityScan(ctx context.Context, projectID, scanID string) (*SecurityScanStatus, error) {
	path := fmt.Sprintf("/api/project/%s/security-scan/%s", projectID, scanID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp securityScanGetResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse security scan response: %w", err)
	}
	return &resp.Scan, nil
}

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
