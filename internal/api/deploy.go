package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type PublishEligibility struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

type PublishRequest struct {
	AuthToken string `json:"authToken"`
}

type DownloadResponse struct {
	TaskID string `json:"taskId"`
}

type DownloadURLRequest struct {
	TaskID string `json:"taskId"`
}

type DownloadURLResponse struct {
	URL string `json:"url"`
}

func (c *Client) CheckPublishEligibility(ctx context.Context, projectID string) (*PublishEligibility, error) {
	path := fmt.Sprintf("/api/project/%s/check-publish-eligibility", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp PublishEligibility
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) PublishProject(ctx context.Context, projectID, target string) error {
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

	token, err := c.AuthManager.GetToken()
	if err != nil {
		return err
	}

	_, err = c.Do(ctx, "POST", path, PublishRequest{AuthToken: token})
	return err
}

func (c *Client) DownloadCode(ctx context.Context, projectID string) (*DownloadResponse, error) {
	path := fmt.Sprintf("/api/project/%s/download", projectID)
	body, err := c.Do(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}

	var resp DownloadResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetDownloadURL(ctx context.Context, projectID, taskID string) (*DownloadURLResponse, error) {
	path := fmt.Sprintf("/api/project/%s/download/get-signed-url", projectID)
	body, err := c.Do(ctx, "POST", path, DownloadURLRequest{TaskID: taskID})
	if err != nil {
		return nil, err
	}

	var resp DownloadURLResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
