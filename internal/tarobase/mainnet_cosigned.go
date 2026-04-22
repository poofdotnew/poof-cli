package tarobase

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// signServerCosignedTx handles the server-cosigned path: the server has
// already assembled a VersionedTransaction with all CPI metadata and its own
// signature slots populated, and we just need to refresh the blockhash and
// add the caller's signature. Used by actions whose hooks CPI into other
// programs (Phoenix perps, Meteora pool creation), where the Poof service
// pre-attests to the CPI arg bytes.
//
// Mirrors `solana-keypair-provider.ts`:runTransactionInner — the branch that
// triggers when `sol.signedTransaction` is set.
func (c *Client) signServerCosignedTx(
	ctx context.Context,
	rpcClient *rpc.Client,
	signedTxB64 string,
) (*solana.Transaction, error) {
	raw, err := base64.StdEncoding.DecodeString(signedTxB64)
	if err != nil {
		return nil, fmt.Errorf("decode server-cosigned tx base64: %w", err)
	}

	tx, err := solana.TransactionFromBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("deserialize server-cosigned tx: %w", err)
	}

	// Refresh the blockhash on the tx. The server's signed tx has a
	// stale-ish blockhash; we swap in a fresh one before countersigning so
	// the tx has the full ~150-slot window to land.
	bh, err := rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("get blockhash: %w", err)
	}
	if err := setVersionedBlockhash(tx, bh.Value.Blockhash); err != nil {
		return nil, fmt.Errorf("set blockhash on cosigned tx: %w", err)
	}

	// Countersign as the wallet. We use PartialSign (not Sign) because the
	// server has already signed some of the required slots (its own authority
	// key for CPI attestations, e.g. Meteora config) and Sign pre-checks that
	// every required signer's getter returns non-nil — erroring as "signer
	// key ... not found in vault" when it's really just a signer whose slot
	// is already filled. PartialSign matches the TS SDK's `tx.sign([kp])`
	// behavior: fill only the slot matching our keypair, leave everything
	// else intact.
	signer := solana.PrivateKey(c.Keypair.PrivateKey)
	payer := solana.PublicKeyFromBytes(c.Keypair.PublicKey)
	if _, err := tx.PartialSign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(payer) {
			return &signer
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("countersign: %w", err)
	}
	return tx, nil
}

// setVersionedBlockhash rewrites the recentBlockhash on an already-
// deserialized solana-go Transaction. For a v0 message the field lives on
// Message.RecentBlockhash directly; for a legacy transaction it's the same.
// Both are plain Hash fields we can overwrite.
func setVersionedBlockhash(tx *solana.Transaction, blockhash solana.Hash) error {
	tx.Message.RecentBlockhash = blockhash
	return nil
}
