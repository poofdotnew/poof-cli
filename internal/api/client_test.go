package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock AuthProvider
// ---------------------------------------------------------------------------

// mockAuthProvider implements AuthProvider for testing.
type mockAuthProvider struct {
	token         string
	walletAddress string
	tokenErr      error

	// invalidateCalled tracks how many times InvalidateToken was called.
	invalidateCalled atomic.Int32
	// getTokenCalled tracks how many times GetToken was called.
	getTokenCalled atomic.Int32
	// tokenFunc, if set, overrides the default GetToken behavior.
	tokenFunc func() (string, error)
}

func (m *mockAuthProvider) GetToken() (string, error) {
	m.getTokenCalled.Add(1)
	if m.tokenFunc != nil {
		return m.tokenFunc()
	}
	return m.token, m.tokenErr
}

func (m *mockAuthProvider) InvalidateToken() {
	m.invalidateCalled.Add(1)
}

func (m *mockAuthProvider) WalletAddress() string {
	return m.walletAddress
}

// newTestClient creates a Client wired to the given test server and mock auth.
func newTestClient(serverURL string, auth *mockAuthProvider) *Client {
	return &Client{
		BaseURL:     serverURL,
		AuthManager: auth,
		HTTPClient:  &http.Client{},
	}
}

// ---------------------------------------------------------------------------
// Tests for Do()
// ---------------------------------------------------------------------------

func TestDo_SuccessfulRequest(t *testing.T) {
	expected := map[string]string{"status": "ok"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "test-token", walletAddress: "wallet123"}
	client := newTestClient(srv.URL, auth)

	body, err := client.Do(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("Do() returned unexpected error: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if got["status"] != "ok" {
		t.Errorf("got status=%q, want %q", got["status"], "ok")
	}
	if auth.invalidateCalled.Load() != 0 {
		t.Error("InvalidateToken should not have been called on success")
	}
}

func TestDo_401AutoRetry_ThenSuccess(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// First call: return 401
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"token expired"}`))
			return
		}
		// Second call: return 200
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"success"}`))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "fresh-token", walletAddress: "wallet123"}
	client := newTestClient(srv.URL, auth)

	body, err := client.Do(context.Background(), http.MethodGet, "/retry", nil)
	if err != nil {
		t.Fatalf("Do() returned unexpected error: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if got["result"] != "success" {
		t.Errorf("got result=%q, want %q", got["result"], "success")
	}
	if auth.invalidateCalled.Load() != 1 {
		t.Errorf("InvalidateToken should have been called once, got %d", auth.invalidateCalled.Load())
	}
	// GetToken called twice: once for the first request, once for the retry
	if auth.getTokenCalled.Load() != 2 {
		t.Errorf("GetToken should have been called twice, got %d", auth.getTokenCalled.Load())
	}
}

func TestDo_401AutoRetry_StillFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 401
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"permanently unauthorized"}`))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "bad-token", walletAddress: "wallet123"}
	client := newTestClient(srv.URL, auth)

	_, err := client.Do(context.Background(), http.MethodGet, "/always-401", nil)
	if err == nil {
		t.Fatal("Do() should have returned an error for persistent 401")
	}

	apiErr, ok := IsAPIError(err)
	if !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected status code 401, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "permanently unauthorized" {
		t.Errorf("expected message %q, got %q", "permanently unauthorized", apiErr.Message)
	}
	// InvalidateToken called once, then second 401 triggers parseAPIError
	if auth.invalidateCalled.Load() != 1 {
		t.Errorf("InvalidateToken should have been called once, got %d", auth.invalidateCalled.Load())
	}
}

func TestDo_4xxErrorParsing(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		respBody   string
		wantMsg    string
	}{
		{
			name:       "400 Bad Request with JSON error",
			statusCode: http.StatusBadRequest,
			respBody:   `{"error":"invalid input","code":"INVALID"}`,
			wantMsg:    "invalid input",
		},
		{
			name:       "403 Forbidden",
			statusCode: http.StatusForbidden,
			respBody:   `{"error":"access denied"}`,
			wantMsg:    "access denied",
		},
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			respBody:   `{"error":"resource not found"}`,
			wantMsg:    "resource not found",
		},
		{
			name:       "422 Unprocessable Entity",
			statusCode: 422,
			respBody:   `{"error":"validation failed"}`,
			wantMsg:    "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.respBody))
			}))
			defer srv.Close()

			auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
			client := newTestClient(srv.URL, auth)

			_, err := client.Do(context.Background(), http.MethodGet, "/error", nil)
			if err == nil {
				t.Fatal("Do() should have returned an error")
			}

			apiErr, ok := IsAPIError(err)
			if !ok {
				t.Fatalf("expected APIError, got %T: %v", err, err)
			}
			if apiErr.StatusCode != tt.statusCode {
				t.Errorf("got status %d, want %d", apiErr.StatusCode, tt.statusCode)
			}
			if apiErr.Message != tt.wantMsg {
				t.Errorf("got message %q, want %q", apiErr.Message, tt.wantMsg)
			}
		})
	}
}

func TestDo_5xxErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"something went wrong","code":"INTERNAL"}`))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)

	_, err := client.Do(context.Background(), http.MethodGet, "/crash", nil)
	if err == nil {
		t.Fatal("Do() should have returned an error for 500")
	}

	apiErr, ok := IsAPIError(err)
	if !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("got status %d, want 500", apiErr.StatusCode)
	}
	if apiErr.Message != "something went wrong" {
		t.Errorf("got message %q, want %q", apiErr.Message, "something went wrong")
	}
	if apiErr.Code != "INTERNAL" {
		t.Errorf("got code %q, want %q", apiErr.Code, "INTERNAL")
	}
}

func TestDo_AuthErrorPropagation(t *testing.T) {
	auth := &mockAuthProvider{
		tokenErr:      &APIError{StatusCode: 401, Message: "auth service down"},
		walletAddress: "wallet",
	}
	// BaseURL doesn't matter since GetToken fails before any HTTP call.
	client := &Client{
		BaseURL:     "http://localhost:0",
		AuthManager: auth,
		HTTPClient:  &http.Client{},
	}

	_, err := client.Do(context.Background(), http.MethodGet, "/anything", nil)
	if err == nil {
		t.Fatal("Do() should have returned an error when GetToken fails")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("expected error to contain 'auth failed', got: %v", err)
	}
}

func TestDo_PostWithBody(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	var received payload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)

	sent := payload{Name: "alice", Age: 30}
	_, err := client.Do(context.Background(), http.MethodPost, "/create", sent)
	if err != nil {
		t.Fatalf("Do() returned unexpected error: %v", err)
	}

	if received.Name != "alice" || received.Age != 30 {
		t.Errorf("server received %+v, want %+v", received, sent)
	}
}

// ---------------------------------------------------------------------------
// Tests for parseAPIError()
// ---------------------------------------------------------------------------

func TestParseAPIError_ValidJSON(t *testing.T) {
	body := []byte(`{"error":"not found","code":"NOT_FOUND","membershipRequired":true}`)

	err := parseAPIError(body, 404)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("got StatusCode=%d, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "not found" {
		t.Errorf("got Message=%q, want %q", apiErr.Message, "not found")
	}
	if apiErr.Code != "NOT_FOUND" {
		t.Errorf("got Code=%q, want %q", apiErr.Code, "NOT_FOUND")
	}
	if !apiErr.MembershipRequired {
		t.Error("expected MembershipRequired=true")
	}
}

func TestParseAPIError_InvalidJSON(t *testing.T) {
	body := []byte(`This is not JSON at all`)

	err := parseAPIError(body, 502)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 502 {
		t.Errorf("got StatusCode=%d, want 502", apiErr.StatusCode)
	}
	if apiErr.Message != "This is not JSON at all" {
		t.Errorf("got Message=%q, want %q", apiErr.Message, "This is not JSON at all")
	}
}

func TestParseAPIError_EmptyBody(t *testing.T) {
	err := parseAPIError([]byte{}, 500)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("got StatusCode=%d, want 500", apiErr.StatusCode)
	}
	// Empty body is invalid JSON, so Message should be the empty string.
	if apiErr.Message != "" {
		t.Errorf("got Message=%q, want empty string", apiErr.Message)
	}
}

func TestParseAPIError_StatusCodeOverride(t *testing.T) {
	// Even if the JSON body somehow contains no StatusCode (it's json:"-"),
	// the passed-in status code should always be used.
	body := []byte(`{"error":"rate limited"}`)

	err := parseAPIError(body, 429)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("got StatusCode=%d, want 429", apiErr.StatusCode)
	}
}

func TestParseAPIError_HTMLBody(t *testing.T) {
	// Simulate a proxy/load-balancer returning HTML instead of JSON.
	body := []byte(`<html><body>Bad Gateway</body></html>`)

	err := parseAPIError(body, 502)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 502 {
		t.Errorf("got StatusCode=%d, want 502", apiErr.StatusCode)
	}
	if apiErr.Message != string(body) {
		t.Errorf("got Message=%q, want %q", apiErr.Message, string(body))
	}
}

func TestParseAPIError_ErrorString(t *testing.T) {
	body := []byte(`{"error":"oops"}`)
	err := parseAPIError(body, 500)
	expected := "poof API error (500): oops"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

// ---------------------------------------------------------------------------
// Tests for doRequest()
// ---------------------------------------------------------------------------

func TestDoRequest_BodyMarshalingAndHeaders(t *testing.T) {
	type reqPayload struct {
		Key   string `json:"key"`
		Value int    `json:"value"`
	}

	var (
		receivedAuth   string
		receivedWallet string
		receivedCT     string
		receivedBody   []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedWallet = r.Header.Get("X-Wallet-Address")
		receivedCT = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "my-secret-token", walletAddress: "0xABCDEF"}
	client := newTestClient(srv.URL, auth)

	payload := reqPayload{Key: "foo", Value: 42}
	respBody, statusCode, err := client.doRequest(context.Background(), http.MethodPost, "/data", payload)
	if err != nil {
		t.Fatalf("doRequest() returned unexpected error: %v", err)
	}
	if statusCode != 200 {
		t.Errorf("got status %d, want 200", statusCode)
	}

	// Check Authorization header
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("got Authorization=%q, want %q", receivedAuth, "Bearer my-secret-token")
	}
	// Check wallet header
	if receivedWallet != "0xABCDEF" {
		t.Errorf("got X-Wallet-Address=%q, want %q", receivedWallet, "0xABCDEF")
	}
	// Check Content-Type header
	if receivedCT != "application/json" {
		t.Errorf("got Content-Type=%q, want %q", receivedCT, "application/json")
	}
	// Check body was marshaled correctly
	var got reqPayload
	if err := json.Unmarshal(receivedBody, &got); err != nil {
		t.Fatalf("failed to unmarshal received body: %v", err)
	}
	if got.Key != "foo" || got.Value != 42 {
		t.Errorf("got body %+v, want {Key:foo Value:42}", got)
	}
	// Check we got a response body
	if string(respBody) != `{"ok":true}` {
		t.Errorf("got response body %q, want %q", string(respBody), `{"ok":true}`)
	}
}

func TestDoRequest_NilBody(t *testing.T) {
	var receivedContentLength int64
	var receivedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentLength = r.ContentLength
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)

	_, statusCode, err := client.doRequest(context.Background(), http.MethodGet, "/nodata", nil)
	if err != nil {
		t.Fatalf("doRequest() returned unexpected error: %v", err)
	}
	if statusCode != 200 {
		t.Errorf("got status %d, want 200", statusCode)
	}
	// With nil body, no content should be sent.
	if len(receivedBody) != 0 {
		t.Errorf("expected empty body, got %q", string(receivedBody))
	}
	// ContentLength should be 0 or -1 (no body).
	if receivedContentLength > 0 {
		t.Errorf("expected ContentLength <= 0, got %d", receivedContentLength)
	}
}

func TestDoRequest_BypassToken(t *testing.T) {
	var receivedBypass string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBypass = r.Header.Get("x-vercel-protection-bypass")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)
	client.BypassToken = "my-bypass-token"

	_, _, err := client.doRequest(context.Background(), http.MethodGet, "/protected", nil)
	if err != nil {
		t.Fatalf("doRequest() returned unexpected error: %v", err)
	}
	if receivedBypass != "my-bypass-token" {
		t.Errorf("got bypass header %q, want %q", receivedBypass, "my-bypass-token")
	}
}

func TestDoRequest_NoBypassTokenWhenEmpty(t *testing.T) {
	var receivedBypass string
	var bypassHeaderPresent bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBypass = r.Header.Get("X-Vercel-Protection-Bypass")
		_, bypassHeaderPresent = r.Header["X-Vercel-Protection-Bypass"]
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)
	// BypassToken is empty (zero value)

	_, _, err := client.doRequest(context.Background(), http.MethodGet, "/open", nil)
	if err != nil {
		t.Fatalf("doRequest() returned unexpected error: %v", err)
	}
	if receivedBypass != "" {
		t.Errorf("got bypass header %q, want empty", receivedBypass)
	}
	if bypassHeaderPresent {
		t.Error("x-vercel-protection-bypass header should not be present when BypassToken is empty")
	}
}

func TestDoRequest_AuthFailure(t *testing.T) {
	auth := &mockAuthProvider{
		tokenErr:      &APIError{StatusCode: 500, Message: "auth exploded"},
		walletAddress: "wallet",
	}
	client := &Client{
		BaseURL:     "http://localhost:0",
		AuthManager: auth,
		HTTPClient:  &http.Client{},
	}

	_, _, err := client.doRequest(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("doRequest() should fail when GetToken returns an error")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("expected 'auth failed' in error, got: %v", err)
	}
}

func TestDoRequest_UnmarshalableBody(t *testing.T) {
	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := &Client{
		BaseURL:     "http://localhost:0",
		AuthManager: auth,
		HTTPClient:  &http.Client{},
	}

	// Channels cannot be marshaled to JSON.
	unmarshalable := make(chan int)
	_, _, err := client.doRequest(context.Background(), http.MethodPost, "/test", unmarshalable)
	if err == nil {
		t.Fatal("doRequest() should fail when body cannot be marshaled")
	}
	if !strings.Contains(err.Error(), "failed to marshal request body") {
		t.Errorf("expected 'failed to marshal request body' in error, got: %v", err)
	}
}

func TestDoRequest_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the request

	_, _, err := client.doRequest(ctx, http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("doRequest() should fail with canceled context")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("expected 'request failed' in error, got: %v", err)
	}
}

func TestDoRequest_MethodAndPath(t *testing.T) {
	var receivedMethod, receivedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)

	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		_, _, err := client.doRequest(context.Background(), method, "/api/test", nil)
		if err != nil {
			t.Fatalf("doRequest(%s) returned unexpected error: %v", method, err)
		}
		if receivedMethod != method {
			t.Errorf("got method %q, want %q", receivedMethod, method)
		}
		if receivedPath != "/api/test" {
			t.Errorf("got path %q, want %q", receivedPath, "/api/test")
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for Do() edge cases
// ---------------------------------------------------------------------------

func TestDo_401RetryWithNewToken(t *testing.T) {
	// Verify that after InvalidateToken, the retry uses a fresh token.
	var callCount atomic.Int32
	var receivedTokens []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTokens = append(receivedTokens, r.Header.Get("Authorization"))
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"expired"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tokenCall := atomic.Int32{}
	auth := &mockAuthProvider{
		walletAddress: "wallet",
		tokenFunc: func() (string, error) {
			n := tokenCall.Add(1)
			if n == 1 {
				return "old-token", nil
			}
			return "new-token", nil
		},
	}
	client := newTestClient(srv.URL, auth)

	_, err := client.Do(context.Background(), http.MethodGet, "/refresh-test", nil)
	if err != nil {
		t.Fatalf("Do() returned unexpected error: %v", err)
	}

	if len(receivedTokens) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(receivedTokens))
	}
	if receivedTokens[0] != "Bearer old-token" {
		t.Errorf("first request token: got %q, want %q", receivedTokens[0], "Bearer old-token")
	}
	if receivedTokens[1] != "Bearer new-token" {
		t.Errorf("second request token: got %q, want %q", receivedTokens[1], "Bearer new-token")
	}
}

func TestDo_NoRetryOnNon401Errors(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)

	_, err := client.Do(context.Background(), http.MethodGet, "/forbidden", nil)
	if err == nil {
		t.Fatal("Do() should have returned an error for 403")
	}
	// Should NOT retry -- only one request.
	if callCount.Load() != 1 {
		t.Errorf("expected 1 request (no retry), got %d", callCount.Load())
	}
	if auth.invalidateCalled.Load() != 0 {
		t.Error("InvalidateToken should not be called for non-401 errors")
	}
}

func TestDo_2xxNoError(t *testing.T) {
	statusCodes := []int{200, 201, 204, 299}
	for _, code := range statusCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
			client := newTestClient(srv.URL, auth)

			_, err := client.Do(context.Background(), http.MethodGet, "/ok", nil)
			if err != nil {
				t.Errorf("Do() returned error for status %d: %v", code, err)
			}
		})
	}
}

func TestDo_3xxNoError(t *testing.T) {
	// 3xx codes are < 400, so they should not be treated as errors by Do().
	// (The HTTP client won't follow redirects for non-GET by default, but
	// the status code itself should not trigger an error in Do.)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified) // 304
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	auth := &mockAuthProvider{token: "token", walletAddress: "wallet"}
	client := newTestClient(srv.URL, auth)

	_, err := client.Do(context.Background(), http.MethodGet, "/cached", nil)
	if err != nil {
		t.Errorf("Do() returned error for 304: %v", err)
	}
}
