# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial CLI with 19 commands: auth, build, chat, config, credits, deploy, domain, files, iterate, keygen, logs, preferences, project, secrets, security, ship, task, template, version
- Solana keypair authentication with token caching and auto-refresh
- x402 USDC credit topup via Solana
- Composite commands: `build` (create + poll + status), `iterate` (chat + poll + test results), `ship` (scan + eligibility + deploy)
- JSON, text, and quiet output formats
- Multi-environment support: production, staging, local
- GoReleaser configuration for cross-platform releases
- GitHub Actions CI/CD (lint, test, build, release)
- Homebrew tap distribution
