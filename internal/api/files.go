package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type FilesResponse struct {
	Files map[string]string `json:"files"`
}

type UpdateFilesRequest struct {
	Files         map[string]string `json:"files"`
	TarobaseToken string            `json:"tarobaseToken"`
}

func (c *Client) GetFiles(ctx context.Context, projectID string) (*FilesResponse, error) {
	path := fmt.Sprintf("/api/project/%s/files", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp FilesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) UpdateFiles(ctx context.Context, projectID string, files map[string]string) error {
	path := fmt.Sprintf("/api/project/%s/files/update", projectID)

	token, err := c.AuthManager.GetToken()
	if err != nil {
		return err
	}

	req := UpdateFilesRequest{
		Files:         files,
		TarobaseToken: token,
	}
	_, err = c.Do(ctx, "POST", path, req)
	return err
}
