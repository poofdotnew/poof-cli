package auth

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mr-tron/base58"
)

// Keypair holds a Solana Ed25519 keypair.
type Keypair struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Address    string // base58-encoded public key
}

// LoadKeypair loads a Solana keypair from a base58-encoded secret key or JSON byte array.
func LoadKeypair(secret string) (*Keypair, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, fmt.Errorf("empty secret key")
	}

	var secretBytes []byte

	// Try JSON array format first: [1,2,3,...]
	if strings.HasPrefix(secret, "[") {
		var arr []byte
		if err := json.Unmarshal([]byte(secret), &arr); err != nil {
			return nil, fmt.Errorf("invalid JSON keypair: %w", err)
		}
		secretBytes = arr
	} else {
		// Base58-encoded secret key
		decoded, err := base58.Decode(secret)
		if err != nil {
			return nil, fmt.Errorf("invalid base58 keypair: %w", err)
		}
		secretBytes = decoded
	}

	if len(secretBytes) != 64 {
		return nil, fmt.Errorf("invalid secret key length: got %d bytes, expected 64", len(secretBytes))
	}

	// First 32 bytes = seed, last 32 bytes = public key
	seed := secretBytes[:32]
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected public key type")
	}
	address := base58.Encode(publicKey)

	return &Keypair{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Address:    address,
	}, nil
}

// GenSolanaMessage constructs the authentication message to be signed.
func GenSolanaMessage(address, nonce string) string {
	now := time.Now().UTC()
	issuedAt := now.Format("2006-01-02T15:04:05.000Z")
	expiration := now.Add(5 * time.Minute).Format("2006-01-02T15:04:05.000Z")

	return fmt.Sprintf(
		"Sign this message to authenticate with our application.\n"+
			"\n"+
			"Wallet address:\n"+
			"%s\n"+
			"\n"+
			"Domain: server\n"+
			"Origin: server\n"+
			"Nonce: %s\n"+
			"Issued At: %s\n"+
			"Expiration Time: %s",
		address, nonce, issuedAt, expiration,
	)
}

// Sign signs a message with the keypair's private key and returns the raw signature.
func (kp *Keypair) Sign(message []byte) []byte {
	return ed25519.Sign(kp.PrivateKey, message)
}

// SignTransaction signs a serialized Solana V0 transaction.
// The wire format is: [numSigs, sig0..sigN, messageBytes].
// We find our pubkey in the message's static account keys and place
// our signature at the corresponding index.
func (kp *Keypair) SignTransaction(txBytes []byte) ([]byte, error) {
	if len(txBytes) < 2 {
		return nil, fmt.Errorf("transaction too short")
	}

	// Decode compact-u16 signature count (Solana wire format)
	numSigs, headerLen := decodeCompactU16(txBytes)
	if numSigs == 0 {
		return nil, fmt.Errorf("transaction has 0 signature slots")
	}

	sigAreaSize := headerLen + numSigs*64
	if len(txBytes) < sigAreaSize {
		return nil, fmt.Errorf("transaction too short for %d signatures", numSigs)
	}

	messageBytes := txBytes[sigAreaSize:]
	sig := kp.Sign(messageBytes)

	out := make([]byte, len(txBytes))
	copy(out, txBytes)
	copy(out[headerLen:headerLen+64], sig) // first signature slot

	return out, nil
}

// decodeCompactU16 decodes a Solana compact-u16 value from the start of buf.
// Returns (value, bytesConsumed).
func decodeCompactU16(buf []byte) (int, int) {
	if len(buf) == 0 {
		return 0, 0
	}
	b0 := int(buf[0])
	if b0 < 0x80 {
		return b0, 1
	}
	if len(buf) < 2 {
		return b0 & 0x7f, 1
	}
	b1 := int(buf[1])
	if b1 < 0x80 {
		return (b0 & 0x7f) | (b1 << 7), 2
	}
	if len(buf) < 3 {
		return (b0 & 0x7f) | ((b1 & 0x7f) << 7), 2
	}
	b2 := int(buf[2])
	return (b0 & 0x7f) | ((b1 & 0x7f) << 7) | (b2 << 14), 3
}
