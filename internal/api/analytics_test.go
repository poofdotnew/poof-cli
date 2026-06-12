package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetClientAppAnalytics_BuildsQuery(t *testing.T) {
	expected := ClientAppAnalyticsResponse{
		ProjectID:   "proj-1",
		Environment: "mainnet-preview",
		Dataset:     "poof_client_app_events",
		Summary: ClientAnalyticsSummary{
			Events:    12,
			PageViews: 3,
			Errors:    1,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/project/proj-1/client-analytics" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("environment") != "mainnet-preview" {
			t.Errorf("expected environment=mainnet-preview, got %q", query.Get("environment"))
		}
		if query.Get("range") != "1h" {
			t.Errorf("expected range=1h, got %q", query.Get("range"))
		}
		if query.Get("limit") != "25" {
			t.Errorf("expected limit=25, got %q", query.Get("limit"))
		}
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetClientAppAnalytics(context.Background(), "proj-1", "preview", "1h", 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Environment != "mainnet-preview" {
		t.Errorf("expected mainnet-preview, got %q", resp.Environment)
	}
	if resp.Summary.Events != 12 {
		t.Errorf("expected 12 events, got %.0f", resp.Summary.Events)
	}
}

func TestGetClientAppAnalytics_DecodesP75Fields(t *testing.T) {
	// Raw JSON (not a struct round-trip) so a typo'd json tag cannot
	// self-consistently pass: these names are the server wire contract.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"projectId": "proj-1",
			"environment": "production",
			"summary": {
				"averageLcpMs": 21828,
				"p75TtfbMs": 350.5,
				"p75FcpMs": 1200,
				"p75LcpMs": 2900,
				"p75InpMs": 120,
				"p75Cls": 0.009,
				"ttfbSampleCount": 40,
				"fcpSampleCount": 38,
				"lcpSampleCount": 37,
				"inpSampleCount": 21,
				"clsSampleCount": 44
			}
		}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetClientAppAnalytics(context.Background(), "proj-1", "production", "1h", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Summary.P75LCPMs != 2900 {
		t.Errorf("expected p75LcpMs 2900, got %v", resp.Summary.P75LCPMs)
	}
	if resp.Summary.P75CLS != 0.009 {
		t.Errorf("expected p75Cls 0.009, got %v", resp.Summary.P75CLS)
	}
	if resp.Summary.LCPSampleCount != 37 {
		t.Errorf("expected lcpSampleCount 37, got %v", resp.Summary.LCPSampleCount)
	}
	if resp.Summary.AverageLCPMs != 21828 {
		t.Errorf("expected averageLcpMs 21828, got %v", resp.Summary.AverageLCPMs)
	}
}

func TestGetClientAppAnalytics_RejectsBadRange(t *testing.T) {
	client := newTestClient("http://example.test", &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.GetClientAppAnalytics(context.Background(), "proj-1", "draft", "30d", 10)
	if err == nil {
		t.Fatal("expected invalid range error")
	}
}
