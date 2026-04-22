// Package tarobase is the Go client backing `poof data` — reads, writes,
// policy queries, and atomic setMany bundles against a Poof project's runtime
// data plane. Split by concern:
//
//	client.go           — HTTP plumbing, headers, doExpect wrapper, ServerError
//	session.go          — nonce + Solana-signed session, scoped to an appId
//	items.go            — /items GET/PUT for single + batch reads and writes
//	queries.go          — /queries POST for policy read-only queries
//	submit.go           — dispatches SetManyAndSubmit to the right chain path
//	submit_mainnet.go   — Solana RPC submit entry point for mainnet
//	mainnet_types.go    — JSON types + FieldValue union parser
//	mainnet_borsh.go    — hand-rolled borsh encoder for Anchor set_documents args
//	mainnet_assemble.go — LUT-aware VersionedTransaction assembly + sign
//	resolve.go          — project ID + environment -> per-env appId + chain
//
// Usage pattern (mirrored in internal/cli/data.go):
//
//	resolved, err := tarobase.Resolve(ctx, poofAPI, projectID, tarobase.EnvDraft)
//	client, err   := tarobase.NewClient(ctx, tarobase.Config{
//	    AppID: resolved.AppID, Chain: resolved.Chain,
//	    PrivateKey: cfg.SolanaPrivateKey,
//	})
//	res, err := client.SetManyAndSubmit(ctx, []tarobase.Document{...})
//
// Test coverage lives in *_test.go alongside each file and runs via `go test
// ./internal/tarobase/...` — no network calls, all HTTP stubbed through
// httptest.Server.
package tarobase
