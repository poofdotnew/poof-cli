package x402

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
	"github.com/poofdotnew/poof-cli/internal/api"
)

var (
	// USDC mint on Solana mainnet
	USDCMint = solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

	// SPL Token program
	TokenProgramID = solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")

	// Associated Token Account program
	ATAProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
)

// PaymentPayload is the x402 v2 payment structure.
type PaymentPayload struct {
	X402Version int    `json:"x402Version"`
	Scheme      string `json:"scheme"`
	Network     string `json:"network"`
	Payload     struct {
		Transaction string `json:"transaction"`
	} `json:"payload"`
}

// BuildPayment constructs a signed x402 payment for the topup endpoint.
// It builds a USDC transfer transaction with the facilitator as fee payer,
// partially signs it, and encodes it as a base64 x402 v2 PaymentPayload.
func BuildPayment(
	privateKeyBase58 string,
	reqs *api.PaymentRequirements,
	recentBlockhash string,
) (string, error) {
	if len(reqs.Accepts) == 0 {
		return "", fmt.Errorf("no payment methods in response")
	}
	accept := reqs.Accepts[0]

	// Parse addresses
	facilitatorPubkey := solana.MustPublicKeyFromBase58(accept.Extra.FeePayer)
	treasuryPubkey := solana.MustPublicKeyFromBase58(accept.PayTo)

	// Load keypair
	secretBytes, err := base58.Decode(privateKeyBase58)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	if len(secretBytes) != 64 {
		return "", fmt.Errorf("invalid private key length: %d", len(secretBytes))
	}
	privKey := ed25519.NewKeyFromSeed(secretBytes[:32])
	pubKey, ok := privKey.Public().(ed25519.PublicKey)
	if !ok {
		return "", fmt.Errorf("unexpected public key type")
	}
	walletPubkey := solana.PublicKeyFromBytes(pubKey)

	// Parse amount (already in USDC atomic units)
	amount, err := strconv.ParseUint(accept.Amount, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid amount %q: %w", accept.Amount, err)
	}

	// Derive associated token accounts
	senderATA, _, err := solana.FindProgramAddress(
		[][]byte{
			walletPubkey.Bytes(),
			TokenProgramID.Bytes(),
			USDCMint.Bytes(),
		},
		ATAProgramID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to derive sender ATA: %w", err)
	}

	treasuryATA, _, err := solana.FindProgramAddress(
		[][]byte{
			treasuryPubkey.Bytes(),
			TokenProgramID.Bytes(),
			USDCMint.Bytes(),
		},
		ATAProgramID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to derive treasury ATA: %w", err)
	}

	// Build transaction with exactly 3 instructions:
	// 1. SetComputeUnitLimit (≤50000)
	// 2. SetComputeUnitPrice
	// 3. TransferChecked (USDC)
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			newSetComputeUnitLimitInstruction(50000),
			newSetComputeUnitPriceInstruction(1000),
			newTransferCheckedInstruction(
				senderATA,
				USDCMint,
				treasuryATA,
				walletPubkey,
				amount,
				6, // USDC decimals
			),
		},
		solana.MustHashFromBase58(recentBlockhash),
		solana.TransactionPayer(facilitatorPubkey),
	)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Pre-allocate signature slots if NewTransaction didn't
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	if len(tx.Signatures) < numSigners {
		sigs := make([]solana.Signature, numSigners)
		copy(sigs, tx.Signatures)
		tx.Signatures = sigs
	}

	// Partially sign with wallet key (not facilitator)
	walletIdx := findSignerIndex(tx, walletPubkey)
	if walletIdx < 0 {
		return "", fmt.Errorf("wallet %s not found in transaction signers", walletPubkey)
	}
	solanaPrivKey := solana.PrivateKey(privKey)
	sig, err := signTransaction(tx, solanaPrivKey)
	if err != nil {
		return "", err
	}
	tx.Signatures[walletIdx] = sig

	// Serialize with requireAllSignatures=false
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}
	txBase64 := base64.StdEncoding.EncodeToString(txBytes)

	// Build x402 v2 PaymentPayload
	payload := PaymentPayload{
		X402Version: 2,
		Scheme:      "exact",
		Network:     accept.Network,
	}
	payload.Payload.Transaction = txBase64

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payment payload: %w", err)
	}

	return base64.StdEncoding.EncodeToString(payloadJSON), nil
}

func signTransaction(tx *solana.Transaction, privKey solana.PrivateKey) (solana.Signature, error) {
	messageBytes, err := tx.Message.MarshalBinary()
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to marshal transaction message: %w", err)
	}
	sig := ed25519.Sign(ed25519.PrivateKey(privKey), messageBytes)
	var solSig solana.Signature
	copy(solSig[:], sig)
	return solSig, nil
}

func findSignerIndex(tx *solana.Transaction, pubkey solana.PublicKey) int {
	for i, key := range tx.Message.AccountKeys {
		if key.Equals(pubkey) {
			return i
		}
	}
	return -1
}

// ComputeBudget program ID
var computeBudgetProgramID = solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")

// Instruction builders (raw, no external SPL token dependency needed)

func newSetComputeUnitLimitInstruction(units uint32) solana.Instruction {
	data := make([]byte, 5)
	data[0] = 2 // SetComputeUnitLimit instruction discriminator
	data[1] = byte(units)
	data[2] = byte(units >> 8)
	data[3] = byte(units >> 16)
	data[4] = byte(units >> 24)

	return &genericInstruction{
		programID: computeBudgetProgramID,
		accounts:  []*solana.AccountMeta{},
		data:      data,
	}
}

func newSetComputeUnitPriceInstruction(microLamports uint64) solana.Instruction {
	data := make([]byte, 9)
	data[0] = 3 // SetComputeUnitPrice instruction discriminator
	for i := 0; i < 8; i++ {
		data[1+i] = byte(microLamports >> (i * 8))
	}

	return &genericInstruction{
		programID: computeBudgetProgramID,
		accounts:  []*solana.AccountMeta{},
		data:      data,
	}
}

func newTransferCheckedInstruction(
	source, mint, dest, authority solana.PublicKey,
	amount uint64,
	decimals uint8,
) solana.Instruction {
	// SPL Token TransferChecked instruction (index 12)
	data := make([]byte, 10)
	data[0] = 12 // TransferChecked discriminator
	for i := 0; i < 8; i++ {
		data[1+i] = byte(amount >> (i * 8))
	}
	data[9] = decimals

	return &genericInstruction{
		programID: TokenProgramID,
		accounts: []*solana.AccountMeta{
			{PublicKey: source, IsSigner: false, IsWritable: true},
			{PublicKey: mint, IsSigner: false, IsWritable: false},
			{PublicKey: dest, IsSigner: false, IsWritable: true},
			{PublicKey: authority, IsSigner: true, IsWritable: false},
		},
		data: data,
	}
}

// genericInstruction implements solana.Instruction.
type genericInstruction struct {
	programID solana.PublicKey
	accounts  []*solana.AccountMeta
	data      []byte
}

func (i *genericInstruction) ProgramID() solana.PublicKey     { return i.programID }
func (i *genericInstruction) Accounts() []*solana.AccountMeta { return i.accounts }
func (i *genericInstruction) Data() ([]byte, error)           { return i.data, nil }
