package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/poofdotnew/poof-cli/internal/auth"
	"github.com/poofdotnew/poof-cli/internal/config"
)

// AuthProvider is the interface used by Client for authentication.
// *auth.Manager satisfies this interface.
type AuthProvider interface {
	GetToken() (string, error)
	InvalidateToken()
	WalletAddress() string
}

// Client is the Poof REST API client.
type Client struct {
	BaseURL     string
	AuthManager AuthProvider
	BypassToken string
	HTTPClient  *http.Client
}

// NewClient creates a new API client from config.
func NewClient(cfg *config.Config, authMgr *auth.Manager) (*Client, error) {
	env, err := cfg.GetEnvironment()
	if err != nil {
		return nil, err
	}

	return &Client{
		BaseURL:     env.BaseURL,
		AuthManager: authMgr,
		BypassToken: cfg.BypassToken,
		HTTPClient:  &http.Client{},
	}, nil
}

// Do executes an API request with auth headers and automatic 401 retry.
func (c *Client) Do(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	respBody, statusCode, err := c.doRequest(ctx, method, path, body, nil)
	if err != nil {
		return nil, err
	}

	// Auto-retry on 401
	if statusCode == http.StatusUnauthorized {
		c.AuthManager.InvalidateToken()
		respBody, statusCode, err = c.doRequest(ctx, method, path, body, nil)
		if err != nil {
			return nil, err
		}
	}

	if statusCode >= 400 {
		return nil, parseAPIError(respBody, statusCode)
	}

	return respBody, nil
}

// DoRaw executes a request with auth and 401 retry, returning the raw
// response body and status code without treating status >= 400 as errors.
// Use this for endpoints that return non-2xx codes as part of their protocol (e.g. 402).
func (c *Client) DoRaw(ctx context.Context, method, path string, body interface{}, extraHeaders map[string]string) ([]byte, int, error) {
	respBody, statusCode, err := c.doRequest(ctx, method, path, body, extraHeaders)
	if err != nil {
		return nil, 0, err
	}

	// Auto-retry on 401
	if statusCode == http.StatusUnauthorized {
		c.AuthManager.InvalidateToken()
		respBody, statusCode, err = c.doRequest(ctx, method, path, body, extraHeaders)
		if err != nil {
			return nil, 0, err
		}
	}

	return respBody, statusCode, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, extraHeaders map[string]string) ([]byte, int, error) {
	token, err := c.AuthManager.GetToken()
	if err != nil {
		return nil, 0, fmt.Errorf("auth failed: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Wallet-Address", c.AuthManager.WalletAddress())
	req.Header.Set("Content-Type", "application/json")

	if c.BypassToken != "" {
		req.Header.Set("x-vercel-protection-bypass", c.BypassToken)
	}

	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// doWithTokenBody executes a request where the body includes a token field
// (e.g. TarobaseToken). On 401 retry, the body is rebuilt with a fresh token
// to avoid stale token mismatch between the Authorization header and body.
func (c *Client) doWithTokenBody(ctx context.Context, method, path string, buildBody func() (interface{}, error)) ([]byte, error) {
	body, err := buildBody()
	if err != nil {
		return nil, err
	}

	respBody, statusCode, err := c.doRequest(ctx, method, path, body, nil)
	if err != nil {
		return nil, err
	}

	// Auto-retry on 401 with a fresh body (new token)
	if statusCode == http.StatusUnauthorized {
		c.AuthManager.InvalidateToken()
		body, err = buildBody()
		if err != nil {
			return nil, err
		}
		respBody, statusCode, err = c.doRequest(ctx, method, path, body, nil)
		if err != nil {
			return nil, err
		}
	}

	if statusCode >= 400 {
		return nil, parseAPIError(respBody, statusCode)
	}

	return respBody, nil
}

func parseAPIError(body []byte, statusCode int) error {
	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		// If we can't parse the error, return a generic one
		apiErr = APIError{
			StatusCode: statusCode,
			Message:    string(body),
		}
	}
	apiErr.StatusCode = statusCode
	return &apiErr
}
