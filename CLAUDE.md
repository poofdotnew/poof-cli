# CLAUDE.md

Guidelines for AI agents (Claude Code, Codex, Gemini, etc.) working on poof-cli.

## Project overview

Go CLI for [poof.new](https://poof.new) — build, deploy, and manage Solana dApps.
Entry point: `cmd/poof/main.go` -> `internal/cli.Execute()`.

## Build & test

```bash
make build          # build to bin/poof
make test           # go test -race -count=1 ./...
make lint           # golangci-lint run ./...
make fmt            # gofmt -w .
make vet            # go vet ./...
make all            # lint + test + build
```

If `make build` fails due to xcode-select, use: `CGO_ENABLED=0 go build -o bin/poof ./cmd/poof/`

Always run `make test` after changes. The pre-commit hook runs fmt, vet, lint, build, and test — all must pass.

## Project structure

```
cmd/poof/           Entry point (thin wrapper)
internal/
  cli/              Cobra command definitions (one file per command group)
  api/              REST API client (Client.Do / Client.DoRaw)
  auth/             Solana auth, session tokens, token cache
  config/           Config loading (flags > env > .env > ~/.poof/config.yaml)
  output/           Text/JSON/Quiet formatting (fatih/color)
  poll/             Async task polling
  tarobase/         Tarobase data plane client (items, queries, submit)
  version/          Version info injected via ldflags
  x402/             Solana USDC payment flow
```

## Realtime DO Architecture

The TaroBase platform has a new Cloudflare Durable Object (DO) based realtime engine that replaces the legacy Express/MongoDB client API for realtime apps.

### Key concepts
- **Chains**: `offchain` (legacy Express API), `solana_mainnet` (Solana RPC), `realtime_offchain` (CF DO)
- **Routing**: The CF worker at `tarobase-realtime-staging.buildwithtarobase.workers.dev` (staging) or `tarobase-realtime.buildwithtarobase.workers.dev` (prod) checks KV for the appId — if present routes to DO, otherwise proxies to legacy
- **Data plane**: The DO uses SQLite for storage, WebSocket Hibernation for subscriptions, and compiled bytecodes for policy enforcement
- **Auth**: Same Cognito JWT as legacy — `auth.tarobase.com` (prod) or `auth-staging.tarobase.com` (staging)

### CLI realtime chain (`--chain realtime`)
- Maps to `ChainRealtimeOffchain` in `internal/tarobase/client.go`
- Routes API calls to the CF worker URL instead of `api.tarobase.com`
- For staging (`POOF_ENV=staging`), uses `auth-staging.tarobase.com` for session creation
- Writes are direct — no offchainTransaction/sign/submit dance. The DO's `PUT /items` applies writes immediately

### Testing with the CLI
```bash
# Auth against staging
POOF_ENV=staging SOLANA_PRIVATE_KEY="$KEY" ./bin/poof auth login

# Write data via realtime DO
POOF_ENV=staging SOLANA_PRIVATE_KEY="$KEY" ./bin/poof data set \
  --app-id "<appid>" --chain realtime \
  --path notes/doc1 --data '{"title":"test","body":"hello"}'

# Read data back
POOF_ENV=staging SOLANA_PRIVATE_KEY="$KEY" ./bin/poof data get \
  --app-id "<appid>" --chain realtime --path notes/doc1
```

### Creating test apps on staging
1. Login to staging: `POOF_ENV=staging ./bin/poof auth login`
2. Create app via developer API (use the JWT from `~/.poof/tokens.json`):
   ```bash
   TOKEN=$(python3 -c "import json; print(json.load(open('$HOME/.poof/tokens.json'))['id_token'])")
   curl -s -X POST "https://developer-api-staging.tarobase.com/" \
     -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"action":"createApp","appName":"test","appAuth":"onboard","appProtocol":"realtime_offchain"}'
   ```
3. Register in KV: `wrangler kv key put --namespace-id <id> "<appId>" '{"protocol":"realtime_offchain"}' --remote`
4. Deploy policy via `updateApp` action (policy must be a JSON **string**, not object):
   ```bash
   curl -X POST "https://developer-api-staging.tarobase.com/" \
     -H "Authorization: Bearer $TOKEN" -H "X-User-Address: $WALLET" \
     -d '{"action":"updateApp","appId":"<id>","appModifications":{"policy":"{...json string...}","plugins":{}}}'
   ```
5. If bytecodes don't compile on staging, push config directly to DO:
   ```bash
   curl -X POST -H "X-Internal-Secret: <secret>" -H "X-Api-Key: <appId>" \
     "<worker-url>/internal/config-update" -d '{"config":{...}}'
   ```

### Staging infrastructure
- **CF Worker**: `tarobase-realtime-staging.buildwithtarobase.workers.dev`
- **KV namespace**: `2b5025564e1b4c5a8f865c2759d51be9`
- **Developer API**: `developer-api-staging.tarobase.com` (Fly app: `tarobase-developer-api-staging`)
- **Auth**: `auth-staging.tarobase.com`
- **INTERNAL_SECRET**: Must match `POOF_INTERNAL_API_KEY` on the developer API staging
- **Vercel staging** (`v2-staging.poof.new`) has auth protection — needs `VERCEL_BYPASS_TOKEN`
- **Fly logs**: `fly logs -a tarobase-developer-api-staging --no-tail`

### Known staging issues
- `validateAndPreparePolicy` expects policy as a JSON string, not an object
- Bytecode compilation on staging dev API may fail silently — push config directly to DO as workaround
- The staging Poof frontend (`v2-staging.poof.new`) requires Vercel bypass token

## Code conventions

- **Go version**: See `go.mod` (currently 1.26.1)
- **CLI framework**: Cobra. Each command group is a file in `internal/cli/`
- **Linter config**: `.golangci.yml` — errcheck, govet, staticcheck, gocritic, misspell, gofmt, and more
- **Formatting**: `gofmt` only (no goimports)
- **Error handling**: Wrap errors with `fmt.Errorf("context: %w", err)`. Use `api.IsAPIError()` + `handleError()` for API errors in CLI commands
- **Auth pattern**: Call `requireAuth()` at the start of commands that need the API. This initializes `authMgr` and `apiClient` package-level vars
- **Output pattern**: Use `output.Success()`, `output.Error()`, `output.Info()`, `output.Print()`. Never write directly to stdout in CLI commands
- **Test pattern**: Unit tests with mock providers (see `mockAuthProvider` in `api/client_test.go`). Use `httptest.NewServer` for API tests
- **No CGO**: All builds use `CGO_ENABLED=0`

## Adding a new command

1. Create `internal/cli/<command>.go`
2. Define a `*cobra.Command` var and register it in `root.go`'s `init()` with `rootCmd.AddCommand()`
3. Call `requireAuth()` in `RunE` if the command needs the API
4. Use `getProjectID()` for commands that operate on a project
5. Use `output.Print()` for output formatting support
6. **Update `README.md`** — add the command to the command reference section with usage and examples

## Adding a new API endpoint

1. Add the method to `internal/api/endpoints.go` on the `*Client` struct
2. Use `c.Do()` for standard requests, `c.DoRaw()` for non-2xx protocol responses (e.g. 402)
3. Define request/response structs in the same file

## README sync rule

**When commands change, the README must be updated in the same commit.** This includes:
- Adding a new command or subcommand
- Renaming or removing a command
- Changing a command's flags, arguments, or behavior
- Changing global flags or output formats

The README command reference section is the user-facing documentation. It must always reflect the current CLI surface. Run `./bin/poof <command> --help` to get the exact usage text if needed.

## Things to avoid

- Don't commit `.env` files or private keys
- Don't add dependencies without justification — this is a small, focused CLI
- Don't use `os.Exit()` outside of `main.go` — return errors up the call chain
- Don't write to stdout/stderr directly in CLI commands — use the `output` package
- Don't skip the pre-commit hook with `--no-verify`

## Release process

Releases are automated via GoReleaser + GitHub Actions:
1. Tag: `git tag vX.Y.Z && git push origin vX.Y.Z`
2. CI builds binaries for macOS/Linux/Windows, creates GitHub release, updates Homebrew tap

## Environment config

Three environments: `production` (default), `staging`, `local`.
Set via `--env` flag, `POOF_ENV` env var, or `~/.poof/config.yaml`.
