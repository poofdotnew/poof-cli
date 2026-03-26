package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type Domain struct {
	Domain    string `json:"domain"`
	IsDefault bool   `json:"isDefault"`
	Status    string `json:"status"`
}

type DomainsResponse struct {
	Domains []Domain `json:"domains"`
}

type AddDomainRequest struct {
	Domain        string `json:"domain"`
	IsDefault     bool   `json:"isDefault"`
	TarobaseToken string `json:"tarobaseToken"`
}

func (c *Client) GetDomains(ctx context.Context, projectID string) (*DomainsResponse, error) {
	path := fmt.Sprintf("/api/project/%s/domains", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp DomainsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) AddDomain(ctx context.Context, projectID, domain string, isDefault bool) error {
	path := fmt.Sprintf("/api/project/%s/domains", projectID)

	_, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return AddDomainRequest{
			Domain:        domain,
			IsDefault:     isDefault,
			TarobaseToken: token,
		}, nil
	})
	return err
}
