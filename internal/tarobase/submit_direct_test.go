package tarobase

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// Non-passthrough off-chain writes like memories/$userId apply at REST with
// a 200 and a `{"success":true,...}` body. No RPC hop. Make sure SubmitMany-
// AndSubmit in that case returns cleanly with an empty txid rather than
// failing to find an offchainTransaction.
func TestSubmitOffchain_DirectAppliedWriteReturnsEmptyTxID(t *testing.T) {
	client, cleanup := mockServer(t, map[string]http.HandlerFunc{
		"/items": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"success":true,"result":true,"trace":[]}`)
		},
	})
	defer cleanup()
	result, err := client.SetManyAndSubmit(context.Background(), []Document{
		{Path: "memories/x", Document: map[string]any{"content": "hi"}},
	})
	if err != nil {
		t.Fatalf("SetManyAndSubmit (direct-applied): %v", err)
	}
	if result.TransactionID != "" {
		t.Errorf("expected empty txid for direct-applied write, got %q", result.TransactionID)
	}
	if result.Chain != ChainOffchain {
		t.Errorf("chain mismatch: %q", result.Chain)
	}
}
