package api

import (
	"context"
	"encoding/json"
	"fmt"
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

type SecretsStatusResponse struct {
	Success             bool                       `json:"success"`
	StatusByEnvironment SecretsStatusByEnvironment `json:"statusByEnvironment"`
}

type SecretsStatusByEnvironment struct {
	Development    EnvironmentSecretsStatus `json:"development"`
	MainnetPreview EnvironmentSecretsStatus `json:"mainnet-preview"`
	Production     EnvironmentSecretsStatus `json:"production"`
}

type EnvironmentSecretsStatus struct {
	AppID   string   `json:"appId"`
	Secrets []string `json:"secrets"`
}

func (r *SecretsStatusResponse) SecretsForEnvironment(environment string) []string {
	if r == nil {
		return nil
	}
	switch environment {
	case "production":
		return r.StatusByEnvironment.Production.Secrets
	case "mainnet-preview":
		return r.StatusByEnvironment.MainnetPreview.Secrets
	default:
		return r.StatusByEnvironment.Development.Secrets
	}
}

type SetSecretsRequest struct {
	Secrets     map[string]string `json:"secrets"`
	Environment string            `json:"environment,omitempty"`
}

func (c *Client) GetSecrets(ctx context.Context, projectID, environment string) (*SecretsResponse, error) {
	path := fmt.Sprintf("/api/project/%s/secrets", projectID)
	if environment != "" {
		path += "?environment=" + environment
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

func (c *Client) SetSecrets(ctx context.Context, projectID, environment string, secrets map[string]string) error {
	path := fmt.Sprintf("/api/project/%s/secret-ticket", projectID)
	req := SetSecretsRequest{Secrets: secrets, Environment: environment}
	_, err := c.Do(ctx, "POST", path, req)
	return err
}

func (c *Client) GetSecretsStatus(ctx context.Context, projectID string) (*SecretsStatusResponse, error) {
	path := fmt.Sprintf("/api/project/%s/secrets/status", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp SecretsStatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
