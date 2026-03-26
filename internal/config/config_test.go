package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironments(t *testing.T) {
	expected := []string{"production", "staging", "local"}
	for _, name := range expected {
		env, ok := Environments[name]
		if !ok {
			t.Errorf("missing environment: %s", name)
			continue
		}
		if env.AppID == "" {
			t.Errorf("%s: empty AppID", name)
		}
		if env.BaseURL == "" {
			t.Errorf("%s: empty BaseURL", name)
		}
		if env.AuthURL == "" {
			t.Errorf("%s: empty AuthURL", name)
		}
	}
}

func TestConfig_GetEnvironment(t *testing.T) {
	tests := []struct {
		env     string
		wantErr bool
	}{
		{"production", false},
		{"staging", false},
		{"local", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		cfg := &Config{PoofEnv: tt.env}
		_, err := cfg.GetEnvironment()
		if (err != nil) != tt.wantErr {
			t.Errorf("GetEnvironment(%q): err=%v, wantErr=%v", tt.env, err, tt.wantErr)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars that would override defaults.
	t.Setenv("SOLANA_PRIVATE_KEY", "")
	t.Setenv("SOLANA_WALLET_ADDRESS", "")
	t.Setenv("POOF_ENV", "")
	t.Setenv("VERCEL_BYPASS_TOKEN", "")
	t.Setenv("OUTPUT_FORMAT", "")
	t.Setenv("SOLANA_RPC_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.PoofEnv != "production" {
		t.Errorf("expected default PoofEnv=production, got %q", cfg.PoofEnv)
	}
	if cfg.OutputFormat != "text" {
		t.Errorf("expected default OutputFormat=text, got %q", cfg.OutputFormat)
	}
	if cfg.SolanaRPCURL != "https://api.mainnet-beta.solana.com" {
		t.Errorf("expected default RPC URL, got %q", cfg.SolanaRPCURL)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("POOF_ENV", "staging")
	t.Setenv("SOLANA_PRIVATE_KEY", "test-key-123")
	t.Setenv("SOLANA_WALLET_ADDRESS", "wallet-abc")
	t.Setenv("SOLANA_RPC_URL", "https://custom-rpc.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.PoofEnv != "staging" {
		t.Errorf("expected PoofEnv=staging, got %q", cfg.PoofEnv)
	}
	if cfg.SolanaPrivateKey != "test-key-123" {
		t.Errorf("expected SolanaPrivateKey=test-key-123, got %q", cfg.SolanaPrivateKey)
	}
	if cfg.WalletAddress != "wallet-abc" {
		t.Errorf("expected WalletAddress=wallet-abc, got %q", cfg.WalletAddress)
	}
	if cfg.SolanaRPCURL != "https://custom-rpc.example.com" {
		t.Errorf("expected custom RPC URL, got %q", cfg.SolanaRPCURL)
	}
}

func TestPoofDir_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := PoofDir()
	if !strings.HasSuffix(dir, ".poof") {
		t.Errorf("expected path ending in .poof, got %q", dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("PoofDir directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("PoofDir should be a directory")
	}
}

func TestPoofDir_ExistingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Pre-create the directory.
	poofPath := filepath.Join(tmpDir, ".poof")
	if err := os.MkdirAll(poofPath, 0700); err != nil {
		t.Fatalf("failed to create .poof dir: %v", err)
	}

	dir := PoofDir()
	if dir != poofPath {
		t.Errorf("expected %q, got %q", poofPath, dir)
	}
}

func TestCoalesce(t *testing.T) {
	tests := []struct {
		vals []string
		want string
	}{
		{[]string{"a", "b"}, "a"},
		{[]string{"", "b"}, "b"},
		{[]string{"", "", "c"}, "c"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}

	for _, tt := range tests {
		got := coalesce(tt.vals...)
		if got != tt.want {
			t.Errorf("coalesce(%v) = %q, want %q", tt.vals, got, tt.want)
		}
	}
}
