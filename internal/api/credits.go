package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type CreditsResponse struct {
	Credits CreditDetails `json:"credits"`
}

type CreditDetails struct {
	Daily DailyCredits `json:"daily"`
	AddOn AddOnCredits `json:"addOn"`
	Total int          `json:"total"`
}

type DailyCredits struct {
	Remaining int    `json:"remaining"`
	Allotted  int    `json:"allotted"`
	ResetsAt  string `json:"resetsAt"`
}

type AddOnCredits struct {
	Remaining int `json:"remaining"`
	Purchased int `json:"purchased"`
}

// x402 payment types

type TopupRequest struct {
	Quantity int `json:"quantity"`
}

type PaymentRequirements struct {
	X402Version int              `json:"x402Version"`
	Accepts     []PaymentAccept  `json:"accepts"`
	PriceUsd    float64          `json:"priceUsd"`
	PriceUsdc   string           `json:"priceUsdc"`
	Credits     int              `json:"credits"`
	Quantity    int              `json:"quantity"`
}

type PaymentAccept struct {
	Scheme  string       `json:"scheme"`
	Network string       `json:"network"`
	Amount  string       `json:"amount"`
	PayTo   string       `json:"payTo"`
	Asset   string       `json:"asset"`
	Extra   PaymentExtra `json:"extra"`
}

type PaymentExtra struct {
	FeePayer string `json:"feePayer"`
}

type TopupResult struct {
	Credits  int    `json:"credits"`
	PriceUsd float64 `json:"priceUsd"`
	TxID     string `json:"txId"`
}

func (c *Client) GetCredits(ctx context.Context) (*CreditsResponse, error) {
	body, err := c.Do(ctx, "GET", "/api/user/credits", nil)
	if err != nil {
		return nil, err
	}

	var resp CreditsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// TopupPhase1 sends the initial topup request (no payment).
// The server responds with 402 containing payment requirements.
func (c *Client) TopupPhase1(ctx context.Context, quantity int) (*PaymentRequirements, error) {
	token, err := c.AuthManager.GetToken()
	if err != nil {
		return nil, fmt.Errorf("auth failed: %w", err)
	}

	reqBody, _ := json.Marshal(TopupRequest{Quantity: quantity})

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/credits/topup", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Wallet-Address", c.AuthManager.WalletAddress())
	req.Header.Set("Content-Type", "application/json")
	if c.BypassToken != "" {
		req.Header.Set("x-vercel-protection-bypass", c.BypassToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Expect 402 with payment requirements
	if resp.StatusCode != http.StatusPaymentRequired && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var reqs PaymentRequirements
	if err := json.Unmarshal(body, &reqs); err != nil {
		return nil, fmt.Errorf("failed to parse payment requirements: %w", err)
	}

	if len(reqs.Accepts) == 0 {
		return nil, fmt.Errorf("no payment methods in 402 response")
	}

	return &reqs, nil
}

// TopupPhase2 sends the payment header to complete the purchase.
func (c *Client) TopupPhase2(ctx context.Context, quantity int, paymentHeader string) (*TopupResult, error) {
	token, err := c.AuthManager.GetToken()
	if err != nil {
		return nil, fmt.Errorf("auth failed: %w", err)
	}

	reqBody, _ := json.Marshal(TopupRequest{Quantity: quantity})

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/credits/topup", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Wallet-Address", c.AuthManager.WalletAddress())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PAYMENT", paymentHeader)
	if c.BypassToken != "" {
		req.Header.Set("x-vercel-protection-bypass", c.BypassToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("payment failed (%d): %s", resp.StatusCode, string(body))
	}

	var result TopupResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse topup result: %w", err)
	}

	return &result, nil
}
