package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type ClientAnalyticsTimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Range string `json:"range"`
}

type ClientAnalyticsSummary struct {
	Events            float64 `json:"events"`
	PageViews         float64 `json:"pageViews"`
	RouteViews        float64 `json:"routeViews"`
	Visitors          float64 `json:"visitors"`
	Sessions          float64 `json:"sessions"`
	Errors            float64 `json:"errors"`
	APIErrors         float64 `json:"apiErrors"`
	ResourceErrors    float64 `json:"resourceErrors"`
	JSErrors          float64 `json:"jsErrors"`
	AverageDurationMs float64 `json:"averageDurationMs"`
	AverageTTFBMs     float64 `json:"averageTtfbMs"`
	AverageFCPMs      float64 `json:"averageFcpMs"`
	AverageLCPMs      float64 `json:"averageLcpMs"`
	AverageINPMs      float64 `json:"averageInpMs"`
	AverageCLS        float64 `json:"averageCls"`
	EngagedSeconds    float64 `json:"engagedSeconds"`
}

type ClientAnalyticsTimeSeriesPoint struct {
	Timestamp  string  `json:"timestamp"`
	Events     float64 `json:"events"`
	PageViews  float64 `json:"pageViews"`
	RouteViews float64 `json:"routeViews"`
	Errors     float64 `json:"errors"`
	Visitors   float64 `json:"visitors"`
	Sessions   float64 `json:"sessions"`
}

type ClientAnalyticsPageBreakdown struct {
	Path     string  `json:"path"`
	Events   float64 `json:"events"`
	Visitors float64 `json:"visitors"`
	Errors   float64 `json:"errors"`
}

type ClientAnalyticsErrorBreakdown struct {
	Event        string  `json:"event"`
	FailureClass string  `json:"failureClass"`
	Diagnostic   string  `json:"diagnostic"`
	Path         string  `json:"path"`
	Count        float64 `json:"count"`
	LastSeen     *string `json:"lastSeen"`
}

type ClientAnalyticsDimensionBreakdown struct {
	Value    string  `json:"value"`
	Events   float64 `json:"events"`
	Visitors float64 `json:"visitors"`
}

type ClientAnalyticsMetadata struct {
	FetchedAt  string `json:"fetchedAt"`
	DataSource string `json:"dataSource"`
	Message    string `json:"message,omitempty"`
}

type ClientAppAnalyticsResponse struct {
	ProjectID   string                              `json:"projectId"`
	Environment string                              `json:"environment"`
	SiteIDs     []string                            `json:"siteIds"`
	Dataset     string                              `json:"dataset"`
	TimeRange   ClientAnalyticsTimeRange            `json:"timeRange"`
	Summary     ClientAnalyticsSummary              `json:"summary"`
	TimeSeries  []ClientAnalyticsTimeSeriesPoint    `json:"timeSeries"`
	TopPages    []ClientAnalyticsPageBreakdown      `json:"topPages"`
	Errors      []ClientAnalyticsErrorBreakdown     `json:"errors"`
	Devices     []ClientAnalyticsDimensionBreakdown `json:"devices"`
	Countries   []ClientAnalyticsDimensionBreakdown `json:"countries"`
	Referrers   []ClientAnalyticsDimensionBreakdown `json:"referrers"`
	Metadata    ClientAnalyticsMetadata             `json:"metadata"`
}

func (r *ClientAppAnalyticsResponse) QuietString() string {
	return fmt.Sprintf(
		"%s events=%.0f pageViews=%.0f errors=%.0f",
		r.ProjectID,
		r.Summary.Events,
		r.Summary.PageViews,
		r.Summary.Errors,
	)
}

func (c *Client) GetClientAppAnalytics(ctx context.Context, projectID, environment, timeRange string, limit int) (*ClientAppAnalyticsResponse, error) {
	normalizedEnvironment, err := normalizeProjectRuntimeEnvironment(environment)
	if err != nil {
		return nil, err
	}

	if timeRange == "" {
		timeRange = "24h"
	}
	switch timeRange {
	case "1h", "6h", "24h", "3d", "7d":
	default:
		return nil, fmt.Errorf("invalid range %q (valid: 1h, 6h, 24h, 3d, 7d)", timeRange)
	}

	params := url.Values{}
	if normalizedEnvironment != "" {
		params.Set("environment", normalizedEnvironment)
	}
	params.Set("range", timeRange)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := fmt.Sprintf("/api/project/%s/client-analytics", projectID)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp ClientAppAnalyticsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
