package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

func TestLoadKeypair_Base58(t *testing.T) {
	// Generate a keypair, encode as base58, and verify round-trip
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	secretKey := make([]byte, 64)
	copy(secretKey[:32], priv.Seed())
	copy(secretKey[32:], pub)
	encoded := base58.Encode(secretKey)

	kp, err := LoadKeypair(encoded)
	if err != nil {
		t.Fatalf("LoadKeypair failed: %v", err)
	}

	if kp.Address != base58.Encode(pub) {
		t.Errorf("address mismatch: got %s, want %s", kp.Address, base58.Encode(pub))
	}

	if len(kp.PrivateKey) != ed25519.PrivateKeySize {
		t.Errorf("private key wrong size: got %d, want %d", len(kp.PrivateKey), ed25519.PrivateKeySize)
	}
}

func TestLoadKeypair_Empty(t *testing.T) {
	_, err := LoadKeypair("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestLoadKeypair_InvalidBase58(t *testing.T) {
	_, err := LoadKeypair("not-valid-base58!!!")
	if err == nil {
		t.Fatal("expected error for invalid base58")
	}
}

func TestLoadKeypair_WrongLength(t *testing.T) {
	short := base58.Encode([]byte("tooshort"))
	_, err := LoadKeypair(short)
	if err == nil {
		t.Fatal("expected error for wrong length key")
	}
	if !strings.Contains(err.Error(), "invalid secret key length") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGenSolanaMessage(t *testing.T) {
	msg := GenSolanaMessage("ABC123", "nonce456")

	if !strings.HasPrefix(msg, "Sign this message to authenticate with our application.") {
		t.Error("message missing prefix")
	}
	if !strings.Contains(msg, "Wallet address:\nABC123") {
		t.Error("message missing wallet address")
	}
	if !strings.Contains(msg, "Domain: server") {
		t.Error("message missing domain")
	}
	if !strings.Contains(msg, "Origin: server") {
		t.Error("message missing origin")
	}
	if !strings.Contains(msg, "Nonce: nonce456") {
		t.Error("message missing nonce")
	}
	if !strings.Contains(msg, "Issued At:") {
		t.Error("message missing issued at")
	}
	if !strings.Contains(msg, "Expiration Time:") {
		t.Error("message missing expiration time")
	}

	// Verify ISO 8601 format with milliseconds
	lines := strings.Split(msg, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Issued At: ") {
			ts := strings.TrimPrefix(line, "Issued At: ")
			if !strings.HasSuffix(ts, "Z") {
				t.Errorf("timestamp missing Z suffix: %s", ts)
			}
			// Should have 3 decimal places for milliseconds
			if !strings.Contains(ts, ".") {
				t.Errorf("timestamp missing milliseconds: %s", ts)
			}
		}
	}
}

func TestSign(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	secretKey := make([]byte, 64)
	copy(secretKey[:32], priv.Seed())
	copy(secretKey[32:], pub)

	kp, err := LoadKeypair(base58.Encode(secretKey))
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("test message")
	sig := kp.Sign(message)

	if len(sig) != ed25519.SignatureSize {
		t.Errorf("signature wrong size: got %d, want %d", len(sig), ed25519.SignatureSize)
	}

	// Verify signature
	if !ed25519.Verify(pub, message, sig) {
		t.Error("signature verification failed")
	}

	// Verify base64 encoding works (as used in session.go)
	sig64 := base64.StdEncoding.EncodeToString(sig)
	if sig64 == "" {
		t.Error("base64 encoding produced empty string")
	}
}
