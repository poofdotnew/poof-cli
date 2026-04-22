package tarobase

import (
	"context"
	"encoding/json"
	"fmt"
)

// Query is one entry in a RunQueryMany request. Tarobase's query endpoint
// always takes an array, so even a single query goes through this shape.
type Query struct {
	Path      string         `json:"path"`      // e.g. "queries/getSolBalance"
	QueryName string         `json:"queryName"` // e.g. "getSolBalance"
	QueryArgs map[string]any `json:"queryArgs"` // may be empty but must be present
}

type queriesRequest struct {
	Queries []Query `json:"queries"`
}

// QueryResult is a single row of the response; result is whatever type the
// policy declared for that query (UInt, String, Bool, etc).
type QueryResult struct {
	Path      string          `json:"path"`
	QueryName string          `json:"queryName"`
	QueryArgs json.RawMessage `json:"queryArgs"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error,omitempty"`
}

type queriesResponse struct {
	Queries []QueryResult `json:"queries"`
}

// RunQueryMany runs a batch of policy queries and returns one result per
// entry, preserving order.
func (c *Client) RunQueryMany(ctx context.Context, qs []Query) ([]QueryResult, error) {
	if len(qs) == 0 {
		return nil, fmt.Errorf("RunQueryMany requires at least one query")
	}
	for i, q := range qs {
		if q.Path == "" || q.QueryName == "" {
			return nil, fmt.Errorf("query %d: path and queryName required", i)
		}
		if q.QueryArgs == nil {
			qs[i].QueryArgs = map[string]any{}
		}
	}
	raw, err := c.doExpect(ctx, "POST", "/queries", queriesRequest{Queries: qs})
	if err != nil {
		return nil, err
	}
	var resp queriesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode queries response: %w", err)
	}
	return resp.Queries, nil
}

// RunQuery is the single-query form. The policy path `queries/$queryId` is
// assumed; callers pass just the queryName (e.g. "getSolBalance") and args.
func (c *Client) RunQuery(ctx context.Context, queryName string, args map[string]any) (*QueryResult, error) {
	results, err := c.RunQueryMany(ctx, []Query{{
		Path:      "queries/" + queryName,
		QueryName: queryName,
		QueryArgs: args,
	}})
	if err != nil {
		return nil, err
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != "" {
		return &results[0], fmt.Errorf("%s", results[0].Error)
	}
	return &results[0], nil
}
