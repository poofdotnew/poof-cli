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

func TestGetClientAppAnalytics_RejectsBadRange(t *testing.T) {
	client := newTestClient("http://example.test", &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.GetClientAppAnalytics(context.Background(), "proj-1", "draft", "30d", 10)
	if err == nil {
		t.Fatal("expected invalid range error")
	}
}
