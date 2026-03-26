package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type SecretsResponse struct {
	Secrets SecretsList `json:"secrets"`
}

type SecretsList struct {
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

type SetSecretsRequest struct {
	Secrets     map[string]string `json:"secrets"`
	Environment string            `json:"environment,omitempty"`
}

func (c *Client) GetSecrets(ctx context.Context, projectID string) (*SecretsResponse, error) {
	path := fmt.Sprintf("/api/project/%s/secrets", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp SecretsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) SetSecrets(ctx context.Context, projectID string, secrets map[string]string, environment string) error {
	path := fmt.Sprintf("/api/project/%s/secrets", projectID)
	req := SetSecretsRequest{Secrets: secrets, Environment: environment}
	_, err := c.Do(ctx, "POST", path, req)
	return err
}
