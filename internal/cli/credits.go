package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/x402"
	"github.com/spf13/cobra"
)

var creditsCmd = &cobra.Command{
	Use:   "credits",
	Short: "Manage credits",
}

var creditsBalanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "Check credit balance",
	Example: `  poof credits balance
  poof credits balance --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		resp, err := apiClient.GetCredits(context.Background())
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			c := resp.Credits
			output.Info("Credits:")
			output.Info("  Daily:  %d / %d (resets %s)", c.Daily.Remaining, c.Daily.Allotted, c.Daily.ResetsAt)
			output.Info("  Add-on: %d / %d purchased", c.AddOn.Remaining, c.AddOn.Purchased)
			output.Info("  Total:  %d", c.Total)
		})
		return nil
	},
}

var creditsTopupCmd = &cobra.Command{
	Use:   "topup",
	Short: "Buy credits via x402 USDC payment",
	Long: `Buy credits using USDC on Solana via the x402 payment protocol.

This command handles the full two-phase x402 flow:
  1. Requests payment requirements from the server (402 response)
  2. Builds a USDC transfer transaction with the PayAI facilitator
  3. Partially signs and submits the payment to complete the purchase

Your wallet must have sufficient USDC balance on Solana mainnet.

Pricing: 1 package = $15 USDC = 50 credits (up to 10 packages per purchase).
A completed purchase also unlocks paid features (deployment, downloads, etc.).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		quantity, _ := cmd.Flags().GetInt("quantity")
		if quantity < 1 || quantity > 10 {
			return fmt.Errorf("quantity must be 1-10")
		}

		ctx := context.Background()

		// Phase 1: Get payment requirements
		output.Info("Requesting payment requirements...")
		reqs, err := apiClient.TopupPhase1(ctx, quantity)
		if err != nil {
			return handleError(err)
		}

		if len(reqs.Accepts) == 0 {
			return fmt.Errorf("server returned no payment methods")
		}
		accept := reqs.Accepts[0]
		amountUsdc, err := strconv.ParseFloat(accept.Amount, 64)
		if err != nil {
			return fmt.Errorf("invalid payment amount %q: %w", accept.Amount, err)
		}
		amountUsdc /= 1e6 // Convert atomic units to USDC

		output.Info("Payment required:")
		output.Info("  Amount:    $%.2f USDC", amountUsdc)
		output.Info("  Credits:   %d", reqs.Credits)
		output.Info("  Pay to:    %s", accept.PayTo)
		output.Info("  Facilitator: %s", accept.Extra.FeePayer)

		// Get recent blockhash from Solana RPC
		output.Info("Fetching recent blockhash...")
		blockhash, err := getRecentBlockhash(ctx)
		if err != nil {
			return fmt.Errorf("failed to get blockhash: %w", err)
		}

		// Phase 2: Build and sign transaction
		output.Info("Building payment transaction...")
		paymentHeader, err := x402.BuildPayment(cfg.SolanaPrivateKey, reqs, blockhash)
		if err != nil {
			return fmt.Errorf("failed to build payment: %w", err)
		}

		// Phase 3: Submit payment
		output.Info("Submitting payment...")
		result, err := apiClient.TopupPhase2(ctx, quantity, paymentHeader)
		if err != nil {
			return handleError(err)
		}

		output.Print(result, func() {
			output.Success("Payment successful!")
			output.Info("  Credits purchased: %d", result.Credits)
			output.Info("  Amount:           $%.2f USDC", result.PriceUsd)
			if result.TxID != "" {
				output.Info("  Transaction:      %s", result.TxID)
			}
		})
		return nil
	},
}

// getRecentBlockhash fetches the latest blockhash from Solana RPC.
func getRecentBlockhash(ctx context.Context) (string, error) {
	rpcURL := cfg.SolanaRPCURL

	body := `{"jsonrpc":"2.0","id":1,"method":"getLatestBlockhash","params":[{"commitment":"finalized"}]}`

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Solana RPC returned status %d", resp.StatusCode)
	}

	var rpcResp struct {
		Result struct {
			Value struct {
				Blockhash string `json:"blockhash"`
			} `json:"value"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return "", err
	}

	if rpcResp.Error != nil {
		return "", fmt.Errorf("Solana RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	if rpcResp.Result.Value.Blockhash == "" {
		return "", fmt.Errorf("empty blockhash from Solana RPC")
	}

	return rpcResp.Result.Value.Blockhash, nil
}

func init() {
	creditsTopupCmd.Flags().Int("quantity", 1, "Number of credit packages to buy (1-10, each = 50 credits)")

	creditsCmd.AddCommand(creditsBalanceCmd)
	creditsCmd.AddCommand(creditsTopupCmd)
}
