# CodexSwitch

Go terminal dashboard for managing multiple Codex `auth.json` profiles.

## Install

Prerequisite:

- `codex` CLI must already be installed on the target machine
- Install Codex with `npm install -g @openai/codex` or `brew install --cask codex`
- Go is only needed if you build from source; release install does not require Go

Local install:

```bash
cd /Users/liukaixin02/Code/CodexSwitch
./scripts/install.sh
```

Installed commands:

```bash
ccodex
```

You can also build locally:

```bash
make build
.build/ccodex
```

Upgrade:

```bash
./scripts/upgrade.sh
```

Uninstall:

```bash
./scripts/uninstall.sh
```

## Share It

The easiest sharing path is:

1. Upload this repo to GitHub.

2. Build release archives for all supported platforms:

```bash
make dist-all
```

This creates:

```bash
dist/ccodex-<os>-<arch>.tar.gz
dist/ccodex-<os>-<arch>.tar.gz.sha256
```

3. Create a GitHub Release and upload all files in `dist/`.

4. Others can install with one command:

```bash
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/scripts/install-release.sh | bash -s -- --repo <owner>/<repo>
```

The installer auto-detects macOS/Linux and arm64/amd64, downloads the latest GitHub Release asset, installs `ccodex`, and warns if `codex` CLI is missing.

If you do not want to use GitHub Releases, you can still host archives yourself:

```bash
curl -fsSL https://your-host.example/install-release.sh | bash -s -- --base-url https://your-host.example/releases
```

There is also a ready GitHub Actions workflow at `.github/workflows/release.yml`. If you push a tag like `v0.2.0`, it will build all release archives and attach them to the GitHub Release automatically.

## What It Does

- Detects the real active `CODEX_HOME` via `codex app-server` and `config/read`
- Logs in a new ChatGPT-backed Codex account into an isolated profile store
- Refreshes saved profiles with Codex's own managed refresh flow
- Reads structured Codex rate limits with `account/rateLimits/read`
- Switches the active account by atomically replacing `<CODEX_HOME>/auth.json`
- Deletes a saved profile
- Shows the currently active account

## Run

```bash
ccodex
```

The default mode is an interactive terminal TUI.

Optional flags:

```bash
ccodex --codex-home /path/to/custom/.codex
ccodex --store-dir /path/to/profile-store
ccodex --version
```

Non-interactive commands:

```bash
ccodex tui
ccodex list
ccodex current
ccodex doctor
ccodex version
```

## TUI Commands

- `n`: login a new account
- `r`: refresh the selected profile
- `R`: refresh all saved profiles
- `s`: switch the selected profile into the target `CODEX_HOME`
- `d`: delete the selected saved profile
- `h`: set or clear the target `CODEX_HOME`
- `Enter`: view the selected profile details
- `j` / `k` or arrow keys: move selection
- `?`: help
- `q`: quit

## Notes

- Saved profiles live in `~/.codex-switch/profiles/<profile_id>/`
- The app uses `codex app-server -c 'cli_auth_credentials_store="file"'` so isolated profiles persist to `auth.json`
- Login uses the official Codex browser login flow, not a reimplemented OAuth flow
- Refresh and quota reads happen against each saved profile's isolated `CODEX_HOME`, so they do not overwrite your current active Codex account
