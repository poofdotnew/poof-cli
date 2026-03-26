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
  version/          Version info injected via ldflags
  x402/             Solana USDC payment flow
```

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
