package tarobase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go/rpc"
)

// isDirectApplied returns true when the server's build-tx response signals
// "already applied, no tx to sign" — either as a bare `true` value or as an
// object with `success: true`. Both forms show up in practice (bare `true`
// for mainnet offchain-collection writes, the object form for some draft
// paths) so we accept either.
func isDirectApplied(raw json.RawMessage) bool {
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var obj offchainBuildResponse
	if err := json.Unmarshal(raw, &obj); err == nil {
		return len(obj.OffchainTransaction) == 0 && obj.Success != nil && *obj.Success
	}
	return false
}

// Default Solana RPC endpoint for mainnet. Matches the SDK's default
// (celestia-cegncv-fast-mainnet.helius-rpc.com). Overridable via
// POOF_SOLANA_MAINNET_RPC for ops who want to route through their own
// Helius / Triton / whatever.
const defaultMainnetRPC = "https://celestia-cegncv-fast-mainnet.helius-rpc.com"

// submitMainnet takes the server's build-transaction response, decodes it
// into an Anchor set_documents call, fetches LUT contents + blockhash from
// the Solana RPC, signs with the client keypair, and submits.
//
// When opt.SkipPreflight is true, the RPC's local simulation is bypassed —
// the tx is forwarded to the validator even if it'll fail, so it lands
// on-chain as a failed tx (visible on Solscan). Default is false: preflight
// catches policy rejections client-side and the tx never lands (no fee).
//
// Returns the Solana transaction signature on success. Errors bubble up
// with enough context to see which phase failed (decode / LUT fetch /
// blockhash / sign / submit).
func (c *Client) submitMainnet(ctx context.Context, buildRaw json.RawMessage, opt SubmitOptions) (string, error) {
	// Offchain collection writes (e.g. memories/$userId, AllowlistOffchain/...)
	// are applied server-side at REST — no Solana tx to sign. The server
	// signals this in one of two equivalent shapes that we have to accept both of:
	//   - a bare `true` JSON value (matches the SDK's `setResponse.data === true`
	//     branch)
	//   - an object `{"success": true, "result": ..., "trace": [...]}` similar
	//     to the offchain draft path
	// If either matches, short-circuit with an empty txid.
	if isDirectApplied(buildRaw) {
		return "", nil
	}

	var build MainnetBuildResponse
	if err := json.Unmarshal(buildRaw, &build); err != nil {
		return "", fmt.Errorf("parse mainnet build response: %w", err)
	}
	if len(build.Transactions) == 0 {
		return "", fmt.Errorf("mainnet build response has neither transactions nor a direct-applied success: %s", string(buildRaw))
	}

	rpcClient := rpc.New(c.mainnetRPCURL())
	txBuilt, err := c.buildAndSignMainnetTx(ctx, rpcClient, &build)
	if err != nil {
		return "", err
	}

	sig, err := rpcClient.SendTransactionWithOpts(ctx, txBuilt, rpc.TransactionOpts{
		SkipPreflight:       opt.SkipPreflight,
		PreflightCommitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return "", fmt.Errorf("submit tx: %w", err)
	}
	return sig.String(), nil
}

// mainnetRPCURL picks the Helius default or an override; kept as a method
// so test setups can stub by spinning up a mock rpc server and pointing
// POOF_SOLANA_MAINNET_RPC at it.
func (c *Client) mainnetRPCURL() string {
	if v := strings.TrimSpace(envOrDefault("POOF_SOLANA_MAINNET_RPC", "")); v != "" {
		return v
	}
	return defaultMainnetRPC
}
