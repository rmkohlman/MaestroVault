# Getting Started

## Requirements

- macOS (arm64 or amd64)
- Go 1.25+ (only if building from source)

## Installation

### Homebrew (recommended)

```bash
brew install rmkohlman/tap/maestrovault
```

### From source

```bash
git clone https://github.com/rmkohlman/MaestroVault.git
cd MaestroVault
go build -o mav ./cmd/mav
```

### From GitHub Releases

Download the latest binary from [GitHub Releases](https://github.com/rmkohlman/MaestroVault/releases) and place it in your `$PATH`.

## Initialize a vault

```bash
mav init
```

This does three things:

1. Creates `~/.maestrovault/` directory
2. Generates a 256-bit master key
3. Stores the master key in the macOS Keychain

!!! note
    You only need to run `init` once. The vault persists across terminal sessions.

## Store your first secret

```bash
mav set api-key --value "sk-abc123xyz"
```

You can also add metadata for organization and specify an environment:

```bash
mav set db-password \
  --value "p@ssw0rd" \
  --env production \
  --metadata service=postgres \
  --metadata team=backend
```

If you omit `--value`, MaestroVault reads from stdin (useful for piping):

```bash
echo "my-secret-value" | mav set pipeline-token
```

To store a file byte-for-byte (PEM certificates, SSH keys, etc.):

```bash
mav set tls-cert --from-file cert.pem --env prod
```

## Retrieve a secret

```bash
mav get api-key
```

Output format auto-detects: table for terminals, JSON when piped:

```bash
# Human-readable
mav get api-key

# JSON (piped or explicit)
mav get api-key -o json
```

## List secrets

```bash
mav list
```

Filter by metadata:

```bash
mav list --metadata-key service --metadata-value postgres
```

Filter by environment:

```bash
mav list --env production
```

## Search

```bash
mav search postgres
```

Searches secret names, environments, and metadata in real time.

## Copy to clipboard

```bash
mav copy db-password
```

The clipboard is automatically cleared after 45 seconds. Override with `--clear`:

```bash
mav copy db-password --clear 10s
```

## Generate a password

```bash
# Generate and print
mav generate

# Generate and store
mav generate --name wifi-password --length 24

# Customize character sets
mav generate --no-symbols --length 16
```

## Use secrets as environment variables

```bash
# Print export statements
mav env

# Run a command with secrets injected
mav exec -- env | grep MY_SECRET
```

## Export and import

```bash
# Export to JSON
mav export > backup.json

# Export to .env format
mav export --format env > .env

# Import from JSON
mav import backup.json

# Import from .env
mav import --format env .env
```

!!! warning
    Export files contain plaintext secrets. Handle with care and delete after use.

## Enable TouchID

```bash
mav touchid enable
```

Once enabled, every command that accesses the vault will prompt for biometric authentication.

```bash
mav touchid status   # Check current state
mav touchid disable  # Turn it off (requires TouchID to disable)
```

## Launch the TUI

```bash
mav tui
```

For vim keybindings:

```bash
mav tui --vim
```

## Configuration

MaestroVault stores settings in `~/.maestrovault/config.json`:

```json
{
  "touchid": false,
  "vim_mode": false,
  "fuzzy_search": false
}
```

| Field | Description |
|-------|-------------|
| `touchid` | Enable TouchID biometric gate on vault open |
| `vim_mode` | Enable vim keybinding modes in the TUI |
| `fuzzy_search` | Enable fuzzy matching in TUI search |

All settings can also be toggled from the TUI settings overlay (`S`).

## What's next

- [CLI Reference](cli.md) -- all commands and flags
- [TUI Guide](tui.md) -- keyboard shortcuts and features
- [REST API](api.md) -- run the API server
- [Security](security.md) -- encryption details and threat model
