package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

type SecretsResponse struct {
	SecretRequirements SecretRequirements `json:"secretRequirements"`
	Summary            SecretsSummary     `json:"summary"`
}

type SecretRequirements struct {
	Required []SecretEntry `json:"required"`
	Optional []SecretEntry `json:"optional"`
}

type SecretEntry struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	Description      string `json:"description"`
	Type             string `json:"type"`
	IsRequired       bool   `json:"isRequired"`
	HasValue         bool   `json:"hasValue"`
	Status           string `json:"status"`
	DeprecatedAt     string `json:"deprecatedAt,omitempty"`
	DeprecatedReason string `json:"deprecatedReason,omitempty"`
}

type SecretsSummary struct {
	TotalRequired      int `json:"totalRequired"`
	TotalOptional      int `json:"totalOptional"`
	RequiredWithValues int `json:"requiredWithValues"`
	OptionalWithValues int `json:"optionalWithValues"`
}

type SetSecretsRequest struct {
	Secrets map[string]string `json:"secrets"`
}

func (c *Client) GetSecrets(ctx context.Context, projectID, environment string) (*SecretsResponse, error) {
	normalizedEnvironment, err := normalizeProjectRuntimeEnvironment(environment)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/project/%s/secrets", projectID)
	params := url.Values{}
	if normalizedEnvironment != "" {
		params.Set("environment", normalizedEnvironment)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
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

func (c *Client) SetSecrets(ctx context.Context, projectID string, secrets map[string]string) error {
	path := fmt.Sprintf("/api/project/%s/secrets", projectID)
	req := SetSecretsRequest{Secrets: secrets}
	_, err := c.Do(ctx, "POST", path, req)
	return err
}
