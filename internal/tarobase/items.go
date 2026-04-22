package tarobase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Document is one entry in a SetMany request: a Tarobase path plus the data
// to write there. For a single-doc write use SetMany with a one-element slice
// or the Set helper below.
type Document struct {
	Path     string         `json:"destinationPath"`
	Document map[string]any `json:"document"`
}

// setManyRequest is the `PUT /items` body.
type setManyRequest struct {
	Documents []Document `json:"documents"`
}

// SetMany writes one or more documents atomically. If any rule or hook
// fails, the whole bundle rejects. The returned raw JSON is the server's
// build-transaction response — an `offchainTransaction` (poofnet) or a
// `transactions` array (mainnet); the caller hands it to Submit* to actually
// sign and land it.
func (c *Client) SetMany(ctx context.Context, docs []Document) (json.RawMessage, error) {
	if len(docs) == 0 {
		return nil, fmt.Errorf("SetMany requires at least one document")
	}
	for i, d := range docs {
		if d.Path == "" {
			return nil, fmt.Errorf("document %d: path is required", i)
		}
		if d.Document == nil {
			return nil, fmt.Errorf("document %d: document is required", i)
		}
	}
	raw, err := c.doExpect(ctx, "PUT", "/items", setManyRequest{Documents: docs})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// Set is the single-document form. Equivalent to SetMany with one entry.
func (c *Client) Set(ctx context.Context, path string, document map[string]any) (json.RawMessage, error) {
	return c.SetMany(ctx, []Document{{Path: path, Document: document}})
}

// Get reads a single document or collection by path. Tarobase returns an
// array — a document hit shows up as a single-element array, a collection
// path returns the matching docs. Read rules that reject filter entries out
// of the result rather than throwing, so a `200 []` can mean either "no docs"
// or "docs exist but your read rule denies them".
func (c *Client) Get(ctx context.Context, path string) (json.RawMessage, error) {
	q := "/items?path=" + url.QueryEscape(path)
	raw, err := c.doExpect(ctx, "GET", q, nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// GetMany fans out Get for each path. Tarobase's read endpoint is one-path-
// per-request, so this is a convenience; inputs are returned in order.
func (c *Client) GetMany(ctx context.Context, paths []string) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, len(paths))
	for i, p := range paths {
		r, err := c.Get(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("path %d (%s): %w", i, p, err)
		}
		out[i] = r
	}
	return out, nil
}
