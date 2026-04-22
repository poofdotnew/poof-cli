package tarobase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeKey is a deterministic base58 secret key (64 bytes) used across tests
// so the signing stays reproducible but no real keypair is exposed.
const fakeKey = "4gmezmYFQCkCg8STyjYSyVUxbYXfrQPamrDVEs65f92wPk5QSTp98GaUYBmSt9XKmodpyrzpcVK3k5yaxiWaQvP4"

// mockServer boots a httptest.Server that answers the nonce+session auth
// dance and then dispatches /items, /queries, /app/.../rpc to the provided
// handlers. Returns the Client pointed at it plus a cleanup func.
func mockServer(t *testing.T, routes map[string]http.HandlerFunc) (*Client, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Auth endpoints.
		if r.URL.Path == "/auth/nonce" {
			_ = json.NewEncoder(w).Encode(map[string]string{"nonce": "test-nonce"})
			return
		}
		if r.URL.Path == "/session" {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"idToken":      "test-id-token",
				"accessToken":  "test-access-token",
				"refreshToken": "test-refresh-token",
			})
			return
		}
		if h, ok := routes[r.URL.Path]; ok {
			h(w, r)
			return
		}
		// Match prefix for /app/<id>/rpc which has a dynamic segment.
		if strings.HasPrefix(r.URL.Path, "/app/") && strings.HasSuffix(r.URL.Path, "/rpc") {
			if h, ok := routes["/app/*/rpc"]; ok {
				h(w, r)
				return
			}
		}
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	}))

	client, err := NewClient(context.Background(), Config{
		AppID:      "test-app",
		Chain:      ChainOffchain,
		PrivateKey: fakeKey,
		APIURL:     server.URL,
		AuthURL:    server.URL,
	})
	if err != nil {
		server.Close()
		t.Fatalf("NewClient: %v", err)
	}
	return client, server.Close
}

func TestSetMany_RoundTripsDocumentsAndReturnsRaw(t *testing.T) {
	var received setManyRequest
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/items": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PUT" {
				t.Errorf("expected PUT, got %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &received); err != nil {
				t.Fatalf("decode req: %v", err)
			}
			// Verify required headers.
			if got := r.Header.Get("X-App-Id"); got != "test-app" {
				t.Errorf("X-App-Id header missing, got %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-id-token" {
				t.Errorf("Authorization header wrong: %q", got)
			}
			_, _ = io.WriteString(w, `{"offchainTransaction":{"message":"{\"appId\":\"test-app\"}"}}`)
		},
	})
	defer cleanup()

	raw, err := client.SetMany(context.Background(), []Document{
		{Path: "memories/alice", Document: map[string]any{"content": "hi"}},
		{Path: "memories/bob", Document: map[string]any{"content": "yo"}},
	})
	if err != nil {
		t.Fatalf("SetMany: %v", err)
	}
	if len(received.Documents) != 2 {
		t.Errorf("server saw %d docs, want 2", len(received.Documents))
	}
	if received.Documents[0].Path != "memories/alice" {
		t.Errorf("doc0 path mismatch: %q", received.Documents[0].Path)
	}
	if !json.Valid(raw) {
		t.Errorf("raw response is not valid JSON")
	}
}

func TestSetMany_RejectsEmptyDocumentList(t *testing.T) {
	client, cleanup := mockServer(t, nil)
	defer cleanup()
	if _, err := client.SetMany(context.Background(), nil); err == nil {
		t.Error("expected error for empty doc list")
	}
}

func TestSetMany_RejectsDocumentWithoutPath(t *testing.T) {
	client, cleanup := mockServer(t, nil)
	defer cleanup()
	_, err := client.SetMany(context.Background(), []Document{{Document: map[string]any{"x": 1}}})
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Errorf("expected path-required error, got %v", err)
	}
}

func TestGet_URLEncodesPath(t *testing.T) {
	var seenQuery string
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/items": func(w http.ResponseWriter, r *http.Request) {
			seenQuery = r.URL.RawQuery
			_, _ = io.WriteString(w, `[]`)
		},
	})
	defer cleanup()
	_, err := client.Get(context.Background(), "user/abc/TokenTransfer/tt1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(seenQuery, "path=user%2Fabc%2FTokenTransfer%2Ftt1") {
		t.Errorf("path not URL-encoded: %q", seenQuery)
	}
}

func TestSet_IsOneDocumentSetMany(t *testing.T) {
	var received setManyRequest
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/items": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &received)
			_, _ = io.WriteString(w, `{"offchainTransaction":{"message":"{}"}}`)
		},
	})
	defer cleanup()
	_, err := client.Set(context.Background(), "memories/alice", map[string]any{"content": "hi"})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(received.Documents) != 1 || received.Documents[0].Path != "memories/alice" {
		t.Errorf("Set didn't route through SetMany with 1 doc: %+v", received)
	}
}
