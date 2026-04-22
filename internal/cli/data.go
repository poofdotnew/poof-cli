package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/poofdotnew/poof-cli/internal/tarobase"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// `poof data` — write and read documents against a Poof project's data plane.
// Draft (poofnet / offchain) uses the SHA-256-mock signing path + Poof's
// offchain RPC. Preview and production (Solana mainnet) do real Anchor
// borsh-encoded `set_documents` signing + Helius RPC submit. Both paths are
// wired end-to-end through internal/tarobase.
// ---------------------------------------------------------------------------

var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "Read and write documents via the Tarobase data plane",
	Long: `Work with your project's runtime data — reads, writes, policy queries,
and atomic setMany bundles — without dropping to raw REST.

Two targeting modes:
  - Project-based (default): -p <project-id> + -e draft|preview|production.
    Looks up the right appId from the project's connectionInfo. Requires
    that you own (or have access to) the project.
  - Shared-appid: --app-id <id> + --chain offchain|mainnet. Talks to a
    Tarobase appid directly — e.g. someone else's deployed primitives
    library. Skips the project lookup; auth is still your own wallet.`,
}

var (
	flagDataEnv   string
	flagDataAppID string
	flagDataChain string
)

func dataClient(ctx context.Context) (*tarobase.Client, error) {
	if err := requireAuth(); err != nil {
		return nil, err
	}
	resolved, err := resolveDataTarget(ctx)
	if err != nil {
		return nil, err
	}
	return tarobase.NewClient(ctx, &tarobase.Config{
		AppID:      resolved.AppID,
		Chain:      resolved.Chain,
		PrivateKey: cfg.SolanaPrivateKey,
		APIURL:     resolved.APIURL,
		AuthURL:    resolved.AuthURL,
	})
}

// resolveDataTarget picks between shared-appid mode (--app-id/--chain,
// pointing directly at a Tarobase appid someone else has deployed) and
// project-based mode (-p/-e, looking up the appId from connectionInfo).
// Enforces that the two modes don't mix — --environment is project-scoped
// and --chain is appid-scoped.
func resolveDataTarget(ctx context.Context) (*tarobase.ResolvedEnv, error) {
	if flagDataAppID != "" {
		if flagDataEnv != "" {
			return nil, fmt.Errorf("--environment conflicts with --app-id (environment is a project-based concept; --app-id points at a specific Tarobase appid). Pick one.")
		}
		chain, err := parseChainFlag(flagDataChain)
		if err != nil {
			return nil, err
		}
		return &tarobase.ResolvedEnv{
			AppID:   flagDataAppID,
			Chain:   chain,
			APIURL:  "https://api.tarobase.com",
			AuthURL: "https://auth.tarobase.com",
		}, nil
	}
	if flagDataChain != "" {
		return nil, fmt.Errorf("--chain only applies with --app-id. Drop --chain and use -e draft|preview|production for project-based mode.")
	}
	projectID, err := getProjectID()
	if err != nil {
		return nil, err
	}
	env, err := tarobase.ParseEnvironment(flagDataEnv)
	if err != nil {
		return nil, err
	}
	return tarobase.Resolve(ctx, apiClient, projectID, env)
}

// parseChainFlag maps the user-facing --chain value to the internal Chain
// constant. Accepts `mainnet` as a shorthand for `solana_mainnet` since
// nothing else routes through the mainnet path today.
func parseChainFlag(s string) (tarobase.Chain, error) {
	switch s {
	case "":
		return "", fmt.Errorf("--chain is required when --app-id is set (offchain | mainnet)")
	case "offchain":
		return tarobase.ChainOffchain, nil
	case "mainnet", "solana_mainnet":
		return tarobase.ChainMainnet, nil
	default:
		return "", fmt.Errorf("invalid --chain %q (valid: offchain, mainnet)", s)
	}
}

// ---- data set ----------------------------------------------------------------

var dataSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Write a single document",
	Example: `  poof data set -p <id> --path memories/<addr> --data '{"content":"hi"}'
  poof data set -p <id> --path user/<addr>/TimeWindow/tw1 --data '{"startTime":0,"endTime":4102444800}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		path, _ := cmd.Flags().GetString("path")
		data, _ := cmd.Flags().GetString("data")
		skipPreflight, _ := cmd.Flags().GetBool("skip-preflight")
		if path == "" || data == "" {
			return fmt.Errorf("--path and --data are required")
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(data), &doc); err != nil {
			return fmt.Errorf("invalid --data JSON: %w", err)
		}
		client, err := dataClient(ctx)
		if err != nil {
			return err
		}
		return runAndSubmit(ctx, client, []tarobase.Document{{Path: path, Document: doc}}, skipPreflight)
	},
}

// ---- data set-many -----------------------------------------------------------

var dataSetManyCmd = &cobra.Command{
	Use:   "set-many",
	Short: "Atomically write multiple documents (all-or-nothing)",
	Long: `Write multiple documents in a single Tarobase transaction. If any
rule, hook, or on-chain check fails, the whole bundle rejects and nothing
is applied.

Payload shape (--from-json): either a raw array of {path, document}, or an
object {"documents":[{path, document}, ...]}. Stdin is accepted via
--from-json /dev/stdin.`,
	Example: `  cat <<'EOF' | poof data set-many -p <id> --from-json /dev/stdin
  [
    {"path":"user/<addr>/TimeWindow/g1","document":{"startTime":0,"endTime":4102444800}},
    {"path":"user/<addr>/BalanceCheck/g1","document":{"mint":"...","op":"gte","threshold":0}}
  ]
  EOF`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fromJSON, _ := cmd.Flags().GetString("from-json")
		skipPreflight, _ := cmd.Flags().GetBool("skip-preflight")
		if fromJSON == "" {
			return fmt.Errorf("--from-json is required")
		}
		raw, err := os.ReadFile(fromJSON)
		if err != nil {
			return fmt.Errorf("read %s: %w", fromJSON, err)
		}
		docs, err := parseSetManyPayload(raw)
		if err != nil {
			return err
		}
		client, err := dataClient(ctx)
		if err != nil {
			return err
		}
		return runAndSubmit(ctx, client, docs, skipPreflight)
	},
}

// parseSetManyPayload accepts either a bare array of {path, document} or an
// {"documents":[...]} wrapper — both are common in agent-generated files.
func parseSetManyPayload(raw []byte) ([]tarobase.Document, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("empty payload")
	}
	if trimmed[0] == '[' {
		var arr []tarobaseDocInput
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("parse array: %w", err)
		}
		return toDocs(arr)
	}
	var wrap struct {
		Documents []tarobaseDocInput `json:"documents"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("parse object: %w", err)
	}
	return toDocs(wrap.Documents)
}

type tarobaseDocInput struct {
	Path     string         `json:"path"`
	Document map[string]any `json:"document"`
}

func toDocs(in []tarobaseDocInput) ([]tarobase.Document, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("no documents in payload")
	}
	out := make([]tarobase.Document, len(in))
	for i, d := range in {
		if d.Path == "" {
			return nil, fmt.Errorf("entry %d: path is required", i)
		}
		out[i] = tarobase.Document{Path: d.Path, Document: d.Document}
	}
	return out, nil
}

func runAndSubmit(ctx context.Context, client *tarobase.Client, docs []tarobase.Document, skipPreflight bool) error {
	result, err := client.SetManyAndSubmit(ctx, docs, tarobase.SubmitOptions{SkipPreflight: skipPreflight})
	if err != nil {
		return handleError(err)
	}
	output.Print(result, func() {
		output.Success("submitted %d document(s) on %s (txid=%s)",
			len(docs), result.Chain, result.TransactionID)
	})
	return nil
}

// ---- data get ----------------------------------------------------------------

var dataGetCmd = &cobra.Command{
	Use:     "get",
	Short:   "Read a document or list a collection by path",
	Example: `  poof data get -p <id> --path memories/<addr>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		path, _ := cmd.Flags().GetString("path")
		if path == "" {
			return fmt.Errorf("--path is required")
		}
		client, err := dataClient(ctx)
		if err != nil {
			return err
		}
		raw, err := client.Get(ctx, path)
		if err != nil {
			return handleError(err)
		}
		return emitRaw(raw)
	},
}

var dataGetManyCmd = &cobra.Command{
	Use:     "get-many",
	Short:   "Batch-read multiple paths",
	Long:    `Reads each path in the --from-json payload (a JSON array of strings) and returns one result per path in the same order.`,
	Example: `  echo '["memories/<addr>","user/<addr>"]' | poof data get-many -p <id> --from-json /dev/stdin`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fromJSON, _ := cmd.Flags().GetString("from-json")
		if fromJSON == "" {
			return fmt.Errorf("--from-json is required")
		}
		raw, err := os.ReadFile(fromJSON)
		if err != nil {
			return fmt.Errorf("read %s: %w", fromJSON, err)
		}
		var paths []string
		if err := json.Unmarshal(raw, &paths); err != nil {
			return fmt.Errorf("expected a JSON array of path strings: %w", err)
		}
		client, err := dataClient(ctx)
		if err != nil {
			return err
		}
		results, err := client.GetMany(ctx, paths)
		if err != nil {
			return handleError(err)
		}
		return emitRaw(json.RawMessage(mustMarshal(map[string]any{"results": results})))
	},
}

// ---- data query --------------------------------------------------------------

var dataQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Run a policy query",
	Long: `Run a policy query.

By default queries run against the global queries/$queryId collection —
--name getSolBalance hits path queries/getSolBalance. Simulate queries
attached to a specific collection (e.g. simulate on BalanceCheck) need
an explicit --path so the server knows which collection's queries map to
look in, e.g. --path user/<addr>/BalanceCheck/any --name simulate.`,
	Example: `  poof data query -p <id> --name getSolBalance --args '{"address":"<addr>"}'
  poof data query -p <id> --name getSolPriceInUSD
  poof data query -p <id> --path 'user/<addr>/BalanceCheck/any' --name simulate --args '{"mint":"...","op":"gte","threshold":100}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		name, _ := cmd.Flags().GetString("name")
		path, _ := cmd.Flags().GetString("path")
		argsJSON, _ := cmd.Flags().GetString("args")
		if name == "" {
			return fmt.Errorf("--name is required (e.g. getSolBalance)")
		}
		qargs := map[string]any{}
		if argsJSON != "" {
			if err := json.Unmarshal([]byte(argsJSON), &qargs); err != nil {
				return fmt.Errorf("invalid --args JSON: %w", err)
			}
		}
		client, err := dataClient(ctx)
		if err != nil {
			return err
		}
		// Default path = "queries/<name>". When --path is given, we use it
		// verbatim and go through RunQueryMany so path can be anywhere.
		if path == "" {
			result, err := client.RunQuery(ctx, name, qargs)
			if err != nil {
				return handleError(err)
			}
			output.Print(result, func() {
				output.Info("%s -> %s", name, string(result.Result))
			})
			return nil
		}
		results, err := client.RunQueryMany(ctx, []tarobase.Query{{
			Path: path, QueryName: name, QueryArgs: qargs,
		}})
		if err != nil {
			return handleError(err)
		}
		if len(results) != 1 {
			return fmt.Errorf("expected 1 result, got %d", len(results))
		}
		r := results[0]
		output.Print(&r, func() {
			output.Info("%s @ %s -> %s", name, path, string(r.Result))
		})
		return nil
	},
}

// ---- data app-ids ------------------------------------------------------------

var dataAppIDsCmd = &cobra.Command{
	Use:   "app-ids",
	Short: "Show a project's Tarobase appIds per environment",
	Long: `List the Tarobase appIds provisioned for a project — one per environment
(draft, preview, production).

Background: a Poof project ID (what you pass to most CLI commands via -p)
is your handle on the project — it owns builds, deploys, and settings. A
Tarobase appId is the data-plane address of a specific deployed policy
instance: each environment of a project gets its own appId, so the draft
(off-chain / Poofnet) and real mainnet instances stay cleanly separated.

Use appIds with ` + "`poof data --app-id <id> --chain offchain|mainnet`" + ` to
talk to a specific appId directly — useful when pointing at a shared
primitives library someone else has deployed, without needing Poof-project
access to it.`,
	Example: `  poof data app-ids -p <project-id>
  poof data app-ids -p <project-id> --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		if err := requireAuth(); err != nil {
			return err
		}
		projectID, err := getProjectID()
		if err != nil {
			return err
		}
		status, err := apiClient.GetProjectStatus(ctx, projectID)
		if err != nil {
			return handleError(err)
		}
		ci := status.ConnectionInfo
		if ci == nil {
			return fmt.Errorf("project %s has no connectionInfo — has it been built yet?", projectID)
		}
		type envRow struct {
			Env   string `json:"env"`
			AppID string `json:"appId"`
			Chain string `json:"chain"`
		}
		mk := func(env, chain string, c *api.ConnectionEnv) envRow {
			if c == nil || c.TarobaseAppId == "" {
				return envRow{Env: env}
			}
			return envRow{Env: env, AppID: c.TarobaseAppId, Chain: chain}
		}
		rows := []envRow{
			mk("draft", "offchain", ci.Draft),
			mk("preview", "mainnet", ci.Preview),
			mk("production", "mainnet", ci.Production),
		}
		output.Print(map[string]any{"projectId": projectID, "envs": rows}, func() {
			output.Info("project %s", projectID)
			for _, r := range rows {
				if r.AppID == "" {
					output.Info("  %-10s (not provisioned)", r.Env)
					continue
				}
				output.Info("  %-10s appId=%s chain=%s", r.Env, r.AppID, r.Chain)
			}
		})
		return nil
	},
}

func emitRaw(raw json.RawMessage) error {
	output.Print(json.RawMessage(raw), func() {
		fmt.Println(string(raw))
	})
	return nil
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func init() {
	dataCmd.PersistentFlags().StringVarP(&flagDataEnv, "environment", "e", "", "Project-based mode: target environment — draft (default), preview, production. Ignored when --app-id is set.")
	dataCmd.PersistentFlags().StringVar(&flagDataAppID, "app-id", "", "Shared-appid mode: talk directly to a Tarobase appid (e.g. a shared primitives library someone else deployed). Requires --chain; skips -p and -e.")
	dataCmd.PersistentFlags().StringVar(&flagDataChain, "chain", "", "Chain for --app-id: offchain | mainnet")

	dataSetCmd.Flags().String("path", "", "Tarobase path (e.g. memories/<addr>)")
	dataSetCmd.Flags().String("data", "", "JSON document body")
	dataSetCmd.Flags().Bool("skip-preflight", false, "Mainnet only: skip RPC preflight simulation so failing txs still land on-chain (visible on Solscan, 5000-lamport fee each). Use for auditing intentional-failure scenarios.")
	_ = dataSetCmd.MarkFlagRequired("path")
	_ = dataSetCmd.MarkFlagRequired("data")

	dataSetManyCmd.Flags().String("from-json", "", "Path to a JSON file with the bundle (or /dev/stdin)")
	dataSetManyCmd.Flags().Bool("skip-preflight", false, "Mainnet only: skip RPC preflight simulation so failing bundles still land on-chain. See `poof data set --help` for use cases.")
	_ = dataSetManyCmd.MarkFlagRequired("from-json")

	dataGetCmd.Flags().String("path", "", "Tarobase path")
	_ = dataGetCmd.MarkFlagRequired("path")

	dataGetManyCmd.Flags().String("from-json", "", "Path to a JSON array of paths (or /dev/stdin)")
	_ = dataGetManyCmd.MarkFlagRequired("from-json")

	dataQueryCmd.Flags().String("name", "", "Policy query name (e.g. getSolBalance, simulate)")
	dataQueryCmd.Flags().String("args", "", "Query args as JSON (default: {})")
	dataQueryCmd.Flags().String("path", "", "Override the query path (default: queries/<name>). Use for simulate queries attached to a specific collection, e.g. user/<addr>/BalanceCheck/any")
	_ = dataQueryCmd.MarkFlagRequired("name")

	dataCmd.AddCommand(dataSetCmd)
	dataCmd.AddCommand(dataSetManyCmd)
	dataCmd.AddCommand(dataGetCmd)
	dataCmd.AddCommand(dataGetManyCmd)
	dataCmd.AddCommand(dataQueryCmd)
	dataCmd.AddCommand(dataAppIDsCmd)
}
