package x402

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
	"github.com/poofdotnew/poof-cli/internal/api"
)

func generateTestKeypair(t *testing.T) (string, solana.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	secretKey := make([]byte, 64)
	copy(secretKey[:32], priv.Seed())
	copy(secretKey[32:], pub)
	return base58.Encode(secretKey), solana.PublicKeyFromBytes(pub)
}

func TestBuildPayment(t *testing.T) {
	privKey, _ := generateTestKeypair(t)
	_, facilitatorPubkey := generateTestKeypair(t)
	_, treasuryPubkey := generateTestKeypair(t)

	reqs := &api.PaymentRequirements{
		X402Version: 2,
		Accepts: []api.PaymentAccept{
			{
				Scheme:  "exact",
				Network: "solana:mainnet",
				Amount:  "15000000", // 15 USDC in atomic units
				PayTo:   treasuryPubkey.String(),
				Asset:   USDCMint.String(),
				Extra: api.PaymentExtra{
					FeePayer: facilitatorPubkey.String(),
				},
			},
		},
		PriceUsd: 15.0,
		Credits:  50,
	}

	// Use a realistic blockhash (base58-encoded 32 bytes)
	blockhash := "EETubP5AKHgjPAhzPkYM9phZaEFgLuRwSzHpMqRmDVY7"

	result, err := BuildPayment(privKey, reqs, blockhash)
	if err != nil {
		t.Fatalf("BuildPayment failed: %v", err)
	}

	if result == "" {
		t.Fatal("BuildPayment returned empty string")
	}

	// Decode the outer base64 to get the JSON payload
	payloadJSON, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("result is not valid base64: %v", err)
	}

	var payload PaymentPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}

	if payload.X402Version != 2 {
		t.Errorf("expected x402Version=2, got %d", payload.X402Version)
	}
	if payload.Scheme != "exact" {
		t.Errorf("expected scheme=exact, got %s", payload.Scheme)
	}
	if payload.Network != "solana:mainnet" {
		t.Errorf("expected network=solana:mainnet, got %s", payload.Network)
	}
	if payload.Payload.Transaction == "" {
		t.Error("expected non-empty transaction")
	}

	// Verify the transaction is valid base64
	_, err = base64.StdEncoding.DecodeString(payload.Payload.Transaction)
	if err != nil {
		t.Fatalf("transaction is not valid base64: %v", err)
	}
}

func TestBuildPayment_InvalidKey(t *testing.T) {
	reqs := &api.PaymentRequirements{
		Accepts: []api.PaymentAccept{{
			Extra:   api.PaymentExtra{FeePayer: "11111111111111111111111111111111"},
			PayTo:   "11111111111111111111111111111111",
			Amount:  "1000000",
			Network: "solana:mainnet",
		}},
	}

	_, err := BuildPayment("not-valid-base58!!!", reqs, "EETubP5AKHgjPAhzPkYM9phZaEFgLuRwSzHpMqRmDVY7")
	if err == nil {
		t.Error("expected error for invalid private key")
	}
	if !strings.Contains(err.Error(), "invalid private key") {
		t.Errorf("expected 'invalid private key' in error, got: %v", err)
	}
}

func TestBuildPayment_ShortKey(t *testing.T) {
	shortKey := base58.Encode([]byte("tooshort"))
	reqs := &api.PaymentRequirements{
		Accepts: []api.PaymentAccept{{
			Extra:   api.PaymentExtra{FeePayer: "11111111111111111111111111111111"},
			PayTo:   "11111111111111111111111111111111",
			Amount:  "1000000",
			Network: "solana:mainnet",
		}},
	}

	_, err := BuildPayment(shortKey, reqs, "EETubP5AKHgjPAhzPkYM9phZaEFgLuRwSzHpMqRmDVY7")
	if err == nil {
		t.Error("expected error for short private key")
	}
	if !strings.Contains(err.Error(), "invalid private key length") {
		t.Errorf("expected 'invalid private key length' in error, got: %v", err)
	}
}

func TestBuildPayment_EmptyAccepts(t *testing.T) {
	privKey, _ := generateTestKeypair(t)

	reqs := &api.PaymentRequirements{
		Accepts: []api.PaymentAccept{},
	}

	_, err := BuildPayment(privKey, reqs, "EETubP5AKHgjPAhzPkYM9phZaEFgLuRwSzHpMqRmDVY7")
	if err == nil {
		t.Error("expected error for empty Accepts")
	}
	if !strings.Contains(err.Error(), "no payment methods") {
		t.Errorf("expected 'no payment methods' in error, got: %v", err)
	}
}

func TestBuildPayment_InvalidAmount(t *testing.T) {
	privKey, _ := generateTestKeypair(t)
	_, facilitatorPubkey := generateTestKeypair(t)
	_, treasuryPubkey := generateTestKeypair(t)

	reqs := &api.PaymentRequirements{
		Accepts: []api.PaymentAccept{{
			Extra:   api.PaymentExtra{FeePayer: facilitatorPubkey.String()},
			PayTo:   treasuryPubkey.String(),
			Amount:  "not-a-number",
			Network: "solana:mainnet",
		}},
	}

	_, err := BuildPayment(privKey, reqs, "EETubP5AKHgjPAhzPkYM9phZaEFgLuRwSzHpMqRmDVY7")
	if err == nil {
		t.Error("expected error for invalid amount")
	}
	if !strings.Contains(err.Error(), "invalid amount") {
		t.Errorf("expected 'invalid amount' in error, got: %v", err)
	}
}

func TestFindSignerIndex(t *testing.T) {
	key1 := solana.MustPublicKeyFromBase58("11111111111111111111111111111111")
	key2 := solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")

	tx := &solana.Transaction{
		Message: solana.Message{
			AccountKeys: []solana.PublicKey{key1, key2},
		},
	}

	if idx := findSignerIndex(tx, key1); idx != 0 {
		t.Errorf("expected index 0 for key1, got %d", idx)
	}
	if idx := findSignerIndex(tx, key2); idx != 1 {
		t.Errorf("expected index 1 for key2, got %d", idx)
	}

	missing := solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
	if idx := findSignerIndex(tx, missing); idx != -1 {
		t.Errorf("expected -1 for missing key, got %d", idx)
	}
}

func TestGenericInstruction(t *testing.T) {
	inst := newSetComputeUnitLimitInstruction(50000)

	if !inst.ProgramID().Equals(computeBudgetProgramID) {
		t.Error("wrong program ID")
	}
	if len(inst.Accounts()) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(inst.Accounts()))
	}
	data, err := inst.Data()
	if err != nil {
		t.Fatal(err)
	}
	if data[0] != 2 {
		t.Errorf("expected discriminator 2, got %d", data[0])
	}
	if len(data) != 5 {
		t.Errorf("expected 5 bytes, got %d", len(data))
	}

	// Verify little-endian encoding of 50000
	val := uint32(data[1]) | uint32(data[2])<<8 | uint32(data[3])<<16 | uint32(data[4])<<24
	if val != 50000 {
		t.Errorf("expected encoded value 50000, got %d", val)
	}
}

func TestSetComputeUnitPriceInstruction(t *testing.T) {
	inst := newSetComputeUnitPriceInstruction(1000)
	data, err := inst.Data()
	if err != nil {
		t.Fatal(err)
	}
	if data[0] != 3 {
		t.Errorf("expected discriminator 3, got %d", data[0])
	}
	if len(data) != 9 {
		t.Errorf("expected 9 bytes, got %d", len(data))
	}

	// Verify little-endian encoding of 1000
	var val uint64
	for i := 0; i < 8; i++ {
		val |= uint64(data[1+i]) << (i * 8)
	}
	if val != 1000 {
		t.Errorf("expected encoded value 1000, got %d", val)
	}
}

func TestTransferCheckedInstruction(t *testing.T) {
	source := solana.MustPublicKeyFromBase58("11111111111111111111111111111111")
	mint := USDCMint
	dest := solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	authority := solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

	inst := newTransferCheckedInstruction(source, mint, dest, authority, 15000000, 6)

	if !inst.ProgramID().Equals(TokenProgramID) {
		t.Error("wrong program ID")
	}

	accounts := inst.Accounts()
	if len(accounts) != 4 {
		t.Fatalf("expected 4 accounts, got %d", len(accounts))
	}

	// source: writable, not signer
	if !accounts[0].IsWritable || accounts[0].IsSigner {
		t.Error("source should be writable, not signer")
	}
	// mint: not writable, not signer
	if accounts[1].IsWritable || accounts[1].IsSigner {
		t.Error("mint should not be writable or signer")
	}
	// dest: writable, not signer
	if !accounts[2].IsWritable || accounts[2].IsSigner {
		t.Error("dest should be writable, not signer")
	}
	// authority: not writable, signer
	if accounts[3].IsWritable || !accounts[3].IsSigner {
		t.Error("authority should be signer, not writable")
	}

	data, err := inst.Data()
	if err != nil {
		t.Fatal(err)
	}
	if data[0] != 12 {
		t.Errorf("expected discriminator 12, got %d", data[0])
	}
	if data[9] != 6 {
		t.Errorf("expected decimals=6, got %d", data[9])
	}

	// Verify amount encoding (15000000 in little-endian)
	var amount uint64
	for i := 0; i < 8; i++ {
		amount |= uint64(data[1+i]) << (i * 8)
	}
	if amount != 15000000 {
		t.Errorf("expected amount 15000000, got %d", amount)
	}
}
