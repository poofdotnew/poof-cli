package tarobase

import (
	"context"
	"fmt"
	"net/http"

	"github.com/poofdotnew/poof-cli/internal/auth"
)

// login runs the Tarobase auth dance: fetch a nonce for our appId, sign the
// canonical message, exchange for a session. The resulting Session is stashed
// on the Client so Do() can attach `Authorization: Bearer <idToken>`.
//
// `idToken` (not accessToken) is what the server verifies against. See
// internal/auth/auth.go:GetToken — it returns IDToken for this reason.
//
// Sessions are cached on-disk per (appId, wallet) in ~/.poof/tarobase-sessions.json
// (see session_cache.go). A chain of `poof data` calls against the same app
// reuses the cached session instead of redoing nonce+sign on every invocation,
// which otherwise hammers auth.tarobase.com into a 429. On a 401 from the
// data-plane API the Client calls refreshSession() to invalidate the bad
// cache entry and re-issue.
func (c *Client) login(_ context.Context) error {
	wallet := c.Keypair.Address
	if cached, ok := loadCachedSession(c.AppID, wallet); ok {
		c.session = cached
		return nil
	}
	return c.issueSession()
}

// issueSession always does the full nonce+sign dance and persists the result.
// Used for the initial login and after a cache invalidation.
func (c *Client) issueSession() error {
	sc := &auth.SessionClient{
		AuthURL:    c.AuthURL,
		AppID:      c.AppID,
		HTTPClient: &http.Client{Timeout: c.httpClient.Timeout},
	}
	nonce, err := sc.FetchNonce()
	if err != nil {
		return fmt.Errorf("fetch nonce: %w", err)
	}
	session, err := sc.CreateSession(c.Keypair, nonce)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	c.session = session
	// Best-effort persist — a failed write doesn't fail the login, we just
	// eat the extra nonce+sign next time.
	_ = saveSessionCacheEntry(c.AppID, c.Keypair.Address, session)
	return nil
}

// refreshSession drops the cached session for this (appId, wallet) and
// re-issues. Called by the HTTP layer on a 401 from the data plane so stale
// cached tokens don't lock out a still-valid keypair.
func (c *Client) refreshSession() error {
	invalidateSessionCacheEntry(c.AppID, c.Keypair.Address)
	c.session = nil
	return c.issueSession()
}
