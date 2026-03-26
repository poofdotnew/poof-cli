package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-tron/base58"
)

// mockSessionServer creates a test server that handles nonce and session endpoints.
func mockSessionServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/nonce":
			json.NewEncoder(w).Encode(map[string]string{"nonce": "test-nonce"})
		case "/session":
			json.NewEncoder(w).Encode(Session{
				AccessToken:  "access-tok",
				IDToken:      "id-tok",
				RefreshToken: "refresh-tok",
			})
		case "/session/refresh":
			json.NewEncoder(w).Encode(Session{
				AccessToken:  "refreshed-access",
				IDToken:      "refreshed-id",
				RefreshToken: "refreshed-refresh",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestNewManager_Success(t *testing.T) {
	kp := newTestKeypair(t)
	srv := mockSessionServer(t)
	defer srv.Close()

	mgr, err := NewManager(keyToBase58(kp), srv.URL, "app-id", "production")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if mgr.WalletAddress() != kp.Address {
		t.Errorf("expected wallet=%s, got %s", kp.Address, mgr.WalletAddress())
	}
}

func TestNewManager_InvalidKey(t *testing.T) {
	_, err := NewManager("invalid-key!!!", "http://localhost", "app-id", "production")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "failed to load keypair") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_Login(t *testing.T) {
	kp := newTestKeypair(t)
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	srv := mockSessionServer(t)
	defer srv.Close()

	mgr, err := NewManager(keyToBase58(kp), srv.URL, "app-id", "production")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	token, err := mgr.Login()
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if token != "id-tok" {
		t.Errorf("expected token='id-tok', got %q", token)
	}

	// Verify tokens were cached to disk.
	cached, err := LoadCachedTokens()
	if err != nil {
		t.Fatalf("LoadCachedTokens failed after login: %v", err)
	}
	if cached.IDToken != "id-tok" {
		t.Errorf("cached IDToken=%q, want 'id-tok'", cached.IDToken)
	}
}

func TestManager_GetToken_InMemory(t *testing.T) {
	kp := newTestKeypair(t)
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	srv := mockSessionServer(t)
	defer srv.Close()

	mgr, err := NewManager(keyToBase58(kp), srv.URL, "app-id", "production")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// First call: full login
	token1, err := mgr.GetToken()
	if err != nil {
		t.Fatalf("GetToken (first) failed: %v", err)
	}

	// Second call: should use in-memory session (no server call needed)
	token2, err := mgr.GetToken()
	if err != nil {
		t.Fatalf("GetToken (second) failed: %v", err)
	}
	if token1 != token2 {
		t.Errorf("expected same token from in-memory cache, got %q vs %q", token1, token2)
	}
}

func TestManager_GetToken_FromDiskCache(t *testing.T) {
	kp := newTestKeypair(t)
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	srv := mockSessionServer(t)
	defer srv.Close()

	// Pre-populate the disk cache with valid tokens.
	session := &Session{
		IDToken:      "cached-id-tok",
		AccessToken:  "cached-access-tok",
		RefreshToken: "cached-refresh-tok",
	}
	if err := SaveCachedTokens(session, kp.Address, "production"); err != nil {
		t.Fatalf("SaveCachedTokens failed: %v", err)
	}

	mgr, err := NewManager(keyToBase58(kp), srv.URL, "app-id", "production")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	token, err := mgr.GetToken()
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}
	if token != "cached-id-tok" {
		t.Errorf("expected cached token 'cached-id-tok', got %q", token)
	}
}

func TestManager_GetToken_RefreshExpired(t *testing.T) {
	kp := newTestKeypair(t)
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	srv := mockSessionServer(t)
	defer srv.Close()

	// Pre-populate with expired tokens that have a refresh token.
	ct := CachedTokens{
		IDToken:      "expired-id",
		AccessToken:  "expired-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired
		Wallet:       kp.Address,
		Environment:  "production",
	}
	data, _ := json.MarshalIndent(ct, "", "  ")
	cachePath := filepath.Join(tmpDir, ".poof", tokenCacheFile)
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		t.Fatalf("failed to write expired cache: %v", err)
	}

	mgr, err := NewManager(keyToBase58(kp), srv.URL, "app-id", "production")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	token, err := mgr.GetToken()
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}
	// Should have refreshed and got "refreshed-id" from the mock server.
	if token != "refreshed-id" {
		t.Errorf("expected refreshed token 'refreshed-id', got %q", token)
	}
}

func TestManager_InvalidateToken(t *testing.T) {
	kp := newTestKeypair(t)
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	srv := mockSessionServer(t)
	defer srv.Close()

	mgr, err := NewManager(keyToBase58(kp), srv.URL, "app-id", "production")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Login to populate in-memory session.
	_, err = mgr.Login()
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Invalidate clears in-memory session.
	mgr.InvalidateToken()

	// Next GetToken should require re-auth (will use disk cache or re-login).
	token, err := mgr.GetToken()
	if err != nil {
		t.Fatalf("GetToken after invalidate failed: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token after invalidate + re-auth")
	}
}

func TestManager_WalletAddress(t *testing.T) {
	kp := newTestKeypair(t)
	srv := mockSessionServer(t)
	defer srv.Close()

	mgr, err := NewManager(keyToBase58(kp), srv.URL, "app-id", "production")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if mgr.WalletAddress() != kp.Address {
		t.Errorf("expected %s, got %s", kp.Address, mgr.WalletAddress())
	}
}

// keyToBase58 converts a Keypair back to a base58-encoded 64-byte secret key.
func keyToBase58(kp *Keypair) string {
	secretKey := make([]byte, 64)
	copy(secretKey[:32], kp.PrivateKey.Seed())
	copy(secretKey[32:], kp.PublicKey)
	return base58.Encode(secretKey)
}
