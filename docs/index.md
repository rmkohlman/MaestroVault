# MaestroVault

**A macOS-first, lightweight secrets manager for developers.**

MaestroVault encrypts your secrets locally with AES-256-GCM envelope encryption, stores the master key in the macOS Keychain, and optionally gates access behind TouchID. No cloud. No subscriptions. Just your secrets, on your machine.

---

## Features

- **AES-256-GCM envelope encryption** -- each secret gets its own data key
- **macOS Keychain** -- master key never touches disk
- **TouchID** -- optional biometric gate when opening the vault
- **Environment-scoped secrets** -- same secret name across dev, staging, prod
- **CLI** -- 20 commands with shell completions and smart output formatting
- **TUI** -- full-screen interactive interface with search, sort, copy, edit, and vim motions
- **REST API** -- Unix socket server with scoped Bearer token auth
- **Go client library** -- programmatic access to the API from any Go project
- **Homebrew** -- `brew install rmkohlman/tap/maestrovault`

## Quick start

```bash
# Install
brew install rmkohlman/tap/maestrovault

# Create a vault
mav init

# Store a secret
mav set db-password --value "s3cret" --env prod --metadata service=postgres

# Retrieve it
mav get db-password --env prod

# Launch the TUI
mav tui
```

## How it works

```
                  macOS Keychain
                       |
                  Master Key (AES-256)
                       |
             +---------+---------+
             |                   |
        Data Key A          Data Key B      (per-secret, random)
             |                   |
        Secret A            Secret B        (AES-256-GCM encrypted)
             |                   |
          SQLite DB (~/.maestrovault/vault.db)
```

Each secret is encrypted with its own randomly generated data key. The data keys are themselves encrypted with the master key (envelope encryption). The master key lives exclusively in the macOS Keychain and is never written to disk.

## Project layout

```
mav
  init          Create a new vault
  set           Store a secret
  get           Retrieve a secret
  list          List all secrets
  delete        Delete a secret
  edit          Edit a secret
  copy          Copy to clipboard
  search        Search by name/metadata
  generate      Generate a password
  env           Export as env vars
  exec          Run command with injected env
  export        Export vault to file
  import        Import from file
  destroy       Destroy the vault
  tui           Interactive terminal UI
  serve         Start REST API server
  token         Manage API tokens
  touchid       Manage TouchID auth
  version       Print version info
```
