package tarobase

import (
	"context"
	"encoding/json"
	"testing"
)

// Mainnet writes that target offchain collections (memories/$userId, any
// AllowlistOffchain member, etc.) don't get a `transactions` payload back —
// the server applied them at REST. submitMainnet needs to detect the same
// `{success: true}` shape submitOffchain does and short-circuit rather than
// try to decode a non-existent mainnet Anchor tx.
func TestSubmitMainnet_DirectAppliedShortCircuits(t *testing.T) {
	c := &Client{Chain: ChainMainnet}
	txid, err := c.submitMainnet(
		context.Background(),
		json.RawMessage(`{"success":true,"result":true,"trace":[]}`),
		SubmitOptions{},
	)
	if err != nil {
		t.Fatalf("direct-applied submit errored: %v", err)
	}
	if txid != "" {
		t.Errorf("expected empty txid for direct-applied mainnet write, got %q", txid)
	}
}

// A mainnet response without either `transactions` or `success:true` is
// malformed and we should say so rather than silently treating it as done.
func TestSubmitMainnet_EmptyResponseErrors(t *testing.T) {
	c := &Client{Chain: ChainMainnet}
	_, err := c.submitMainnet(
		context.Background(),
		json.RawMessage(`{"transactions":[]}`),
		SubmitOptions{},
	)
	if err == nil {
		t.Fatal("expected error on empty transactions + no success")
	}
}
