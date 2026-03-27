package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type CreditsResponse struct {
	Credits CreditDetails `json:"credits"`
}

type CreditDetails struct {
	Daily        DailyCredits        `json:"daily"`
	Subscription SubscriptionCredits `json:"subscription"`
	AddOn        AddOnCredits        `json:"addOn"`
	Total        int                 `json:"total"`
}

type SubscriptionCredits struct {
	Remaining int `json:"remaining"`
	Purchased int `json:"purchased"`
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
	X402Version int             `json:"x402Version"`
	Accepts     []PaymentAccept `json:"accepts"`
	PriceUsd    float64         `json:"priceUsd"`
	PriceUsdc   string          `json:"priceUsdc"`
	Credits     int             `json:"credits"`
	Quantity    int             `json:"quantity"`
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
	Credits  int     `json:"credits"`
	PriceUsd float64 `json:"priceUsd"`
	TxID     string  `json:"txId"`
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
	body, statusCode, err := c.DoRaw(ctx, "POST", "/api/credits/topup", TopupRequest{Quantity: quantity}, nil)
	if err != nil {
		return nil, err
	}

	// Expect 402 with payment requirements (or 200 if already paid)
	if statusCode != http.StatusPaymentRequired && statusCode != http.StatusOK {
		return nil, parseAPIError(body, statusCode)
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
	headers := map[string]string{"X-PAYMENT": paymentHeader}
	body, statusCode, err := c.DoRaw(ctx, "POST", "/api/credits/topup", TopupRequest{Quantity: quantity}, headers)
	if err != nil {
		return nil, err
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("payment failed (%d): %s", statusCode, string(body))
	}

	var result TopupResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse topup result: %w", err)
	}

	return &result, nil
}
