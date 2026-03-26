package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setHomeDir overrides HOME so that config.PoofDir() returns tmpDir/.poof.
// It returns a cleanup function that restores the original HOME.
func setHomeDir(t *testing.T, tmpDir string) {
	t.Helper()
	orig := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	// Ensure the .poof directory exists under the temp HOME.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".poof"), 0700); err != nil {
		t.Fatalf("failed to create .poof dir: %v", err)
	}
	_ = orig // t.Setenv handles restore automatically
}

func TestSaveAndLoadCachedTokens_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	session := &Session{
		IDToken:      "id-tok-123",
		AccessToken:  "access-tok-456",
		RefreshToken: "refresh-tok-789",
	}

	if err := SaveCachedTokens(session, "wallet-abc", "production"); err != nil {
		t.Fatalf("SaveCachedTokens failed: %v", err)
	}

	loaded, err := LoadCachedTokens()
	if err != nil {
		t.Fatalf("LoadCachedTokens failed: %v", err)
	}

	if loaded.IDToken != session.IDToken {
		t.Errorf("IDToken mismatch: got %q, want %q", loaded.IDToken, session.IDToken)
	}
	if loaded.AccessToken != session.AccessToken {
		t.Errorf("AccessToken mismatch: got %q, want %q", loaded.AccessToken, session.AccessToken)
	}
	if loaded.RefreshToken != session.RefreshToken {
		t.Errorf("RefreshToken mismatch: got %q, want %q", loaded.RefreshToken, session.RefreshToken)
	}
	if loaded.Wallet != "wallet-abc" {
		t.Errorf("Wallet mismatch: got %q, want %q", loaded.Wallet, "wallet-abc")
	}
	if loaded.Environment != "production" {
		t.Errorf("Environment mismatch: got %q, want %q", loaded.Environment, "production")
	}

	// ExpiresAt should be roughly tokenTTL from now (55 minutes).
	expectedMin := time.Now().Add(54 * time.Minute)
	expectedMax := time.Now().Add(56 * time.Minute)
	if loaded.ExpiresAt.Before(expectedMin) || loaded.ExpiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt out of expected range: got %v", loaded.ExpiresAt)
	}
}

func TestLoadCachedTokens_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	_, err := LoadCachedTokens()
	if err == nil {
		t.Fatal("expected error when loading from non-existent file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

func TestClearCachedTokens_RemovesFile(t *testing.T) {
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	// Save tokens first so the file exists.
	session := &Session{
		IDToken:      "id",
		AccessToken:  "access",
		RefreshToken: "refresh",
	}
	if err := SaveCachedTokens(session, "w", "e"); err != nil {
		t.Fatalf("SaveCachedTokens failed: %v", err)
	}

	// Verify the file is there.
	cachePath := filepath.Join(tmpDir, ".poof", tokenCacheFile)
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("token cache file should exist before clear: %v", err)
	}

	if err := ClearCachedTokens(); err != nil {
		t.Fatalf("ClearCachedTokens failed: %v", err)
	}

	// Verify the file is gone.
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("expected token cache file to be removed, got stat error: %v", err)
	}
}

func TestClearCachedTokens_MissingFileReturnsNil(t *testing.T) {
	tmpDir := t.TempDir()
	setHomeDir(t, tmpDir)

	// File does not exist; ClearCachedTokens should return nil.
	err := ClearCachedTokens()
	if err != nil {
		t.Errorf("ClearCachedTokens on missing file should return nil, got: %v", err)
	}
}

func TestIsValid_CorrectWalletAndEnv(t *testing.T) {
	ct := &CachedTokens{
		Wallet:      "my-wallet",
		Environment: "production",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	if !ct.IsValid("my-wallet", "production") {
		t.Error("IsValid should return true for matching wallet+env with future expiry")
	}
}

func TestIsValid_WrongWallet(t *testing.T) {
	ct := &CachedTokens{
		Wallet:      "my-wallet",
		Environment: "production",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	if ct.IsValid("other-wallet", "production") {
		t.Error("IsValid should return false for wrong wallet")
	}
}

func TestIsValid_WrongEnv(t *testing.T) {
	ct := &CachedTokens{
		Wallet:      "my-wallet",
		Environment: "production",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	if ct.IsValid("my-wallet", "staging") {
		t.Error("IsValid should return false for wrong environment")
	}
}

func TestIsValid_Expired(t *testing.T) {
	ct := &CachedTokens{
		Wallet:      "my-wallet",
		Environment: "production",
		ExpiresAt:   time.Now().Add(-1 * time.Minute),
	}
	if ct.IsValid("my-wallet", "production") {
		t.Error("IsValid should return false for expired token")
	}
}

func TestHasRefreshToken_Present(t *testing.T) {
	ct := &CachedTokens{
		RefreshToken: "some-refresh-token",
	}
	if !ct.HasRefreshToken() {
		t.Error("HasRefreshToken should return true when refresh token is present")
	}
}

func TestHasRefreshToken_Absent(t *testing.T) {
	ct := &CachedTokens{
		RefreshToken: "",
	}
	if ct.HasRefreshToken() {
		t.Error("HasRefreshToken should return false when refresh token is empty")
	}
}
