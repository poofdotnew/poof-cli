package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// PublishEligibility matches the server's nested response shape.
type PublishEligibility struct {
	Status  string                 `json:"status"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Eligible returns true when the server reports status "approved".
func (e *PublishEligibility) Eligible() bool {
	return e.Status == "approved"
}

type publishEligibilityEnvelope struct {
	Data PublishEligibility `json:"data"`
}

// Publish request types per target.

type PublishRequest struct {
	AuthToken               string                 `json:"authToken"`
	SignedPermitTransaction string                 `json:"signedPermitTransaction,omitempty"`
	ProdConstantsOverrides  map[string]interface{} `json:"prodConstantsOverrides,omitempty"`
	ProdConfig              map[string]interface{} `json:"prodConfig,omitempty"`
}

type PreviewPublishRequest struct {
	AuthToken                        string                 `json:"authToken"`
	SignedPermitTransaction          string                 `json:"signedPermitTransaction"`
	AllowedAddresses                 []string               `json:"allowedAddresses,omitempty"`
	MainnetPreviewConstantsOverrides map[string]interface{} `json:"mainnetPreviewConstantsOverrides,omitempty"`
	MainnetPreviewConfig             map[string]interface{} `json:"mainnetPreviewConfig,omitempty"`
}

// PublishOptions holds optional parameters for preview/production deploys.
type PublishOptions struct {
	SignedPermitTransaction string
	AllowedAddresses        []string
	ConstantsOverrides      map[string]interface{}
	Config                  map[string]interface{}
}

type MobilePublishRequest struct {
	Platform          string `json:"platform"`
	AppName           string `json:"appName"`
	AppIconUrl        string `json:"appIconUrl"`
	AppDescription    string `json:"appDescription,omitempty"`
	ThemeColor        string `json:"themeColor,omitempty"`
	IsDraft           bool   `json:"isDraft,omitempty"`
	TargetEnvironment string `json:"targetEnvironment,omitempty"`
}

// Download response types — server wraps in { data: {...} }.

type downloadDataEnvelope struct {
	Data struct {
		DownloadTaskID string `json:"downloadTaskId"`
		ProjectID      string `json:"projectId"`
		Status         string `json:"status"`
	} `json:"data"`
}

type DownloadResponse struct {
	TaskID    string `json:"taskId"`
	ProjectID string `json:"projectId"`
	Status    string `json:"status"`
}

func (r *DownloadResponse) QuietString() string { return r.TaskID }

type downloadURLDataEnvelope struct {
	Data struct {
		DownloadURL string `json:"downloadUrl"`
		ExpiresAt   string `json:"expiresAt"`
		FileName    string `json:"fileName"`
	} `json:"data"`
}

type DownloadURLRequest struct {
	TaskID string `json:"taskId"`
}

type DownloadURLResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
	FileName  string `json:"fileName"`
}

func (r *DownloadURLResponse) QuietString() string { return r.URL }

func (c *Client) CheckPublishEligibility(ctx context.Context, projectID string) (*PublishEligibility, error) {
	path := fmt.Sprintf("/api/project/%s/check-publish-eligibility", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var envelope publishEligibilityEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &envelope.Data, nil
}

func (c *Client) PublishProject(ctx context.Context, projectID, target string, opts ...interface{}) error {
	var path string
	switch target {
	case "preview":
		path = fmt.Sprintf("/api/project/%s/deploy-mainnet-preview", projectID)
	case "production":
		path = fmt.Sprintf("/api/project/%s/deploy-prod", projectID)
	case "mobile":
		path = fmt.Sprintf("/api/project/%s/mobile/publish", projectID)
	default:
		return fmt.Errorf("invalid target %q (valid: preview, production, mobile)", target)
	}

	// Mobile uses a different payload shape (no auth token in body).
	if target == "mobile" {
		if len(opts) == 0 {
			return fmt.Errorf("mobile publish requires a MobilePublishRequest")
		}
		mobileReq, ok := opts[0].(*MobilePublishRequest)
		if !ok {
			return fmt.Errorf("mobile publish requires a *MobilePublishRequest")
		}
		_, err := c.Do(ctx, "POST", path, mobileReq)
		return err
	}

	// Extract opts: first arg can be a string (signedPermit) or *PublishOptions.
	var publishOpts PublishOptions
	if len(opts) > 0 {
		switch v := opts[0].(type) {
		case string:
			publishOpts.SignedPermitTransaction = v
		case *PublishOptions:
			if v != nil {
				publishOpts = *v
			}
		}
	}

	_, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		if target == "preview" {
			return PreviewPublishRequest{
				AuthToken:                        token,
				SignedPermitTransaction:          publishOpts.SignedPermitTransaction,
				AllowedAddresses:                 publishOpts.AllowedAddresses,
				MainnetPreviewConstantsOverrides: publishOpts.ConstantsOverrides,
				MainnetPreviewConfig:             publishOpts.Config,
			}, nil
		}
		// production
		return PublishRequest{
			AuthToken:               token,
			SignedPermitTransaction: publishOpts.SignedPermitTransaction,
			ProdConstantsOverrides:  publishOpts.ConstantsOverrides,
			ProdConfig:              publishOpts.Config,
		}, nil
	})
	return err
}

func (c *Client) DownloadCode(ctx context.Context, projectID string) (*DownloadResponse, error) {
	path := fmt.Sprintf("/api/project/%s/download", projectID)
	body, err := c.Do(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}

	var envelope downloadDataEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &DownloadResponse{
		TaskID:    envelope.Data.DownloadTaskID,
		ProjectID: envelope.Data.ProjectID,
		Status:    envelope.Data.Status,
	}, nil
}

func (c *Client) GetDownloadURL(ctx context.Context, projectID, taskID string) (*DownloadURLResponse, error) {
	path := fmt.Sprintf("/api/project/%s/download/get-signed-url", projectID)
	body, err := c.Do(ctx, "POST", path, DownloadURLRequest{TaskID: taskID})
	if err != nil {
		return nil, err
	}

	var envelope downloadURLDataEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &DownloadURLResponse{
		URL:       envelope.Data.DownloadURL,
		ExpiresAt: envelope.Data.ExpiresAt,
		FileName:  envelope.Data.FileName,
	}, nil
}
