package tarobase

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/poofdotnew/poof-cli/internal/auth"
	"github.com/poofdotnew/poof-cli/internal/config"
)

// On-disk cache of per-(appId, wallet) Tarobase sessions. Lives alongside
// the main poof.new auth cache in ~/.poof/ (separate file so the two auth
// surfaces don't step on each other). Consumers get it through the Client
// transparently — see session.go's login().
//
// Keyed by `appId:wallet`. A wallet-swap without `poof auth logout` won't
// accidentally reuse a stale session for the wrong identity, and running
// `poof data` against multiple environments (draft + preview) with the same
// wallet keeps distinct entries.
//
// Sessions are treated as valid for 55 minutes from issuance — conservative
// vs. the ~60 minute true TTL Cognito enforces. On a 401 the client
// invalidates the entry and re-logs in once.

const sessionCacheFile = "tarobase-sessions.json"

// cacheTTL is how long we treat a freshly-issued session as valid before
// forcing a re-login. Short of Cognito's true lifetime so we never hand the
// server a just-expired token and eat a 401 retry.
const sessionCacheTTL = 55 * time.Minute

type cachedSession struct {
	IDToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Wallet       string    `json:"wallet"`
}

// cacheFileMu serializes read-modify-write on the cache file. The cache is
// a single JSON map; if two `poof data` invocations race they'd otherwise
// clobber each other's writes.
var cacheFileMu sync.Mutex

func sessionCachePath() string {
	return filepath.Join(config.PoofDir(), sessionCacheFile)
}

// sessionCacheKey is the on-disk lookup key. Keeping wallet in the key means
// a wallet-swap doesn't hand you the old wallet's session for the same app.
func sessionCacheKey(appID, wallet string) string {
	return appID + ":" + wallet
}

// loadSessionCache reads the whole cache file. A missing file is not an
// error — callers get an empty map.
func loadSessionCache() (map[string]cachedSession, error) {
	cacheFileMu.Lock()
	defer cacheFileMu.Unlock()
	data, err := os.ReadFile(sessionCachePath())
	if os.IsNotExist(err) {
		return map[string]cachedSession{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session cache: %w", err)
	}
	var cache map[string]cachedSession
	if err := json.Unmarshal(data, &cache); err != nil {
		// Corrupt cache — treat as empty rather than propagate. The next
		// successful login overwrites it cleanly.
		return map[string]cachedSession{}, nil
	}
	return cache, nil
}

// saveSessionCacheEntry upserts one (appId, wallet) entry in the cache.
func saveSessionCacheEntry(appID, wallet string, session *auth.Session) error {
	cache, err := loadSessionCache()
	if err != nil {
		return err
	}
	cache[sessionCacheKey(appID, wallet)] = cachedSession{
		IDToken:      session.IDToken,
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		ExpiresAt:    time.Now().Add(sessionCacheTTL),
		Wallet:       wallet,
	}
	return writeSessionCache(cache)
}

// invalidateSessionCacheEntry drops one key. Called on 401 to force a fresh
// login on the next attempt.
func invalidateSessionCacheEntry(appID, wallet string) {
	cache, err := loadSessionCache()
	if err != nil {
		return
	}
	delete(cache, sessionCacheKey(appID, wallet))
	_ = writeSessionCache(cache)
}

func writeSessionCache(cache map[string]cachedSession) error {
	cacheFileMu.Lock()
	defer cacheFileMu.Unlock()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionCachePath(), data, 0o600)
}

// loadCachedSession returns the cached session for (appId, wallet) if one
// exists and hasn't expired. Ok=false signals "go do the full login dance."
func loadCachedSession(appID, wallet string) (*auth.Session, bool) {
	cache, err := loadSessionCache()
	if err != nil {
		return nil, false
	}
	entry, ok := cache[sessionCacheKey(appID, wallet)]
	if !ok {
		return nil, false
	}
	if entry.Wallet != wallet {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return &auth.Session{
		IDToken:      entry.IDToken,
		AccessToken:  entry.AccessToken,
		RefreshToken: entry.RefreshToken,
	}, true
}
