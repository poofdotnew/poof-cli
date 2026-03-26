# poof

A command-line tool for building, deploying, and managing Solana dApps on [poof.new](https://poof.new).

Single binary. No Node.js. No browser. All 30 Poof platform operations from your terminal.

## Install

**Go:**

```bash
go install github.com/poofdotnew/poof-cli/cmd/poof@latest
```

**From source:**

```bash
git clone https://github.com/poofdotnew/poof-cli.git
cd poof-cli
make install
```

**Download binary:**

Grab the latest release from [GitHub Releases](https://github.com/poofdotnew/poof-cli/releases) for your platform.

## Quick Start

```bash
# 1. Generate a Solana keypair
poof keygen
# Output:
#   SOLANA_PRIVATE_KEY=...
#   SOLANA_WALLET_ADDRESS=...

# 2. Save to .env
poof keygen >> .env

# 3. Authenticate
poof auth login

# 4. Build a dApp
poof build -m "Build a token-gated voting app with Solana wallet auth"

# 5. Iterate on it
poof iterate -p <project-id> -m "Add a leaderboard page"

# 6. Deploy to mainnet preview
poof ship -p <project-id>
```

## Configuration

The CLI reads configuration from (highest priority first):

1. **CLI flags** (`--project`, `--env`, `--json`)
2. **Environment variables** (`SOLANA_PRIVATE_KEY`, `POOF_ENV`, etc.)
3. **`.env` file** in the current directory
4. **`~/.poof/config.yaml`** for persistent settings

### Required Environment Variables

| Variable | Description |
|---|---|
| `SOLANA_PRIVATE_KEY` | Base58-encoded Solana private key |
| `SOLANA_WALLET_ADDRESS` | Solana wallet public address |

### Optional Environment Variables

| Variable | Default | Description |
|---|---|---|
| `POOF_ENV` | `production` | Environment: `production`, `staging`, or `local` |
| `VERCEL_BYPASS_TOKEN` | | Vercel protection bypass for staging |

### Persistent Config

```bash
poof config set default_project_id <your-project-id>
poof config set environment staging
poof config show
```

Config is stored at `~/.poof/config.yaml`.

## Commands

### Composite Commands (Workflows)

These chain multiple operations together with polling and progress display.

#### `poof build` — Create and build a project

```bash
poof build -m "Build a todo app with Solana wallet auth"
poof build -m "NFT marketplace" --mode policy --public=false
```

Creates a project, waits for the AI to finish building, and prints the draft URL. This replaces the typical `create_project` + polling loop + `get_project_status` script.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `-m, --message` | (required) | What to build |
| `--mode` | `full` | Generation mode: `full`, `policy`, `ui,policy`, `backend,policy` |
| `--public` | `true` | Make project publicly visible |

#### `poof iterate` — Chat and check results

```bash
poof iterate -p <id> -m "Add dark mode support"
poof iterate -p <id> -m "Fix the login button styling"
```

Sends a chat message, waits for the AI to finish, and shows test results if any exist.

#### `poof ship` — Scan, check, and deploy

```bash
poof ship -p <id>                    # deploy to preview (default)
poof ship -p <id> -t production      # deploy to production
poof ship -p <id> -t mobile          # publish mobile app
```

Runs a security scan, checks publish eligibility, and deploys.

### Project Management

```bash
poof project list                              # list all projects
poof project create -m "Build a ..."           # create a project
poof project status -p <id>                    # get status, URLs, deploy info
poof project messages -p <id>                  # view conversation history
poof project update -p <id> --title "My App"   # update metadata
poof project delete -p <id>                    # delete (irreversible)
```

### AI Chat

```bash
poof chat send -p <id> -m "Add a settings page"   # send a message
poof chat active -p <id>                           # check if AI is processing
poof chat steer -p <id> -m "Focus on backend"      # redirect mid-build
poof chat cancel -p <id>                           # cancel current build
```

### Files

```bash
poof files get -p <id>                                    # list all files (paid)
poof files update -p <id> --file src/config.ts --content "export const X = 1;"
poof files update -p <id> --from-json files.json          # bulk update from JSON
```

### Deployment

```bash
poof deploy check -p <id>              # check publish eligibility
poof deploy preview -p <id>            # deploy to mainnet preview
poof deploy production -p <id>         # deploy to production
poof deploy mobile -p <id>            # publish mobile app
poof deploy download -p <id>           # start code export
poof deploy download-url -p <id> --task <taskId>   # get download link
```

### Tasks and Testing

```bash
poof task list -p <id>                 # list builds, deployments, downloads
poof task get <taskId> -p <id>         # get task details
poof task test-results -p <id>         # view structured test results
```

### Credits

```bash
poof credits balance                   # check credit balance
poof credits topup --quantity 5        # buy credits via x402 USDC
```

### Security

```bash
poof security scan -p <id>            # run security audit
```

### Secrets

```bash
poof secrets get -p <id>                                       # list required/optional secrets
poof secrets set -p <id> API_KEY=sk-123 DB_URL=postgres://...  # set secret values
poof secrets set -p <id> --environment preview API_KEY=sk-456  # per-environment
```

### Custom Domains

```bash
poof domain list -p <id>                        # list domains (paid)
poof domain add myapp.com -p <id>               # add domain (paid)
poof domain add myapp.com -p <id> --default     # set as default
```

### Logs

```bash
poof logs -p <id>                              # get runtime logs
poof logs -p <id> --environment preview        # filter by environment
poof logs -p <id> --limit 50                   # limit entries
```

### Templates

```bash
poof template list                             # browse all templates
poof template list --category defi             # filter by category
poof template list --search "nft"              # search
```

### AI Preferences

```bash
poof preferences get                                       # view model tiers
poof preferences set mainChat=genius codingAgent=smart     # update tiers (paid)
```

### Authentication

```bash
poof auth login            # authenticate and cache token
poof auth status           # check token validity
poof auth logout           # clear cached credentials
```

### Utilities

```bash
poof keygen                # generate a new Solana keypair
poof config show           # show current configuration
poof config set <key> <val>  # set a config value
poof version               # show version, commit, build date
```

## Global Flags

These flags work with every command:

| Flag | Description |
|---|---|
| `-p, --project <id>` | Project ID (or set `default_project_id` in config) |
| `--env <name>` | Environment: `production`, `staging`, `local` |
| `--json` | Output as JSON (for scripting) |
| `--quiet` | Minimal output (IDs and URLs only) |

## Output Formats

```bash
# Human-readable (default)
poof project list

# JSON (for scripting and piping)
poof project list --json
poof project list --json | jq '.projects[].id'

# Quiet (just essential values)
poof project list --quiet
```

## Environments

| Environment | Base URL | Use case |
|---|---|---|
| `production` | `https://poof.new` | Live apps (default) |
| `staging` | `https://staging.poof.new` | Testing |
| `local` | `http://localhost:3000` | Local development |

Switch environments:

```bash
poof --env staging project list
POOF_ENV=staging poof project list
poof config set environment staging
```

## Generation Modes

Control what Poof generates when creating a project:

| Mode | What's generated | Use case |
|---|---|---|
| `full` | UI + backend + policies + deployment | Turnkey Poof-hosted app |
| `policy` | Database policies + typed SDK only | You build your own frontend and backend |
| `ui,policy` | Frontend + policies | You build your own backend |
| `backend,policy` | Backend API + policies | You build your own frontend |

```bash
poof build -m "Token trading platform" --mode full
poof build -m "Manage my data" --mode policy
```

## Scripting Examples

**Build and get the draft URL:**

```bash
URL=$(poof build -m "Build a voting app" --json | jq -r '.urls.draft')
echo "Draft: $URL"
```

**Check if AI is done:**

```bash
if poof chat active -p $ID --json | jq -e '.active' > /dev/null; then
  echo "Still building..."
else
  echo "Done!"
fi
```

**Full CI/CD pipeline:**

```bash
#!/bin/bash
set -e

# Build
PROJECT=$(poof build -m "Build a staking dashboard" --json | jq -r '.projectId')

# Test
poof iterate -p $PROJECT -m "Generate and run lifecycle tests for all policies"

# Check results
FAILED=$(poof task test-results -p $PROJECT --json | jq '.summary.failed')
if [ "$FAILED" -gt 0 ]; then
  echo "Tests failed!"
  exit 1
fi

# Ship
poof ship -p $PROJECT -t preview
```

## Credits

- Free tier: ~10 daily credits (resets daily)
- `poof credits balance` is free and shows your balance
- Deployment and some features require at least one credit purchase
- `poof credits topup` initiates the x402 USDC payment flow

## Development

```bash
git clone https://github.com/poofdotnew/poof-cli.git
cd poof-cli

make build          # build binary to bin/poof
make test           # run all tests
make lint           # fmt + vet
make all            # lint + test + build
make release        # cross-compile for all platforms
make install        # install to $GOPATH/bin
```

### Releasing

Releases are automated with [GoReleaser](https://goreleaser.com/):

```bash
git tag v0.1.0
git push --tags
goreleaser release
```

This builds binaries for macOS (arm64/amd64), Linux (arm64/amd64), and Windows (amd64), creates archives, checksums, and a GitHub release.

## License

MIT
