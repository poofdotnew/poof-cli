package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type TemplatePagination struct {
	Limit   int  `json:"limit"`
	Skip    int  `json:"skip"`
	HasMore bool `json:"hasMore"`
}

type TemplatesResponse struct {
	Templates  []Template         `json:"templates"`
	Pagination TemplatePagination `json:"pagination"`
}

func (c *Client) ListTemplates(ctx context.Context, category, search, sortBy string, limit, skip int) (*TemplatesResponse, error) {
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
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if skip > 0 {
		params.Set("skip", strconv.Itoa(skip))
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
