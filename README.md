# MaestroVault

A macOS-first, developer-focused secrets management tool with CLI, TUI, REST API, and TouchID support.

MaestroVault stores secrets locally using AES-256-GCM envelope encryption with the master key secured in the macOS Keychain. No cloud, no accounts, no network — your secrets stay on your machine.

**[Documentation](https://rmkohlman.github.io/MaestroVault/)**

## Features

- **CLI** — 20 commands with shell completions, JSON/table output, `NO_COLOR` support
- **Environment scoping** — same secret name across dev, staging, prod
- **TUI** — Terminal UI with search, sort, inline editing, password generator, and optional vim motions (`--vim`)
- **REST API** — Unix domain socket server with scoped Bearer token authentication
- **Go Client Library** — Programmatic access via `pkg/client`
- **TouchID** — Biometric authentication gate on vault open (configurable)
- **Encryption** — AES-256-GCM envelope encryption, master key in macOS Keychain
- **SQLite** — Pure Go via `modernc.org/sqlite` (no CGo for database)
- **Homebrew** — `brew install rmkohlman/tap/maestrovault`

## Install

```sh
brew install rmkohlman/tap/maestrovault
```

Or build from source:

```sh
git clone https://github.com/rmkohlman/MaestroVault.git
cd MaestroVault
go build -o mav ./cmd/mav
```

## Quick Start

```sh
# Initialize a new vault (creates ~/.maestrovault/)
mav init

# Store a secret (with optional environment and metadata)
mav set my-api-key --value "sk-abc123" --env prod --metadata service=api

# Retrieve it
mav get my-api-key --env prod

# Copy to clipboard (auto-clears after 45s)
mav copy my-api-key --env prod

# Launch the TUI
mav tui

# Launch with vim motions
mav tui --vim
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize a new vault |
| `set` | Store a secret |
| `get` | Retrieve a secret |
| `list` | List all secrets |
| `delete` | Delete a secret |
| `edit` | Edit a secret interactively |
| `copy` | Copy a secret to clipboard |
| `search` | Search secrets by name |
| `generate` | Generate a password or token |
| `env` | Print secrets as environment variables |
| `exec` | Run a command with secrets injected |
| `export` | Export secrets to a file |
| `import` | Import secrets from a file |
| `destroy` | Destroy the vault |
| `tui` | Launch the terminal UI |
| `serve` | Start the REST API server |
| `token` | Manage API tokens (create/list/revoke) |
| `touchid` | Configure TouchID (enable/disable/status) |
| `version` | Print version info |

## REST API

Start the server over a Unix domain socket:

```sh
mav serve
```

Create a token and make requests:

```sh
# Create a scoped token
mav token create --name ci --scopes read,write

# Use it
curl --unix-socket ~/.maestrovault/maestrovault.sock \
  -H "Authorization: Bearer mvt_..." \
  http://localhost/v1/secrets
```

## Security Model

- **Envelope encryption** — Each secret encrypted with a unique DEK, DEKs encrypted with the master KEK
- **Master key** — Stored in macOS Keychain, never touches disk
- **TouchID** — Optional biometric gate before vault access
- **API tokens** — HMAC-SHA256 hashed at rest, scoped (`read`, `write`, `generate`, `admin`)
- **File permissions** — Vault directory locked to `0700`, database to `0600`

## Documentation

Full documentation is available at **https://rmkohlman.github.io/MaestroVault/**

- [Getting Started](https://rmkohlman.github.io/MaestroVault/getting-started/)
- [CLI Reference](https://rmkohlman.github.io/MaestroVault/cli/)
- [TUI Guide](https://rmkohlman.github.io/MaestroVault/tui/)
- [REST API Reference](https://rmkohlman.github.io/MaestroVault/api/)
- [Go Client Library](https://rmkohlman.github.io/MaestroVault/client/)
- [Security Architecture](https://rmkohlman.github.io/MaestroVault/security/)
- [TouchID](https://rmkohlman.github.io/MaestroVault/touchid/)

## License

Apache 2.0 — see [LICENSE](LICENSE).
