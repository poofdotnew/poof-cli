package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type FilesResponse struct {
	FilesWithContent map[string]string `json:"filesWithContent"`
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

	_, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return UpdateFilesRequest{
			Files:         files,
			TarobaseToken: token,
		}, nil
	})
	return err
}
