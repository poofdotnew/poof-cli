# AGENTS.md

Instructions for all AI coding agents working on this repository.
For detailed project context, see [CLAUDE.md](CLAUDE.md).

## Ground rules

1. **Read before writing.** Understand existing code and patterns before making changes.
2. **Run `make test` after every change.** Tests must pass. If they don't, fix them before moving on.
3. **Run `make lint` before committing.** The CI and pre-commit hook enforce this.
4. **Follow existing patterns.** Look at neighboring files for conventions — don't invent new ones.
5. **Keep changes minimal.** Don't refactor surrounding code, add comments to unchanged code, or "improve" things you weren't asked to change.
6. **Never commit secrets.** No private keys, `.env` files, or credentials.

## Quick reference

| Task | Command |
|------|---------|
| Build | `make build` |
| Test | `make test` |
| Lint | `make lint` |
| Format | `make fmt` |
| All checks | `make all` |

## Architecture at a glance

```
cmd/poof/main.go  ->  internal/cli/  ->  internal/api/  ->  poof.new REST API
                                      ->  internal/auth/ ->  Solana signing + token cache
                                      ->  internal/output/ -> text/json/quiet formatting
```

- **CLI layer** (`internal/cli/`): Cobra commands. One file per command group. Uses `requireAuth()` for authenticated commands, `getProjectID()` for project-scoped commands.
- **API layer** (`internal/api/`): HTTP client with auto 401 retry. `Do()` for standard calls, `DoRaw()` for raw responses.
- **Auth layer** (`internal/auth/`): Solana keypair signing, session tokens, disk cache at `~/.poof/token_cache`.
- **Config layer** (`internal/config/`): Precedence is flags > env vars > `.env` > `~/.poof/config.yaml`.
- **Output layer** (`internal/output/`): Three modes — text (default, colored), json, quiet. All CLI output goes through this.

## Code style

- Standard `gofmt` formatting
- Errors wrapped with context: `fmt.Errorf("what failed: %w", err)`
- No direct stdout/stderr writes in CLI commands — use `output.*` functions
- Tests use `mockAuthProvider` and `httptest.NewServer` — see `internal/api/client_test.go` for the pattern
- No CGO (`CGO_ENABLED=0`)

## What not to do

- Don't add dependencies without a strong reason
- Don't use `os.Exit()` outside `main.go`
- Don't skip the pre-commit hook
- Don't modify CI workflows without understanding the release pipeline (GoReleaser + Homebrew tap)
- Don't change the config loading order — it's intentional
