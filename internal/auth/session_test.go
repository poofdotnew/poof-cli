package auth

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

// newTestKeypair generates a fresh ed25519 keypair suitable for tests.
func newTestKeypair(t *testing.T) *Keypair {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	secretKey := make([]byte, 64)
	copy(secretKey[:32], priv.Seed())
	copy(secretKey[32:], pub)

	kp, err := LoadKeypair(base58.Encode(secretKey))
	if err != nil {
		t.Fatalf("LoadKeypair failed: %v", err)
	}
	return kp
}

// --- FetchNonce tests ---

func TestFetchNonce_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/nonce" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"nonce": "test-nonce-42"})
	}))
	defer srv.Close()

	sc := &SessionClient{
		AuthURL:    srv.URL,
		AppID:      "test-app",
		HTTPClient: srv.Client(),
	}

	nonce, err := sc.FetchNonce()
	if err != nil {
		t.Fatalf("FetchNonce failed: %v", err)
	}
	if nonce != "test-nonce-42" {
		t.Errorf("nonce mismatch: got %q, want %q", nonce, "test-nonce-42")
	}
}

func TestFetchNonce_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server on fire"))
	}))
	defer srv.Close()

	sc := &SessionClient{
		AuthURL:    srv.URL,
		AppID:      "test-app",
		HTTPClient: srv.Client(),
	}

	_, err := sc.FetchNonce()
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code 500: %v", err)
	}
	if !strings.Contains(err.Error(), "server on fire") {
		t.Errorf("error should contain response body: %v", err)
	}
}

func TestFetchNonce_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	sc := &SessionClient{
		AuthURL:    srv.URL,
		AppID:      "test-app",
		HTTPClient: srv.Client(),
	}

	_, err := sc.FetchNonce()
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse nonce response") {
		t.Errorf("error should mention parse failure: %v", err)
	}
}

// --- CreateSession tests ---

func TestCreateSession_Success(t *testing.T) {
	kp := newTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Decode the request body and verify expected fields.
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["address"] != kp.Address {
			t.Errorf("address mismatch in request: got %q, want %q", body["address"], kp.Address)
		}
		if body["appId"] != "test-app" {
			t.Errorf("appId mismatch: got %q", body["appId"])
		}
		if body["authMethod"] != "phantom" {
			t.Errorf("authMethod mismatch: got %q", body["authMethod"])
		}
		if body["message"] == "" {
			t.Error("message should not be empty")
		}
		if body["signature"] == "" {
			t.Error("signature should not be empty")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Session{
			AccessToken:  "access-tok",
			IDToken:      "id-tok",
			RefreshToken: "refresh-tok",
		})
	}))
	defer srv.Close()

	sc := &SessionClient{
		AuthURL:    srv.URL,
		AppID:      "test-app",
		HTTPClient: srv.Client(),
	}

	session, err := sc.CreateSession(kp, "nonce-xyz")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if session.AccessToken != "access-tok" {
		t.Errorf("AccessToken mismatch: got %q, want %q", session.AccessToken, "access-tok")
	}
	if session.IDToken != "id-tok" {
		t.Errorf("IDToken mismatch: got %q, want %q", session.IDToken, "id-tok")
	}
	if session.RefreshToken != "refresh-tok" {
		t.Errorf("RefreshToken mismatch: got %q, want %q", session.RefreshToken, "refresh-tok")
	}
}

func TestCreateSession_Non200(t *testing.T) {
	kp := newTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad credentials"))
	}))
	defer srv.Close()

	sc := &SessionClient{
		AuthURL:    srv.URL,
		AppID:      "test-app",
		HTTPClient: srv.Client(),
	}

	_, err := sc.CreateSession(kp, "nonce-xyz")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status code 401: %v", err)
	}
	if !strings.Contains(err.Error(), "bad credentials") {
		t.Errorf("error should contain response body: %v", err)
	}
}

// --- RefreshSession tests ---

func TestRefreshSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/refresh" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["refreshToken"] != "old-refresh-tok" {
			t.Errorf("refreshToken mismatch: got %q", body["refreshToken"])
		}
		if body["appId"] != "test-app" {
			t.Errorf("appId mismatch: got %q", body["appId"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Session{
			AccessToken:  "new-access",
			IDToken:      "new-id",
			RefreshToken: "new-refresh",
		})
	}))
	defer srv.Close()

	sc := &SessionClient{
		AuthURL:    srv.URL,
		AppID:      "test-app",
		HTTPClient: srv.Client(),
	}

	session, err := sc.RefreshSession("old-refresh-tok")
	if err != nil {
		t.Fatalf("RefreshSession failed: %v", err)
	}
	if session.AccessToken != "new-access" {
		t.Errorf("AccessToken mismatch: got %q, want %q", session.AccessToken, "new-access")
	}
	if session.IDToken != "new-id" {
		t.Errorf("IDToken mismatch: got %q, want %q", session.IDToken, "new-id")
	}
	if session.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken mismatch: got %q, want %q", session.RefreshToken, "new-refresh")
	}
}

func TestRefreshSession_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("token expired"))
	}))
	defer srv.Close()

	sc := &SessionClient{
		AuthURL:    srv.URL,
		AppID:      "test-app",
		HTTPClient: srv.Client(),
	}

	_, err := sc.RefreshSession("stale-token")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status code 403: %v", err)
	}
	if !strings.Contains(err.Error(), "token expired") {
		t.Errorf("error should contain response body: %v", err)
	}
}
