package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// ProjectBankBucket selects which side of the project credit bank a deposit
// or withdrawal touches. Mirrors `ProjectBankBucket` in the server's
// creditService.ts — keep these strings in sync.
type ProjectBankBucket string

const (
	BucketUsage    ProjectBankBucket = "usage"
	BucketChat     ProjectBankBucket = "chat"
	BucketCombined ProjectBankBucket = "combined"
)

// IsValidBucket returns true when v is one of the three accepted buckets.
func IsValidBucket(v string) bool {
	switch ProjectBankBucket(v) {
	case BucketUsage, BucketChat, BucketCombined:
		return true
	}
	return false
}

// PurposeBucketBalance is the per-bucket balance for `usage` and `chat`,
// which carry an isolation flag in addition to the two pools.
type PurposeBucketBalance struct {
	Withdrawable    float64 `json:"withdrawable"`
	NonWithdrawable float64 `json:"nonWithdrawable"`
	Isolated        bool    `json:"isolated"`
}

// CombinedBucketBalance is the per-bucket balance for `combined`, which has
// no isolation flag of its own (it inherits whichever purpose drains it).
type CombinedBucketBalance struct {
	Withdrawable    float64 `json:"withdrawable"`
	NonWithdrawable float64 `json:"nonWithdrawable"`
}

// ProjectBankBalance is the per-bucket shape (no project/owner metadata).
// This is what the server returns nested under `balance` from the deposit
// and withdraw endpoints.
type ProjectBankBalance struct {
	Usage    PurposeBucketBalance  `json:"usage"`
	Chat     PurposeBucketBalance  `json:"chat"`
	Combined CombinedBucketBalance `json:"combined"`
}

// ProjectCreditsResponse is the body returned by GET /api/project/{id}/credits.
// It extends the per-bucket shape with the project id, owner flag, and the
// caller's personal paid balance — fields only present on the read endpoint.
type ProjectCreditsResponse struct {
	ProjectID                string  `json:"projectId"`
	ProjectBankBalance               // embeds Usage / Chat / Combined
	IsOwner                  bool    `json:"isOwner"`
	UserPaidCreditsAvailable float64 `json:"userPaidCreditsAvailable"`
}

// QuietString prints a one-line summary suitable for `--quiet` output.
// Format: "<total> <usage_total> <chat_total> <combined_total>" (whitespace
// separated). Total is the sum of withdrawable+nonWithdrawable across all
// three buckets, clamped at zero per bucket so a transient overdraft can't
// produce a negative report.
func (r *ProjectCreditsResponse) QuietString() string {
	total := bucketSumNN(r.Usage.Withdrawable, r.Usage.NonWithdrawable) +
		bucketSumNN(r.Chat.Withdrawable, r.Chat.NonWithdrawable) +
		bucketSumNN(r.Combined.Withdrawable, r.Combined.NonWithdrawable)
	return fmt.Sprintf(
		"%.2f %.2f %.2f %.2f",
		total,
		bucketSumNN(r.Usage.Withdrawable, r.Usage.NonWithdrawable),
		bucketSumNN(r.Chat.Withdrawable, r.Chat.NonWithdrawable),
		bucketSumNN(r.Combined.Withdrawable, r.Combined.NonWithdrawable),
	)
}

func bucketSumNN(w, nw float64) float64 {
	return clampNonNeg(w) + clampNonNeg(nw)
}

func clampNonNeg(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

// DepositRequest is the POST body for /api/project/{id}/credits/deposit.
type DepositRequest struct {
	Amount int               `json:"amount"`
	Bucket ProjectBankBucket `json:"bucket,omitempty"`
}

// WithdrawRequest is the POST body for /api/project/{id}/credits/withdraw.
type WithdrawRequest struct {
	Amount int               `json:"amount"`
	Bucket ProjectBankBucket `json:"bucket,omitempty"`
}

// DepositResponse mirrors the server's deposit return shape. Note that the
// nested `balance` is the bucket-only shape — it does NOT carry the caller's
// personal paid balance (that's a read-side concern). Re-fetch with
// `GetProjectCredits` if you need the post-deposit personal balance.
type DepositResponse struct {
	Deposited int                `json:"deposited"`
	Bucket    ProjectBankBucket  `json:"bucket"`
	Balance   ProjectBankBalance `json:"balance"`
}

// QuietString prints "<deposited> <bucket>" — useful for shell pipelines
// that want to confirm the amount that actually landed.
func (r *DepositResponse) QuietString() string {
	return fmt.Sprintf("%d %s", r.Deposited, r.Bucket)
}

// WithdrawResponse mirrors the server's withdraw return shape. As with
// `DepositResponse`, the nested `balance` is bucket-only — re-fetch with
// `GetProjectCredits` if the personal paid balance is needed.
type WithdrawResponse struct {
	Withdrawn       int                `json:"withdrawn"`
	Bucket          ProjectBankBucket  `json:"bucket"`
	PaymentRecordID string             `json:"paymentRecordId"`
	Balance         ProjectBankBalance `json:"balance"`
}

// QuietString prints "<withdrawn> <bucket> <paymentRecordId>".
func (r *WithdrawResponse) QuietString() string {
	return fmt.Sprintf("%d %s %s", r.Withdrawn, r.Bucket, r.PaymentRecordID)
}

// GetProjectCredits fetches the project credit bank balance plus the
// caller's personal paid-credit balance.
func (c *Client) GetProjectCredits(ctx context.Context, projectID string) (*ProjectCreditsResponse, error) {
	path := fmt.Sprintf("/api/project/%s/credits", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp ProjectCreditsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// DepositProjectCredits moves credits from the caller's personal paid balance
// into the chosen bucket of the project credit bank. amount must be a
// positive integer (the server floors fractional inputs and rejects ≤ 0).
// Owner-only on the server side; collaborators get a 403.
func (c *Client) DepositProjectCredits(ctx context.Context, projectID string, amount int, bucket ProjectBankBucket) (*DepositResponse, error) {
	path := fmt.Sprintf("/api/project/%s/credits/deposit", projectID)
	body, err := c.Do(ctx, "POST", path, DepositRequest{Amount: amount, Bucket: bucket})
	if err != nil {
		return nil, err
	}

	var resp DepositResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// WithdrawProjectCredits drains the chosen withdrawable bucket back to the
// caller's personal balance as a fresh add-on paymentRecord (6-month expiry).
// Owner-only on the server side; collaborators get a 403.
func (c *Client) WithdrawProjectCredits(ctx context.Context, projectID string, amount int, bucket ProjectBankBucket) (*WithdrawResponse, error) {
	path := fmt.Sprintf("/api/project/%s/credits/withdraw", projectID)
	body, err := c.Do(ctx, "POST", path, WithdrawRequest{Amount: amount, Bucket: bucket})
	if err != nil {
		return nil, err
	}

	var resp WithdrawResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// SetProjectIsolation toggles the per-purpose isolation flags on a project.
// At least one of usage/chat must be non-nil. The server validates these
// fields are booleans and rejects anything else with 400.
//
// usage=true means infra + gas only spend from the project bank (no
// fallback to the owner's personal balance). chat=true is the same for
// AI chat. Both default to false on new projects.
func (c *Client) SetProjectIsolation(ctx context.Context, projectID string, usage *bool, chat *bool) error {
	if usage == nil && chat == nil {
		return fmt.Errorf("SetProjectIsolation: at least one of usage/chat must be set")
	}
	body := map[string]interface{}{}
	if usage != nil {
		body["usageCreditsIsolated"] = *usage
	}
	if chat != nil {
		body["chatCreditsIsolated"] = *chat
	}
	path := fmt.Sprintf("/api/project/%s", projectID)
	_, err := c.Do(ctx, "PUT", path, body)
	return err
}
