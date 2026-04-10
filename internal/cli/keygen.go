package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"

	"github.com/mr-tron/base58"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var keygenCmd = &cobra.Command{
	Use:   "keygen",
	Short: "Generate a new Solana keypair",
	Long:  "Generate a new Ed25519 keypair for use with Poof. Add the output to your .env file.",
	Example: `  poof keygen
  poof keygen >> .env
  poof keygen --json | jq -r '.privateKey'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate keypair: %w", err)
		}

		// Solana stores the full 64-byte secret key (seed + public key)
		secretKey := make([]byte, 64)
		copy(secretKey[:32], priv.Seed())
		copy(secretKey[32:], pub)

		privateKeyB58 := base58.Encode(secretKey)
		walletAddress := base58.Encode(pub)

		data := map[string]string{
			"privateKey":    privateKeyB58,
			"walletAddress": walletAddress,
		}

		// `poof keygen --json` should still emit a JSON document even if
		// redirected, so JSON mode wins over redirect detection.
		if output.GetFormat() == output.FormatJSON {
			output.JSON(data)
			return nil
		}

		// When stdout is redirected (e.g. `poof keygen >> .env`) or the user
		// asked for quiet mode, emit ONLY the env-var lines so the output is
		// drop-in for .env — no preamble, no trailing Info().
		stdoutIsTTY := term.IsTerminal(int(os.Stdout.Fd()))
		if output.GetFormat() == output.FormatQuiet || !stdoutIsTTY {
			fmt.Printf("SOLANA_PRIVATE_KEY=%s\n", privateKeyB58)
			fmt.Printf("SOLANA_WALLET_ADDRESS=%s\n", walletAddress)
			return nil
		}

		// Interactive terminal text output: show the friendly preamble.
		fmt.Println("Generated new Solana keypair:")
		fmt.Println()
		fmt.Printf("SOLANA_PRIVATE_KEY=%s\n", privateKeyB58)
		fmt.Printf("SOLANA_WALLET_ADDRESS=%s\n", walletAddress)
		fmt.Println()
		output.Info("Add these to your .env file to use with Poof.")
		return nil
	},
}
