package tarobase

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestSubmitOffchain_SHA256SignsMessageAndSubmits verifies the full offchain
// pipeline: PUT /items to build, SHA-256 hash the tx.message JSON string,
// base64-wrap, submit to /app/<id>/rpc, return the txid.
func TestSubmitOffchain_SHA256SignsMessageAndSubmits(t *testing.T) {
	const innerMessage = `{"appId":"test-app","instructions":[{"path":"memories/x","data":{}}]}`
	expectedHash := sha256.Sum256([]byte(innerMessage))
	expectedSig := base64.StdEncoding.EncodeToString(expectedHash[:])

	var capturedRPCBody map[string]any
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/items": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"offchainTransaction":{"message":`+
				mustJSONString(innerMessage)+`,"id":"tx-1"}}`)
		},
		"/app/*/rpc": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedRPCBody)
			_, _ = io.WriteString(w, `{"result":"sim_abc123"}`)
		},
	})
	defer cleanup()

	result, err := client.SetManyAndSubmit(context.Background(), []Document{
		{Path: "memories/x", Document: map[string]any{"content": "hi"}},
	})
	if err != nil {
		t.Fatalf("SetManyAndSubmit: %v", err)
	}
	if result.TransactionID != "sim_abc123" {
		t.Errorf("txid mismatch: %q", result.TransactionID)
	}
	if result.Chain != ChainOffchain {
		t.Errorf("chain mismatch: %q", result.Chain)
	}

	// Decode the base64 params[0] and check signature == SHA-256 of inner message.
	params, _ := capturedRPCBody["params"].([]any)
	if len(params) != 1 {
		t.Fatalf("expected 1 RPC param, got %d", len(params))
	}
	encoded, _ := params[0].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode param: %v", err)
	}
	var signed signedOffchainTx
	if err := json.Unmarshal(decoded, &signed); err != nil {
		t.Fatalf("decode signed tx: %v", err)
	}
	if signed.Signature != expectedSig {
		t.Errorf("signature mismatch:\n got  %s\n want %s", signed.Signature, expectedSig)
	}
}

func TestSubmitOffchain_PropagatesServerErrorWithTrace(t *testing.T) {
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/items": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"User X does not have access to create item at path memories/Y","trace":[{"variable":"$userId","resolvedValue":"Y"}]}`)
		},
	})
	defer cleanup()
	_, err := client.SetManyAndSubmit(context.Background(),
		[]Document{{Path: "memories/y", Document: map[string]any{"content": "hi"}}})
	if err == nil {
		t.Fatal("expected error")
	}
	se, ok := err.(*ServerError)
	if !ok {
		t.Fatalf("expected *ServerError, got %T: %v", err, err)
	}
	if se.Status != http.StatusForbidden {
		t.Errorf("status: got %d", se.Status)
	}
	if !strings.Contains(se.Message, "does not have access") {
		t.Errorf("message missing rule-rejection phrasing: %q", se.Message)
	}
	if len(se.Trace) == 0 {
		t.Error("trace not captured")
	}
}

// submitMainnet is covered by mainnet_borsh_test.go (arg encoding) and by
// an integration run against live Helius — there's no value in a local stub
// now that the real path is wired.

func TestSubmitOffchain_EmptyMessageErrors(t *testing.T) {
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/items": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"offchainTransaction":{"message":""}}`)
		},
	})
	defer cleanup()
	_, err := client.SetManyAndSubmit(context.Background(),
		[]Document{{Path: "memories/x", Document: map[string]any{"x": 1}}})
	if err == nil || !strings.Contains(err.Error(), "tx.message is empty") {
		t.Errorf("expected empty-message error, got %v", err)
	}
}

// mustJSONString JSON-encodes a string (wraps in quotes, escapes inner).
// Used to embed a JSON-string literal into raw server response strings.
func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
