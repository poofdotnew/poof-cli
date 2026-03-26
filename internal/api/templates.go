package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type TemplatesResponse struct {
	Templates []Template `json:"templates"`
}

func (c *Client) ListTemplates(ctx context.Context, category, search, sortBy string) (*TemplatesResponse, error) {
	params := url.Values{}
	if category != "" {
		params.Set("category", category)
	}
	if search != "" {
		params.Set("search", search)
	}
	if sortBy != "" {
		params.Set("sortBy", sortBy)
	}

	path := "/api/template"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp TemplatesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
