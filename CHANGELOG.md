# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `poof data` subcommand group — runtime reads, writes, policy queries, and atomic setMany bundles against a Poof project's data plane, all CLI-driven instead of requiring the `@pooflabs/server` SDK. Subcommands: `set`, `set-many`, `get`, `get-many`, `query`. Handles the offchain (draft / Poofnet) path via SHA-256-mock signing + Poof RPC *and* the real mainnet (preview / production) path via borsh-encoded Anchor `set_documents` signing + Solana RPC (Helius default, overridable via `POOF_SOLANA_MAINNET_RPC`). The `query` subcommand accepts `--path` for simulate queries attached to specific collections (e.g. `--path "user/<addr>/BalanceCheck/any" --name simulate`).
- `internal/tarobase` package — standalone Go client backing `poof data`. Split by concern: `client.go`, `session.go`, `items.go`, `queries.go`, `submit.go`, `submit_mainnet.go`, `mainnet_types.go`, `mainnet_borsh.go`, `mainnet_assemble.go`, `resolve.go`. Includes a hand-rolled borsh encoder for the Anchor `set_documents` args (discriminator `[79,46,72,73,24,79,66,245]`) and LUT-aware `MessageV0` assembly using the existing `solana-go` dependency. Unit tests in `*_test.go` stub HTTP via `httptest` and cover byte-level borsh output for known inputs.
- Initial CLI with 19 commands: auth, build, chat, config, credits, deploy, domain, files, iterate, keygen, logs, preferences, project, secrets, security, ship, task, template, version
- Solana keypair authentication with token caching and auto-refresh
- x402 USDC credit topup via Solana
- Composite commands: `build` (create + poll + status), `iterate` (chat + poll + test results), `ship` (scan + eligibility + deploy)
- JSON, text, and quiet output formats
- Multi-environment support: production, staging, local
- GoReleaser configuration for cross-platform releases
- GitHub Actions CI/CD (lint, test, build, release)
- Homebrew tap distribution
