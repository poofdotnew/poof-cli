package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Session holds the tokens returned from authentication.
type Session struct {
	AccessToken  string `json:"accessToken"`
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
}

// SessionClient handles nonce fetching and session creation.
type SessionClient struct {
	AuthURL    string
	AppID      string
	HTTPClient *http.Client
}

// nonceResponse is the response from the /auth/nonce endpoint.
type nonceResponse struct {
	Nonce string `json:"nonce"`
}

// FetchNonce gets a nonce from the auth server.
func (sc *SessionClient) FetchNonce() (string, error) {
	body, _ := json.Marshal(map[string]string{"appId": sc.AppID})

	resp, err := sc.HTTPClient.Post(sc.AuthURL+"/auth/nonce", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("nonce request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("nonce request returned %d: %s", resp.StatusCode, string(respBody))
	}

	var nr nonceResponse
	if err := json.NewDecoder(resp.Body).Decode(&nr); err != nil {
		return "", fmt.Errorf("failed to parse nonce response: %w", err)
	}

	return nr.Nonce, nil
}

// CreateSession authenticates with a signed message and returns tokens.
func (sc *SessionClient) CreateSession(kp *Keypair, nonce string) (*Session, error) {
	message := GenSolanaMessage(kp.Address, nonce)
	signature := kp.Sign([]byte(message))
	sig64 := base64.StdEncoding.EncodeToString(signature)

	reqBody, _ := json.Marshal(map[string]string{
		"appId":      sc.AppID,
		"address":    kp.Address,
		"message":    message,
		"signature":  sig64,
		"authMethod": "phantom",
	})

	resp, err := sc.HTTPClient.Post(sc.AuthURL+"/session", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("session request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("session request returned %d: %s", resp.StatusCode, string(respBody))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to parse session response: %w", err)
	}

	return &session, nil
}

// RefreshSession refreshes tokens using a refresh token.
func (sc *SessionClient) RefreshSession(refreshToken string) (*Session, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"refreshToken": refreshToken,
		"appId":        sc.AppID,
	})

	resp, err := sc.HTTPClient.Post(sc.AuthURL+"/session/refresh", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh request returned %d: %s", resp.StatusCode, string(respBody))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &session, nil
}
