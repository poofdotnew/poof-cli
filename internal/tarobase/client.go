// Package tarobase is the Go client for the Tarobase data API
// (api.tarobase.com/items, /queries, /app/{appId}/rpc). It's how the
// `poof data ...` subcommands talk to the user-scoped data plane of a
// Poof project — distinct from internal/api which wraps the project-
// management plane on poof.new.
//
// Auth, session creation, signing, and submission are split across files
// in this package so each piece is testable in isolation. Public entry
// points:
//
//   - NewClient: construct a Client bound to a specific tarobase appId
//     and environment (draft/preview/production)
//   - Client.Get / Client.GetMany / Client.SetMany / Client.RunQuery:
//     the data operations surfaced as `poof data ...` subcommands
//   - Resolve: derive (appId, chain) from a Poof project id + env
package tarobase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/poofdotnew/poof-cli/internal/auth"
)

// envOrDefault returns os.Getenv(key) or the fallback when unset/empty. Used
// by mainnet-rpc-url resolution; kept generic in case we grow more env knobs.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Environment names the per-env appid slot on a project's ConnectionInfo.
type Environment string

const (
	EnvDraft      Environment = "draft"      // Poofnet — offchain simulated chain
	EnvPreview    Environment = "preview"    // Mainnet preview — real Solana
	EnvProduction Environment = "production" // Mainnet production — real Solana
)

// Chain is the wire-level distinction the data plane cares about:
// "offchain" means SHA-256 mock signing + Tarobase-RPC submit; "solana_mainnet"
// means real ed25519 signing + Solana RPC submit. The mainnet preview and
// production environments both use "solana_mainnet".
type Chain string

const (
	ChainOffchain Chain = "offchain"
	ChainMainnet  Chain = "solana_mainnet"
)

// Client talks to the Tarobase data plane for a single Poof app environment.
// One Client == one (appId, chain, session). Create a new one per environment;
// the session is pinned to the app at construction.
type Client struct {
	AppID   string
	Chain   Chain
	APIURL  string
	AuthURL string
	Keypair *auth.Keypair

	session    *auth.Session
	httpClient *http.Client
}

// Config holds the inputs needed to construct a Client. Defaults:
// APIURL=https://api.tarobase.com, AuthURL=https://auth.tarobase.com,
// HTTP timeout=30s.
type Config struct {
	AppID      string
	Chain      Chain
	PrivateKey string // base58 secret key
	APIURL     string // optional override
	AuthURL    string // optional override
	Timeout    time.Duration
}

// NewClient constructs a Client and performs the nonce+sign session exchange
// so subsequent Do calls carry a valid Bearer token. cfg is taken by pointer
// because the Config struct is big enough (88B) that passing by value trips
// golangci's gocritic/hugeParam check.
func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is nil")
	}
	if cfg.AppID == "" {
		return nil, fmt.Errorf("AppID is required")
	}
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("PrivateKey is required")
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "https://api.tarobase.com"
	}
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://auth.tarobase.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Chain == "" {
		cfg.Chain = ChainOffchain
	}

	kp, err := auth.LoadKeypair(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("load keypair: %w", err)
	}

	c := &Client{
		AppID:      cfg.AppID,
		Chain:      cfg.Chain,
		APIURL:     cfg.APIURL,
		AuthURL:    cfg.AuthURL,
		Keypair:    kp,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}

	if err := c.login(ctx); err != nil {
		return nil, fmt.Errorf("tarobase session: %w", err)
	}
	return c, nil
}

// WalletAddress is the base58 pubkey of the client's keypair.
func (c *Client) WalletAddress() string { return c.Keypair.Address }

// do is the shared HTTP helper that signs every request with the session
// Bearer + the X-App-Id headers the server requires. Returns the raw body and
// the status code so callers can inspect both (some endpoints respond with
// non-2xx as part of their protocol).
func (c *Client) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.APIURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if c.session != nil {
		req.Header.Set("Authorization", "Bearer "+c.session.IDToken)
	}
	req.Header.Set("X-App-Id", c.AppID)
	req.Header.Set("X-Public-App-Id", c.AppID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

// doExpect is do + "treat non-2xx as error with parsed message."
// Most data endpoints use this shape.
func (c *Client) doExpect(ctx context.Context, method, path string, body any) ([]byte, error) {
	status, raw, err := c.do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, parseServerError(status, raw)
	}
	return raw, nil
}

// ServerError is what tarobase returns in the body on 4xx/5xx. The `trace`
// field carries the variable-by-variable evaluation of the failed predicate,
// which is what we want to surface verbatim when a rule rejects.
type ServerError struct {
	Status    int             `json:"-"`
	Message   string          `json:"message"`
	RequestID string          `json:"requestId,omitempty"`
	Trace     json.RawMessage `json:"trace,omitempty"`
}

func (e *ServerError) Error() string {
	if len(e.Trace) > 0 {
		return fmt.Sprintf("%s (trace: %s)", e.Message, string(e.Trace))
	}
	return e.Message
}

func parseServerError(status int, raw []byte) error {
	var se ServerError
	if err := json.Unmarshal(raw, &se); err != nil || se.Message == "" {
		return fmt.Errorf("tarobase %d: %s", status, string(raw))
	}
	se.Status = status
	return &se
}
