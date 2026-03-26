package auth

import (
	"fmt"
	"net/http"
	"os"
	"sync"
)

// Manager orchestrates authentication: login, token caching, and refresh.
type Manager struct {
	keypair       *Keypair
	sessionClient *SessionClient
	wallet        string
	env           string

	mu      sync.Mutex
	session *Session
}

// NewManager creates a new auth manager.
func NewManager(privateKey, authURL, appID, env string) (*Manager, error) {
	kp, err := LoadKeypair(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load keypair: %w", err)
	}

	sc := &SessionClient{
		AuthURL:    authURL,
		AppID:      appID,
		HTTPClient: &http.Client{},
	}

	return &Manager{
		keypair:       kp,
		sessionClient: sc,
		wallet:        kp.Address,
		env:           env,
	}, nil
}

// GetToken returns a valid ID token, refreshing or re-authenticating as needed.
func (m *Manager) GetToken() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Check in-memory session
	if m.session != nil {
		return m.session.IDToken, nil
	}

	// 2. Check on-disk cache
	cached, err := LoadCachedTokens()
	if err == nil && cached.IsValid(m.wallet, m.env) {
		m.session = &Session{
			IDToken:      cached.IDToken,
			AccessToken:  cached.AccessToken,
			RefreshToken: cached.RefreshToken,
		}
		return m.session.IDToken, nil
	}

	// 3. Try refresh if we have a refresh token
	if cached != nil && cached.HasRefreshToken() && cached.Wallet == m.wallet && cached.Environment == m.env {
		session, err := m.sessionClient.RefreshSession(cached.RefreshToken)
		if err == nil {
			m.session = session
			if cacheErr := SaveCachedTokens(session, m.wallet, m.env); cacheErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to cache tokens: %v\n", cacheErr)
			}
			return session.IDToken, nil
		}
		// Refresh failed, fall through to full login
	}

	// 4. Full login
	return m.login()
}

// Login performs a full authentication flow.
func (m *Manager) Login() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.login()
}

func (m *Manager) login() (string, error) {
	nonce, err := m.sessionClient.FetchNonce()
	if err != nil {
		return "", err
	}

	session, err := m.sessionClient.CreateSession(m.keypair, nonce)
	if err != nil {
		return "", err
	}

	m.session = session
	if cacheErr := SaveCachedTokens(session, m.wallet, m.env); cacheErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to cache tokens: %v\n", cacheErr)
	}
	return session.IDToken, nil
}

// InvalidateToken clears the cached token so the next call re-authenticates.
func (m *Manager) InvalidateToken() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = nil
}

// WalletAddress returns the wallet address derived from the keypair.
func (m *Manager) WalletAddress() string {
	return m.wallet
}

// Logout clears all cached tokens.
func (m *Manager) Logout() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = nil
	return ClearCachedTokens()
}
