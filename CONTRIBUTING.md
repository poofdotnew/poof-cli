# Contributing to poof-cli

Thanks for your interest in contributing to the Poof CLI!

## Prerequisites

- **Go** (version in `go.mod`, currently 1.26.1+)
- **golangci-lint** (optional but recommended): `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

## Getting started

```bash
# Clone the repo
git clone https://github.com/poofdotnew/poof-cli.git
cd poof-cli

# Install the pre-commit hook
make install-hooks

# Build and test
make all
```

## Development workflow

1. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```

2. **Make your changes.** Follow existing patterns — look at neighboring files.

3. **Run checks locally:**
   ```bash
   make fmt       # Format code
   make vet       # Static analysis
   make lint      # Linter suite
   make test      # Unit tests with race detector
   make build     # Verify it compiles
   ```
   Or run everything at once: `make all`

4. **Commit.** The pre-commit hook will run fmt, vet, lint, build, and test automatically. All must pass.

5. **Open a PR** against `main`.

## Project structure

```
cmd/poof/           Entry point
internal/
  cli/              Command definitions (one file per command group)
  api/              REST API client
  auth/             Solana auth and session management
  config/           Configuration loading
  output/           Text/JSON/Quiet output formatting
  poll/             Async task polling
  version/          Version info (injected at build time)
  x402/             Solana USDC payment flow
```

## Adding a new command

1. Create `internal/cli/<command>.go` with a `cobra.Command`
2. Register it in `internal/cli/root.go` `init()` with `rootCmd.AddCommand()`
3. Use `requireAuth()` if the command needs the API
4. Use `getProjectID()` for project-scoped commands
5. Use `output.Print()` / `output.Success()` for output — never write to stdout directly
6. Add tests in `internal/cli/<command>_test.go`

## Adding a new API endpoint

1. Add a method on `*Client` in `internal/api/endpoints.go`
2. Use `c.Do()` for standard requests, `c.DoRaw()` for non-2xx protocol responses
3. Define request/response structs in the same file
4. Add tests using `httptest.NewServer` (see `internal/api/client_test.go` for patterns)

## Code style

- **Formatting**: `gofmt` (enforced by linter and pre-commit hook)
- **Linting**: Config in `.golangci.yml` — includes errcheck, govet, staticcheck, gocritic, misspell
- **Errors**: Always wrap with context: `fmt.Errorf("what failed: %w", err)`
- **Output**: Use the `output` package, never raw `fmt.Println` in CLI commands
- **Tests**: Unit tests with mocks. Use `httptest.NewServer` for API tests. Run with `-race`

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: Add domain verification command
fix: Handle expired token in deploy flow
chore: Update dependencies
docs: Add contributing guide
```

## CI

Every PR runs:
- **Lint** — golangci-lint with the project's `.golangci.yml`
- **Test** — `go test -race -coverprofile=coverage.out -count=1 ./...`
- **Build** — verifies the binary compiles and runs `poof version`

## Releases

Releases are handled by maintainers. When a `v*` tag is pushed, GoReleaser builds binaries for all platforms and updates the Homebrew tap automatically.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
