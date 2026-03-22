# ccodex

Terminal tool for managing multiple Codex `auth.json` profiles.

## Install

Recommended install:

```bash
curl -fsSL https://raw.githubusercontent.com/axinhouzilaoyue/codexSwitch/main/scripts/install-release.sh | bash
```

Environment notes:

- Supported terminals: macOS terminal, Linux shell, and Windows WSL terminal
- Native Windows PowerShell / cmd is not supported; use WSL on Windows
- `codex` CLI must already be installed on the target machine
- Install Codex with `npm install -g @openai/codex` or `brew install --cask codex`
- Go is not required for the release install above
- Go is only needed if you build from source

Local install:

```bash
git clone https://github.com/axinhouzilaoyue/codexSwitch.git
cd codexSwitch
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

Update:

```bash
ccodex update
```

Uninstall:

```bash
ccodex uninstall
```

`ccodex uninstall` removes the installed binary, legacy command names, and the default data directory `~/.codex-switch`. If you used a custom `--store-dir`, remove that directory manually.

## Release

GitHub release install is based on:

1. Build release archives for all supported platforms:

```bash
make dist-all
```

This creates:

```bash
dist/ccodex-<os>-<arch>.tar.gz
dist/ccodex-<os>-<arch>.tar.gz.sha256
``` 

2. Push a tag such as `v0.2.7`

3. GitHub Actions in `.github/workflows/release.yml` will build and publish the release assets automatically

4. Others install with:

```bash
curl -fsSL https://raw.githubusercontent.com/axinhouzilaoyue/codexSwitch/main/scripts/install-release.sh | bash
```

The installer auto-detects macOS/Linux and arm64/amd64, uses the Linux package inside WSL, installs `ccodex`, and warns if `codex` CLI is missing.

If you do not want to use GitHub Releases, you can still host archives yourself:

```bash
curl -fsSL https://your-host.example/install-release.sh | bash -s -- --base-url https://your-host.example/releases
```

There is also a ready GitHub Actions workflow at `.github/workflows/release.yml`. If you push a tag like `v0.2.7`, it will build all release archives and attach them to the GitHub Release automatically.

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
ccodex update
ccodex uninstall
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
- Each saved profile directory only keeps `auth.json` and `meta.json`
- Older runtime artifacts such as `skills/`, `memories/`, sqlite files, `tmp/`, and root-level `backups/` are cleaned up automatically
- The app uses `codex app-server -c 'cli_auth_credentials_store="file"'` with temporary runtime homes, so saved profiles stay clean
- Login uses the official Codex browser login flow, not a reimplemented OAuth flow
- Refresh and quota reads happen against temporary isolated `CODEX_HOME` directories, so they do not overwrite your current active Codex account
