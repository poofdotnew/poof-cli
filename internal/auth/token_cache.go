package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/poofdotnew/poof-cli/internal/config"
)

// CachedTokens is the on-disk token cache format.
type CachedTokens struct {
	IDToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Wallet       string    `json:"wallet_address"`
	Environment  string    `json:"environment"`
}

const tokenCacheFile = "tokens.json"
const tokenTTL = 55 * time.Minute // conservative: actual is ~60 min

func tokenCachePath() string {
	return filepath.Join(config.PoofDir(), tokenCacheFile)
}

// LoadCachedTokens reads the token cache from disk.
func LoadCachedTokens() (*CachedTokens, error) {
	data, err := os.ReadFile(tokenCachePath())
	if err != nil {
		return nil, err
	}

	var ct CachedTokens
	if err := json.Unmarshal(data, &ct); err != nil {
		return nil, err
	}

	return &ct, nil
}

// SaveCachedTokens writes the token cache to disk.
func SaveCachedTokens(session *Session, wallet, env string) error {
	ct := CachedTokens{
		IDToken:      session.IDToken,
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		ExpiresAt:    time.Now().Add(tokenTTL),
		Wallet:       wallet,
		Environment:  env,
	}

	data, err := json.MarshalIndent(ct, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(tokenCachePath(), data, 0600)
}

// ClearCachedTokens removes the token cache.
func ClearCachedTokens() error {
	return os.Remove(tokenCachePath())
}

// IsValid returns true if the cached tokens are still usable.
func (ct *CachedTokens) IsValid(wallet, env string) bool {
	if ct.Wallet != wallet || ct.Environment != env {
		return false
	}
	return time.Now().Before(ct.ExpiresAt)
}

// HasRefreshToken returns true if a refresh token is available.
func (ct *CachedTokens) HasRefreshToken() bool {
	return ct.RefreshToken != ""
}
