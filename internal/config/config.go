package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds the CLI configuration.
type Config struct {
	SolanaPrivateKey string
	WalletAddress    string
	PoofEnv          string
	BypassToken      string
	OutputFormat     string // "text", "json", "quiet"
	DefaultProject   string
	SolanaRPCURL     string
}

// Load reads configuration from flags, env vars, .env, and ~/.poof/config.yaml.
func Load() (*Config, error) {
	// Load .env from current directory (ignore if missing)
	_ = godotenv.Load()

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	home, _ := os.UserHomeDir()
	if home != "" {
		v.AddConfigPath(filepath.Join(home, ".poof"))
	}
	v.AutomaticEnv()

	v.SetDefault("POOF_ENV", "production")
	v.SetDefault("OUTPUT_FORMAT", "text")

	_ = v.ReadInConfig()

	cfg := &Config{
		SolanaPrivateKey: coalesce(v.GetString("SOLANA_PRIVATE_KEY"), os.Getenv("SOLANA_PRIVATE_KEY")),
		WalletAddress:    coalesce(v.GetString("SOLANA_WALLET_ADDRESS"), os.Getenv("SOLANA_WALLET_ADDRESS")),
		PoofEnv:          coalesce(v.GetString("POOF_ENV"), os.Getenv("POOF_ENV"), "production"),
		BypassToken:      coalesce(v.GetString("VERCEL_BYPASS_TOKEN"), os.Getenv("VERCEL_BYPASS_TOKEN")),
		OutputFormat:     coalesce(v.GetString("OUTPUT_FORMAT"), "text"),
		DefaultProject:   v.GetString("default_project_id"),
		SolanaRPCURL:     coalesce(v.GetString("SOLANA_RPC_URL"), os.Getenv("SOLANA_RPC_URL"), "https://api.mainnet-beta.solana.com"),
	}

	return cfg, nil
}

// GetEnvironment returns the Environment for the current config.
func (c *Config) GetEnvironment() (Environment, error) {
	env, ok := Environments[c.PoofEnv]
	if !ok {
		return Environment{}, fmt.Errorf("unknown environment %q (valid: production, staging, local)", c.PoofEnv)
	}
	return env, nil
}

// PoofDir returns the path to ~/.poof, creating it if needed.
func PoofDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to current directory if home is unavailable
		fmt.Fprintf(os.Stderr, "Warning: could not determine home directory: %v\n", err)
		home = "."
	}
	dir := filepath.Join(home, ".poof")
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create %s: %v\n", dir, err)
	}
	return dir
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
