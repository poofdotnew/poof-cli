package tarobase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
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

// GetOptions tunes a Get call. All fields are optional; a nil or zero-valued
// GetOptions means "list the path as-is, no filter". The knobs here mirror
// the @pooflabs/web / tarobase-core SDK's GetOptions so behavior is
// consistent across the CLI and the app-runtime SDK.
type GetOptions struct {
	// Prompt is a natural-language filter evaluated server-side (e.g.
	// "most recent 20 messages, newest first"). Only meaningful on
	// collection paths. Sent as a base64-encoded query param, matching
	// the SDK's safeBtoa(prompt) wire format.
	Prompt string
	// Limit caps the number of returned documents. Applies to collection
	// paths; ignored on document paths. Zero means "no explicit limit".
	Limit int
	// Cursor is an opaque pagination cursor returned by a prior Get.
	// Pair with Limit to page through a collection.
	Cursor string
	// IncludeSubPaths widens a collection read to also include documents
	// under nested sub-collections of the same path.
	IncludeSubPaths bool
	// Shape is a JSON object describing which related documents to
	// resolve alongside each hit. Serialized verbatim into the `shape`
	// query param.
	Shape json.RawMessage
}

// Get reads a single document or collection by path. Tarobase returns an
// array — a document hit shows up as a single-element array, a collection
// path returns the matching docs. Read rules that reject filter entries out
// of the result rather than throwing, so a `200 []` can mean either "no docs"
// or "docs exist but your read rule denies them".
//
// opts is optional; pass nil for a plain list. See GetOptions for the
// available filter/pagination knobs.
func (c *Client) Get(ctx context.Context, path string, opts *GetOptions) (json.RawMessage, error) {
	q := "/items?path=" + url.QueryEscape(path)
	if opts != nil {
		if opts.Prompt != "" {
			q += "&prompt=" + base64.StdEncoding.EncodeToString([]byte(opts.Prompt))
		}
		if opts.Limit > 0 {
			q += "&limit=" + strconv.Itoa(opts.Limit)
		}
		if opts.Cursor != "" {
			q += "&cursor=" + url.QueryEscape(opts.Cursor)
		}
		if opts.IncludeSubPaths {
			q += "&includeSubPaths=true"
		}
		if len(opts.Shape) > 0 {
			q += "&shape=" + url.QueryEscape(string(opts.Shape))
		}
	}
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
		r, err := c.Get(ctx, p, nil)
		if err != nil {
			return nil, fmt.Errorf("path %d (%s): %w", i, p, err)
		}
		out[i] = r
	}
	return out, nil
}
