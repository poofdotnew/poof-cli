package tarobase

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// SetManyResult is what Submit returns — always a tx signature (or
// simulated tx id for offchain) plus whether submission actually happened.
// Callers who want the raw build-transaction response should use SetMany
// directly instead of SetManyAndSubmit.
type SetManyResult struct {
	// TransactionID is the signature (mainnet) or simulated id (poofnet).
	TransactionID string `json:"transactionId"`
	// Chain reports which path ran — "offchain" or "solana_mainnet".
	Chain Chain `json:"chain"`
	// Raw is the full server response from the build-transaction step,
	// preserved so callers can see the tx payload that was signed.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// SubmitOptions tunes SetManyAndSubmit. Zero-value is the safe default:
// preflight simulation runs on mainnet submits, so a tx whose rule would
// reject gets caught client-side and never lands on-chain (no fee).
type SubmitOptions struct {
	// SkipPreflight disables the RPC's preflight simulation for mainnet
	// submits. Set it when you *want* a failing tx to actually land on-chain
	// as a failed tx — e.g. auditing every guard rejection via Solscan. Costs
	// one base fee per tx whether it succeeds or fails. Ignored on ChainOffchain.
	SkipPreflight bool
}

// SetManyAndSubmit is the full one-shot write: build the tx, sign it, submit
// it, return the signature. The path it takes under the hood depends on
// Client.Chain — for offchain apps (poofnet / draft) it's SHA-256 mock
// signing + Tarobase-RPC submit; for mainnet apps it's real Solana signing +
// Solana RPC submit.
//
// Variadic opts lets callers pass a SubmitOptions without a breaking change
// to the common zero-arg case. Only the first opts entry is consulted.
func (c *Client) SetManyAndSubmit(ctx context.Context, docs []Document, opts ...SubmitOptions) (*SetManyResult, error) {
	var opt SubmitOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	raw, err := c.SetMany(ctx, docs)
	if err != nil {
		return nil, err
	}

	switch c.Chain {
	case ChainOffchain:
		txid, err := c.submitOffchain(ctx, raw)
		if err != nil {
			return nil, err
		}
		return &SetManyResult{TransactionID: txid, Chain: ChainOffchain, Raw: raw}, nil

	case ChainMainnet:
		txid, err := c.submitMainnet(ctx, raw, opt)
		if err != nil {
			return nil, err
		}
		return &SetManyResult{TransactionID: txid, Chain: ChainMainnet, Raw: raw}, nil

	default:
		return nil, fmt.Errorf("unsupported chain %q", c.Chain)
	}
}

// ---------------------------------------------------------------------------
// Offchain (poofnet / draft) — SHA-256 mock signing + /app/{appId}/rpc submit
// ---------------------------------------------------------------------------

// offchainBuildResponse is the `PUT /items` shape on draft/poofnet. The
// `message` field is a stringified JSON blob that we sign (mock) with
// SHA-256; the whole `offchainTransaction` object is then wrapped and
// submitted to the Tarobase RPC endpoint.
//
// Non-passthrough off-chain collections (e.g. `memories/$userId`) short-
// circuit this dance — the server applies the write at REST and returns a
// `success` payload with no `offchainTransaction`. We detect that and skip
// the submit step entirely, returning the server's response as the result.
type offchainBuildResponse struct {
	OffchainTransaction json.RawMessage `json:"offchainTransaction,omitempty"`
	Success             *bool           `json:"success,omitempty"`
	Result              json.RawMessage `json:"result,omitempty"`
}

type signedOffchainTx struct {
	Transaction json.RawMessage `json:"transaction"`
	Signature   string          `json:"signature"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) submitOffchain(ctx context.Context, buildRaw json.RawMessage) (string, error) {
	// Non-passthrough writes (e.g. memories/$userId) apply at REST — the
	// server returns either `{success: true}` or a bare `true`. Either way
	// there's no tx to sign; short-circuit with an empty txid.
	if isDirectApplied(buildRaw) {
		return "", nil
	}

	var build offchainBuildResponse
	if err := json.Unmarshal(buildRaw, &build); err != nil {
		return "", fmt.Errorf("parse offchain build response: %w", err)
	}
	if len(build.OffchainTransaction) == 0 {
		return "", errors.New("offchain submit: build response has neither offchainTransaction nor success:true")
	}

	// Extract tx.message (a stringified JSON inside the tx object). That's
	// what SHA-256-mock signs — matches the SDK's OffchainAuthProvider behavior.
	var tx struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(build.OffchainTransaction, &tx); err != nil {
		return "", fmt.Errorf("extract tx.message: %w", err)
	}
	if tx.Message == "" {
		return "", errors.New("offchain submit: tx.message is empty")
	}

	hash := sha256.Sum256([]byte(tx.Message))
	signature := base64.StdEncoding.EncodeToString(hash[:])

	signed := signedOffchainTx{
		Transaction: build.OffchainTransaction,
		Signature:   signature,
	}
	signedJSON, err := json.Marshal(signed)
	if err != nil {
		return "", fmt.Errorf("marshal signed tx: %w", err)
	}
	param := base64.StdEncoding.EncodeToString(signedJSON)

	rpcPath := "/app/" + c.AppID + "/rpc"
	body := rpcRequest{JSONRPC: "2.0", ID: 1, Method: "sendTransaction", Params: []any{param}}
	rawResp, err := c.doExpect(ctx, "POST", rpcPath, body)
	if err != nil {
		return "", err
	}
	var resp rpcResponse
	if err := json.Unmarshal(rawResp, &resp); err != nil {
		return "", fmt.Errorf("parse rpc response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	// Result may be a bare string or {"result": "..."} — unwrap either.
	var asStr string
	if err := json.Unmarshal(resp.Result, &asStr); err == nil {
		return asStr, nil
	}
	return string(resp.Result), nil
}
