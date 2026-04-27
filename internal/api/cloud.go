package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type CloudProjectCreateRequest struct {
	Title          string `json:"title,omitempty"`
	Description    string `json:"description,omitempty"`
	Slug           string `json:"slug,omitempty"`
	GenerationMode string `json:"generationMode,omitempty"`
	IsPublic       *bool  `json:"isPublic,omitempty"`
	TarobaseToken  string `json:"tarobaseToken,omitempty"`
	Policy         string `json:"policy,omitempty"`
	Constants      string `json:"constants,omitempty"`
}

type CloudProjectCreateResponse struct {
	Success       bool                   `json:"success"`
	ProjectID     string                 `json:"projectId"`
	Message       string                 `json:"message,omitempty"`
	InitialDeploy *PolicyDeployResult    `json:"initialDeploy,omitempty"`
	PolicyState   *PolicyStateSummary    `json:"policyState,omitempty"`
	Bootstrap     map[string]interface{} `json:"bootstrap,omitempty"`
}

func (r *CloudProjectCreateResponse) QuietString() string { return r.ProjectID }

type PolicyStateSummary struct {
	LatestTaskID   string          `json:"latestTaskId,omitempty"`
	PolicyHash     string          `json:"policyHash,omitempty"`
	ConstantsHash  string          `json:"constantsHash,omitempty"`
	ConnectionInfo *ConnectionInfo `json:"connectionInfo,omitempty"`
}

type PolicyStateResponse struct {
	ProjectID      string          `json:"projectId"`
	LatestTaskID   string          `json:"latestTaskId,omitempty"`
	Policy         string          `json:"policy"`
	Constants      string          `json:"constants"`
	PolicyHash     string          `json:"policyHash,omitempty"`
	ConstantsHash  string          `json:"constantsHash,omitempty"`
	ConnectionInfo *ConnectionInfo `json:"connectionInfo,omitempty"`
}

func (r *PolicyStateResponse) QuietString() string {
	if r == nil {
		return ""
	}
	return r.LatestTaskID
}

type PolicyRequest struct {
	TarobaseToken           string `json:"tarobaseToken,omitempty"`
	Environment             string `json:"environment,omitempty"`
	Policy                  string `json:"policy,omitempty"`
	Constants               string `json:"constants,omitempty"`
	SourceTaskID            string `json:"sourceTaskId,omitempty"`
	TaskID                  string `json:"taskId,omitempty"`
	DryRun                  bool   `json:"dryRun,omitempty"`
	SignedPermitTransaction string `json:"signedPermitTransaction,omitempty"`
}

type PolicyValidation struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Message string   `json:"message,omitempty"`
}

type PolicyDeployResult struct {
	Success       bool             `json:"success"`
	ProjectID     string           `json:"projectId"`
	TaskID        string           `json:"taskId,omitempty"`
	Environment   string           `json:"environment"`
	AppID         string           `json:"appId"`
	PolicyHash    string           `json:"policyHash"`
	ConstantsHash string           `json:"constantsHash"`
	Validation    PolicyValidation `json:"validation"`
	Deployed      bool             `json:"deployed"`
	DryRun        bool             `json:"dryRun"`
}

func (r *PolicyDeployResult) QuietString() string {
	if r == nil {
		return ""
	}
	if r.TaskID != "" {
		return r.TaskID
	}
	return r.PolicyHash
}

type PolicyHistoryResponse struct {
	ProjectID string               `json:"projectId"`
	History   []PolicyHistoryEntry `json:"history"`
}

type PolicyHistoryEntry struct {
	TaskID        string `json:"taskId"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	FocusArea     string `json:"focusArea,omitempty"`
	CreatedAt     string `json:"createdAt"`
	PolicyHash    string `json:"policyHash,omitempty"`
	ConstantsHash string `json:"constantsHash,omitempty"`
}

func (r *PolicyHistoryResponse) QuietString() string {
	if r == nil || len(r.History) == 0 {
		return ""
	}
	return r.History[0].TaskID
}

func (c *Client) CreateCloudProject(ctx context.Context, req *CloudProjectCreateRequest) (*CloudProjectCreateResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("cloud project request is required")
	}
	body, err := c.doWithTokenBody(ctx, "POST", "/api/cloud/project", func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		bodyReq := *req
		bodyReq.TarobaseToken = token
		return &bodyReq, nil
	})
	if err != nil {
		return nil, err
	}

	var resp CloudProjectCreateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetPolicy(ctx context.Context, projectID string) (*PolicyStateResponse, error) {
	path := fmt.Sprintf("/api/project/%s/policy", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp PolicyStateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) ValidatePolicy(ctx context.Context, projectID string, req *PolicyRequest) (*PolicyDeployResult, error) {
	path := fmt.Sprintf("/api/project/%s/policy/validate", projectID)
	body, err := c.policyRequest(ctx, projectID, "POST", path, req, false)
	if err != nil {
		return nil, err
	}

	var resp PolicyDeployResult
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) DeployPolicy(ctx context.Context, projectID string, req *PolicyRequest) (*PolicyDeployResult, error) {
	path := fmt.Sprintf("/api/project/%s/policy/deploy", projectID)
	body, err := c.policyRequest(ctx, projectID, "POST", path, req, true)
	if err != nil {
		return nil, err
	}

	var resp PolicyDeployResult
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) RollbackPolicy(ctx context.Context, projectID string, req *PolicyRequest) (*PolicyDeployResult, error) {
	path := fmt.Sprintf("/api/project/%s/policy/rollback", projectID)
	body, err := c.policyRequest(ctx, projectID, "POST", path, req, true)
	if err != nil {
		return nil, err
	}

	var resp PolicyDeployResult
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) PolicyHistory(ctx context.Context, projectID string, limit int) (*PolicyHistoryResponse, error) {
	path := fmt.Sprintf("/api/project/%s/policy/history?limit=%d", projectID, limit)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp PolicyHistoryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) policyRequest(ctx context.Context, projectID, method, path string, req *PolicyRequest, sign bool) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("policy request is required")
	}
	bodyReq := *req
	if sign && bodyReq.Environment != "" && bodyReq.Environment != "draft" && !bodyReq.DryRun && bodyReq.SignedPermitTransaction == "" {
		target := bodyReq.Environment
		if target == "preview" || target == "production" {
			signedPermit, err := c.getSignedPermitForDeploy(ctx, projectID, target)
			if err != nil {
				return nil, fmt.Errorf("failed to generate policy deploy permit: %w", err)
			}
			bodyReq.SignedPermitTransaction = signedPermit
		}
	}

	return c.doWithTokenBody(ctx, method, path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		retryReq := bodyReq
		retryReq.TarobaseToken = token
		return &retryReq, nil
	})
}
