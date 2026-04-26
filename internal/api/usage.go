package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// StorageData mirrors the per-environment storage block returned by the
// infra-usage endpoint.
type StorageData struct {
	DocumentSizeBytes int64 `json:"documentSizeBytes"`
	DocumentCount     int64 `json:"documentCount"`
	IndexSizeBytes    int64 `json:"indexSizeBytes"`
	FileSizeBytes     int64 `json:"fileSizeBytes"`
	FileCount         int64 `json:"fileCount"`
	TotalSizeBytes    int64 `json:"totalSizeBytes"`
}

// EnvUsage is the per-environment compute + storage breakdown.
type EnvUsage struct {
	Requests   int64        `json:"requests"`
	CpuTimeMs  int64        `json:"cpuTimeMs"`
	WallTimeMs int64        `json:"wallTimeMs"`
	Storage    *StorageData `json:"storage,omitempty"`
}

// EnvBreakdown holds the three deploy-target environments. Each is optional
// so we can render "no activity" rows for environments that haven't been
// hit this billing period.
type EnvBreakdown struct {
	Development    *EnvUsage `json:"development,omitempty"`
	MainnetPreview *EnvUsage `json:"mainnetPreview,omitempty"`
	Production     *EnvUsage `json:"production,omitempty"`
}

// UsageStatus is one of the three states the server returns for a project's
// monthly usage: ok / warning / exceeded. exceeded means past the free tier.
type UsageStatus string

const (
	UsageStatusOK       UsageStatus = "ok"
	UsageStatusWarning  UsageStatus = "warning"
	UsageStatusExceeded UsageStatus = "exceeded"
)

// BlockedReason is one of the typed reasons the server attaches when a
// project has been paused. Empty when the project isn't blocked, or when
// the server couldn't derive the reason.
type BlockedReason string

const (
	BlockedNoOveruseLimit    BlockedReason = "no_overuse_limit"
	BlockedThresholdReached  BlockedReason = "threshold_reached"
	BlockedInsufficientFunds BlockedReason = "insufficient_credits"
)

// InfraUsageResponse mirrors the body returned by GET
// /api/project/{id}/infra-usage. Numeric fields are credits unless their
// names say otherwise (counts, ms, bytes).
//
// IMPORTANT: when the server flags `summaryStale=true` the numeric fields
// are zeros (the usage pipeline failed); callers should fall back to a
// cached value rather than treating zeros as authoritative.
//
// IMPORTANT: when `blockedStatusStale=true` the DENY_KV check errored, so
// `isBlocked` / `canResume` / `blockedReason` MUST NOT be trusted on this
// response. (See infra-usage/route.ts for the contract.)
type InfraUsageResponse struct {
	ProjectID          string `json:"projectId"`
	Period             string `json:"period"` // YYYY-MM
	TotalRequests      int64  `json:"totalRequests"`
	TotalCpuTimeMs     int64  `json:"totalCpuTimeMs"`
	TotalWallTimeMs    int64  `json:"totalWallTimeMs"`
	TotalStorageBytes  int64  `json:"totalStorageBytes"`
	TotalDocumentCount int64  `json:"totalDocumentCount"`
	TotalFileCount     int64  `json:"totalFileCount"`

	ComputeCostCredits float64     `json:"computeCostCredits"`
	StorageCostCredits float64     `json:"storageCostCredits"`
	CostCredits        float64     `json:"costCredits"`
	FreeCreditsApplied float64     `json:"freeCreditsApplied"`
	ChargedCredits     float64     `json:"chargedCredits"`
	PercentUsed        float64     `json:"percentUsed"`
	Status             UsageStatus `json:"status"`

	Environments EnvBreakdown `json:"environments"`
	LastUpdated  *string      `json:"lastUpdated"`

	// Server-side stale flags: callers MUST honour these before acting on
	// any of the corresponding fields.
	SummaryStale       bool `json:"summaryStale,omitempty"`
	BlockedStatusStale bool `json:"blockedStatusStale,omitempty"`

	IsBlocked     bool          `json:"isBlocked,omitempty"`
	CanResume     bool          `json:"canResume,omitempty"`
	BlockedReason BlockedReason `json:"blockedReason,omitempty"`

	// Available paid credits to cover overage. When the project is NOT
	// usage-isolated, this includes the project bank's spendable balance
	// for the 'usage' purpose; when isolated, only the project bank's
	// usage+combined buckets count.
	PaidCreditsRemaining float64  `json:"paidCreditsRemaining"`
	InfraOveruseLimit    *float64 `json:"infraOveruseLimit,omitempty"`
}

// QuietString prints a one-line summary suitable for `--quiet`:
//
//	"<costCredits>/<budget> <status> <isBlocked>"
//
// Budget is freeTier+overuse when set, else just the costCredits ceiling.
func (r *InfraUsageResponse) QuietString() string {
	blocked := "live"
	if r.IsBlocked {
		blocked = "paused"
	}
	return fmt.Sprintf("%.2f %s %s", r.CostCredits, r.Status, blocked)
}

// ResumeResponse is the body returned by POST /api/project/{id}/resume.
type ResumeResponse struct {
	Resumed bool `json:"resumed"`
}

// QuietString prints "ok" when resumed, otherwise blank.
func (r *ResumeResponse) QuietString() string {
	if r.Resumed {
		return "ok"
	}
	return ""
}

// GetInfraUsage fetches the current month's compute / storage / cost
// summary plus the project's blocked state and overuse limit.
func (c *Client) GetInfraUsage(ctx context.Context, projectID string) (*InfraUsageResponse, error) {
	path := fmt.Sprintf("/api/project/%s/infra-usage", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp InfraUsageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// SetInfraOveruseLimit configures the monthly overuse limit on the project.
// Pass a positive number to set the limit, or nil to clear it (the server
// rejects 0 / negative / non-finite).
//
// "Limit" is the credit ceiling beyond the free tier — once costCredits
// passes free + limit, the project is paused at the next checkpoint.
func (c *Client) SetInfraOveruseLimit(ctx context.Context, projectID string, limit *float64) error {
	body := map[string]interface{}{}
	if limit == nil {
		// Explicit JSON null clears the limit on the server.
		body["infraOveruseLimit"] = nil
	} else {
		body["infraOveruseLimit"] = *limit
	}
	path := fmt.Sprintf("/api/project/%s", projectID)
	_, err := c.Do(ctx, "PUT", path, body)
	return err
}

// ResumeProject explicitly unblocks a paused project. Requires that the
// server reports canResume=true — the call returns an error otherwise.
// Owner-only.
func (c *Client) ResumeProject(ctx context.Context, projectID string) (*ResumeResponse, error) {
	path := fmt.Sprintf("/api/project/%s/resume", projectID)
	body, err := c.Do(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}

	var resp ResumeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
