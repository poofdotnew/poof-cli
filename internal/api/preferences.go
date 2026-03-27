package api

import (
	"context"
	"encoding/json"
	"fmt"
)

type PreferencesResponse struct {
	Preferences    map[string]interface{} `json:"preferences"`
	HasMembership  bool                   `json:"hasMembership"`
	MembershipTier string                 `json:"membershipTier"`
}

type SetPreferencesRequest struct {
	Preferences map[string]interface{} `json:"preferences"`
}

func (c *Client) GetPreferences(ctx context.Context) (*PreferencesResponse, error) {
	body, err := c.Do(ctx, "GET", "/api/user/ai-preferences", nil)
	if err != nil {
		return nil, err
	}

	var resp PreferencesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) SetPreferences(ctx context.Context, preferences map[string]interface{}) error {
	req := SetPreferencesRequest{Preferences: preferences}
	_, err := c.Do(ctx, "PUT", "/api/user/ai-preferences", req)
	return err
}
