package tarobase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gagliardetto/solana-go"
	lookup "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"
)

// buildAndSignMainnetTx takes the server's mainnet build response and produces
// a fully-signed VersionedTransaction ready for submission. Two cases:
//
//   - Server-cosigned (CPI) path: when `signedTransaction` is set, deserialize
//     that pre-built VersionedTransaction, refresh the blockhash, and add the
//     wallet's signature. Used for actions whose hooks CPI into other programs
//     (Phoenix perps, Meteora pool creation).
//   - Client-built path: otherwise, borsh-encode the Anchor `set_documents`
//     args (including `tx_data` if the server provided any for non-CPI cases),
//     prepend `preInstructions`, assemble a LUT-aware MessageV0, sign.
//
// Returns the signed *solana.Transaction; the caller submits it via RPC.
func (c *Client) buildAndSignMainnetTx(
	ctx context.Context,
	rpcClient *rpc.Client,
	build *MainnetBuildResponse,
) (*solana.Transaction, error) {
	if len(build.Transactions) == 0 {
		return nil, fmt.Errorf("mainnet build response has no transactions")
	}
	// Multi-tx setMany isn't supported yet — the server only splits across
	// transactions when a single tx would exceed size limits. Flag it so we
	// don't silently sign only the first.
	if len(build.Transactions) > 1 {
		return nil, fmt.Errorf("server split the setMany into %d transactions; multi-tx signing not yet supported in the CLI — use the SDK", len(build.Transactions))
	}

	tx := build.Transactions[0]

	// Server-cosigned path: the server has already built and signed the tx
	// (CPI attestation). We deserialize, refresh blockhash, and countersign.
	if tx.SignedTransaction != "" {
		return c.signServerCosignedTx(ctx, rpcClient, tx.SignedTransaction)
	}

	payer := solana.PublicKeyFromBytes(c.Keypair.PublicKey)

	// 1. Convert server's transactionArgs into the borsh-encodable struct.
	anchorArgs, err := convertTransactionArgs(c.AppID, tx.TransactionArgs, tx.TxData)
	if err != nil {
		return nil, fmt.Errorf("convert args: %w", err)
	}

	// 2. Borsh-encode the instruction data.
	insData, err := EncodeSetDocumentsArgs(anchorArgs)
	if err != nil {
		return nil, fmt.Errorf("encode args: %w", err)
	}

	// 3. Build the set_documents instruction. Account order mirrors the IDL:
	//    payer (signer, writable) + system_program + remainingAccounts (in order).
	programID := solana.MustPublicKeyFromBase58(tarobaseProgramID)
	metas := solana.AccountMetaSlice{
		{PublicKey: payer, IsSigner: true, IsWritable: true},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
	}
	for i, ra := range tx.RemainingAccounts {
		pk, err := solana.PublicKeyFromBase58(ra.Pubkey)
		if err != nil {
			return nil, fmt.Errorf("remainingAccounts[%d] pubkey: %w", i, err)
		}
		metas = append(metas, &solana.AccountMeta{
			PublicKey:  pk,
			IsSigner:   ra.IsSigner,
			IsWritable: ra.IsWritable,
		})
	}
	setDocumentsIx := solana.NewInstruction(programID, metas, insData)

	// 4. Assemble pre-instructions (if any) + set_documents.
	instructions := []solana.Instruction{}
	for i, pi := range tx.PreInstructions {
		ix, err := convertPreInstruction(pi)
		if err != nil {
			return nil, fmt.Errorf("preInstructions[%d]: %w", i, err)
		}
		instructions = append(instructions, ix)
	}
	instructions = append(instructions, setDocumentsIx)

	// 5. Fetch latest blockhash + LUT contents in advance so we can pass
	//    them into NewTransaction, which does the v0-compile-with-LUT pass
	//    (skipping LUT-resolvable accounts from AccountKeys and emitting
	//    compressed AddressTableLookups). Calling SetAddressTables *after*
	//    building doesn't compress — it only populates the map used by
	//    ResolveLookups for decoding.
	bhResp, err := rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("get blockhash: %w", err)
	}

	lutKeys := make([]string, 0, 1+len(tx.AdditionalLutAddresses))
	if tx.LutAddress != "" {
		lutKeys = append(lutKeys, tx.LutAddress)
	}
	lutKeys = append(lutKeys, tx.AdditionalLutAddresses...)
	txOpts := []solana.TransactionOption{solana.TransactionPayer(payer)}
	if len(lutKeys) > 0 {
		tables, err := fetchLookupTables(ctx, rpcClient, lutKeys)
		if err != nil {
			return nil, fmt.Errorf("fetch LUTs: %w", err)
		}
		txOpts = append(txOpts, solana.TransactionAddressTables(tables))
	}
	txBuilt, err := solana.NewTransaction(instructions, bhResp.Value.Blockhash, txOpts...)
	if err != nil {
		return nil, fmt.Errorf("assemble tx: %w", err)
	}

	// 8. Sign with the wallet keypair.
	signer := solana.PrivateKey(c.Keypair.PrivateKey)
	if _, err := txBuilt.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(payer) {
			return &signer
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	return txBuilt, nil
}

// convertTransactionArgs turns the server's JSON tx args into the borsh-
// encodable Anchor form. This is where FieldValue union parsing happens, and
// where `txData` (CPI attestations for Phoenix/Meteora-pool/etc.) gets
// normalized into its borsh-encodable form.
func convertTransactionArgs(appID string, args TransactionArgs, serverTxData []TxDataEntry) (AnchorSetDocuments, error) {
	docs := make([]AnchorSetDocumentData, len(args.SetDocumentData))
	for i, sd := range args.SetDocumentData {
		ops := make([]AnchorFieldOperation, len(sd.Operations))
		for j, op := range sd.Operations {
			val, err := parseFieldValue(op.Value)
			if err != nil {
				return AnchorSetDocuments{}, fmt.Errorf("doc %d op %d (%s): %w", i, j, op.Key, err)
			}
			ops[j] = AnchorFieldOperation{Key: op.Key, Value: val, Operation: op.Operation}
		}
		docs[i] = AnchorSetDocumentData{Path: sd.Path, Operations: ops}
	}
	txData := make([]AnchorTxData, len(serverTxData))
	for i, td := range serverTxData {
		txData[i] = AnchorTxData{
			PluginFunctionKey: td.PluginFunctionKey,
			TxData:            []byte(td.TxData),
			RaIndices:         []uint64(td.RaIndices),
		}
	}
	return AnchorSetDocuments{
		AppID:       appID,
		Documents:   docs,
		DeletePaths: args.DeletePaths,
		TxData:      txData,
		Simulate:    false,
	}, nil
}

func convertPreInstruction(pi PreInstruction) (solana.Instruction, error) {
	programID, err := solana.PublicKeyFromBase58(pi.ProgramID)
	if err != nil {
		return nil, fmt.Errorf("programId: %w", err)
	}
	metas := make(solana.AccountMetaSlice, len(pi.Keys))
	for i, k := range pi.Keys {
		pk, err := solana.PublicKeyFromBase58(k.Pubkey)
		if err != nil {
			return nil, fmt.Errorf("keys[%d] pubkey: %w", i, err)
		}
		metas[i] = &solana.AccountMeta{PublicKey: pk, IsSigner: k.IsSigner, IsWritable: k.IsWritable}
	}
	return solana.NewInstruction(programID, metas, pi.Data), nil
}

// fetchLookupTables reads each LUT account from the RPC and extracts the
// address list. Returned as the map shape solana-go's SetAddressTables wants.
func fetchLookupTables(ctx context.Context, rpcClient *rpc.Client, addresses []string) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	tables := make(map[solana.PublicKey]solana.PublicKeySlice, len(addresses))
	for _, addr := range addresses {
		pk, err := solana.PublicKeyFromBase58(addr)
		if err != nil {
			return nil, fmt.Errorf("lut addr %s: %w", addr, err)
		}
		state, err := lookup.GetAddressLookupTable(ctx, rpcClient, pk)
		if err != nil {
			return nil, fmt.Errorf("fetch LUT %s: %w", addr, err)
		}
		tables[pk] = state.Addresses
	}
	return tables, nil
}

// mustJSON is a no-op helper kept for debugging during development; retains
// `encoding/json` usage even if local code stops referencing it directly.
func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

var _ = mustJSON
