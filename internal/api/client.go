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

// Client is the Poof REST API client.
type Client struct {
	BaseURL     string
	AuthManager *auth.Manager
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
	respBody, statusCode, err := c.doRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	// Auto-retry on 401
	if statusCode == http.StatusUnauthorized {
		c.AuthManager.InvalidateToken()
		respBody, statusCode, err = c.doRequest(ctx, method, path, body)
		if err != nil {
			return nil, err
		}
	}

	if statusCode >= 400 {
		return nil, parseAPIError(respBody, statusCode)
	}

	return respBody, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
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
