package tarobase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRunQuery_SingleQueryRoundTrip(t *testing.T) {
	var received queriesRequest
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/queries": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &received)
			_, _ = io.WriteString(w, `{"queries":[{"path":"queries/getSolBalance","queryName":"getSolBalance","queryArgs":{"address":"A"},"result":"12345"}]}`)
		},
	})
	defer cleanup()
	result, err := client.RunQuery(context.Background(), "getSolBalance", map[string]any{"address": "A"})
	if err != nil {
		t.Fatalf("RunQuery: %v", err)
	}
	if result.QueryName != "getSolBalance" {
		t.Errorf("queryName mismatch: %q", result.QueryName)
	}
	if strings.TrimSpace(string(result.Result)) != `"12345"` {
		t.Errorf("result mismatch: %s", result.Result)
	}
	// Server should have seen exactly one query with the right path.
	if len(received.Queries) != 1 || received.Queries[0].Path != "queries/getSolBalance" {
		t.Errorf("server saw wrong request: %+v", received)
	}
}

func TestRunQuery_DefaultsArgsToEmpty(t *testing.T) {
	var received queriesRequest
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/queries": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &received)
			_, _ = io.WriteString(w, `{"queries":[{"path":"queries/x","queryName":"x","queryArgs":{},"result":"1"}]}`)
		},
	})
	defer cleanup()
	if _, err := client.RunQuery(context.Background(), "x", nil); err != nil {
		t.Fatalf("RunQuery: %v", err)
	}
	if got := received.Queries[0].QueryArgs; got == nil {
		t.Error("queryArgs should be non-nil (empty map), got nil")
	}
}

func TestRunQueryMany_RequiresNonEmpty(t *testing.T) {
	client, cleanup := mockServer(t, nil)
	defer cleanup()
	if _, err := client.RunQueryMany(context.Background(), nil); err == nil {
		t.Error("expected error on empty queries list")
	}
}

func TestRunQueryMany_ValidatesFields(t *testing.T) {
	client, cleanup := mockServer(t, nil)
	defer cleanup()
	_, err := client.RunQueryMany(context.Background(), []Query{{Path: "", QueryName: "x"}})
	if err == nil {
		t.Error("expected error for missing path")
	}
}
