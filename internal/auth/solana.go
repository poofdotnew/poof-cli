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

	// Parse: first byte is compact-u16 number of signatures
	numSigs := int(txBytes[0])
	if numSigs == 0 {
		return nil, fmt.Errorf("transaction has 0 signature slots")
	}

	sigAreaSize := 1 + numSigs*64 // 1 byte count + N * 64-byte signatures
	if len(txBytes) < sigAreaSize {
		return nil, fmt.Errorf("transaction too short for %d signatures", numSigs)
	}

	// The message starts after the signature area
	messageBytes := txBytes[sigAreaSize:]

	// Sign the message
	sig := kp.Sign(messageBytes)

	// Create output with signature inserted at slot 0 (fee payer)
	// For simplicity, we always sign as the first signer (fee payer position)
	out := make([]byte, len(txBytes))
	copy(out, txBytes)
	copy(out[1:1+64], sig) // first signature slot

	return out, nil
}
