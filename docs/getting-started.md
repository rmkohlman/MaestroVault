# Getting Started

## Requirements

- macOS (arm64 or amd64)
- Go 1.21+ (only if building from source)

## Installation

### Homebrew (recommended)

```bash
brew install rmkohlman/tap/maestrovault
```

### From source

```bash
git clone https://github.com/rmkohlman/MaestroVault.git
cd MaestroVault
go build -o maestrovault ./cmd/maestro
```

### From GitHub Releases

Download the latest binary from [GitHub Releases](https://github.com/rmkohlman/MaestroVault/releases) and place it in your `$PATH`.

## Initialize a vault

```bash
maestrovault init
```

This does three things:

1. Creates `~/.maestrovault/` directory
2. Generates a 256-bit master key
3. Stores the master key in the macOS Keychain

!!! note
    You only need to run `init` once. The vault persists across terminal sessions.

## Store your first secret

```bash
maestrovault set api-key --value "sk-abc123xyz"
```

You can also add metadata for organization and specify an environment:

```bash
maestrovault set db-password \
  --value "p@ssw0rd" \
  --env production \
  --metadata service=postgres \
  --metadata team=backend
```

If you omit `--value`, MaestroVault reads from stdin (useful for piping):

```bash
echo "my-secret-value" | maestrovault set pipeline-token
```

## Retrieve a secret

```bash
maestrovault get api-key
```

Output format auto-detects: table for terminals, JSON when piped:

```bash
# Human-readable
maestrovault get api-key

# JSON (piped or explicit)
maestrovault get api-key -o json
```

## List secrets

```bash
maestrovault list
```

Filter by metadata:

```bash
maestrovault list --metadata-key service --metadata-value postgres
```

Filter by environment:

```bash
maestrovault list --env production
```

## Search

```bash
maestrovault search postgres
```

Searches secret names, environments, and metadata in real time.

## Copy to clipboard

```bash
maestrovault copy db-password
```

The clipboard is automatically cleared after 45 seconds. Override with `--clear`:

```bash
maestrovault copy db-password --clear 10s
```

## Generate a password

```bash
# Generate and print
maestrovault generate

# Generate and store
maestrovault generate --name wifi-password --length 24

# Customize character sets
maestrovault generate --no-symbols --length 16
```

## Use secrets as environment variables

```bash
# Print export statements
maestrovault env

# Run a command with secrets injected
maestrovault exec -- env | grep MY_SECRET
```

## Export and import

```bash
# Export to JSON
maestrovault export > backup.json

# Export to .env format
maestrovault export --format env > .env

# Import from JSON
maestrovault import backup.json

# Import from .env
maestrovault import --format env .env
```

!!! warning
    Export files contain plaintext secrets. Handle with care and delete after use.

## Enable TouchID

```bash
maestrovault touchid enable
```

Once enabled, every command that accesses the vault will prompt for biometric authentication.

```bash
maestrovault touchid status   # Check current state
maestrovault touchid disable  # Turn it off (requires TouchID to disable)
```

## Launch the TUI

```bash
maestrovault tui
```

For vim keybindings:

```bash
maestrovault tui --vim
```

## What's next

- [CLI Reference](cli.md) -- all commands and flags
- [TUI Guide](tui.md) -- keyboard shortcuts and features
- [REST API](api.md) -- run the API server
- [Security](security.md) -- encryption details and threat model
