package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/auth"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Generate a sign-in link for poof.new using your CLI session",
	Example: `  poof browser
  poof browser --env staging`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		// Load cached tokens to get accessToken and refreshToken
		cached, err := auth.LoadCachedTokens()
		if err != nil {
			return fmt.Errorf("no cached tokens found. Run 'poof auth login' first")
		}

		ctx := context.Background()

		// Send accessToken and refreshToken in the body.
		// The idToken and walletAddress are sent via headers by apiClient.
		body := map[string]string{
			"accessToken":  cached.AccessToken,
			"refreshToken": cached.RefreshToken,
		}

		respBody, err := apiClient.Do(ctx, "POST", "/api/auth/cli-exchange", body)
		if err != nil {
			return handleError(err)
		}

		var resp struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(respBody, &resp); err != nil {
			return fmt.Errorf("unexpected server response")
		}
		if resp.Code == "" {
			return fmt.Errorf("server returned empty exchange code")
		}

		// Build URL with code in fragment (not query param — fragments don't hit server logs)
		env, err := cfg.GetEnvironment()
		if err != nil {
			return err
		}
		url := env.BaseURL + "/auth/cli#code=" + resp.Code

		output.Print(map[string]string{
			"url": url,
		}, func() {
			output.Success("Sign-in link (expires in 30s):")
			fmt.Println()
			fmt.Printf("  %s\n", url)
			fmt.Println()
		})
		return nil
	},
}
