# CLI Reference

All commands support `--format json` (or `-o json`) for machine-readable output and `--no-color` to disable colored output. When stdout is not a TTY, JSON is used automatically.

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--format` | `-o` | Output format: `json`, `table` (default: auto-detect) |
| `--no-color` | | Disable colored output (also respects `NO_COLOR` env var) |

---

## `maestrovault init`

Initialize a new vault.

```bash
maestrovault init
```

Creates `~/.maestrovault/`, generates a master key, and stores it in the macOS Keychain. Fails if a vault already exists.

---

## `maestrovault set <name>`

Store or update a secret.

```bash
maestrovault set db-password --value "s3cret"
maestrovault set api-key --value "sk-123" --env prod --metadata service=api
echo "piped-value" | maestrovault set token-name
```

| Flag | Description |
|------|-------------|
| `--value`, `-v` | Secret value (reads from stdin if omitted) |
| `--metadata`, `-m` | Metadata as `key=value` (repeatable) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |
| `--generate`, `-g` | Auto-generate a random password as the value |
| `--length` | Generated password length (with `--generate`, default: 32) |
| `--symbols` | Include symbols in generated password (with `--generate`) |

---

## `maestrovault get <name>`

Retrieve and decrypt a secret.

```bash
maestrovault get db-password
maestrovault get db-password --env prod
maestrovault get db-password -o json
```

| Flag | Description |
|------|-------------|
| `--quiet`, `-q` | Print only the value (for piping) |
| `--clip`, `-c` | Copy value to clipboard (auto-clears in 45s) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

Shell completion is available for secret names.

---

## `maestrovault list`

List all secrets (values are not shown).

```bash
maestrovault list
maestrovault list --env prod
maestrovault list --metadata-key service --metadata-value postgres
maestrovault list -o json
```

| Flag | Description |
|------|-------------|
| `--filter`, `-f` | Filter secrets by name or metadata content |
| `--metadata-key` | Filter by metadata key |
| `--metadata-value` | Filter by metadata value (used with `--metadata-key`) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `maestrovault delete <name>`

Delete a secret. Requires confirmation unless `--force` is used.

```bash
maestrovault delete old-key
maestrovault delete old-key --force
```

| Flag | Description |
|------|-------------|
| `--force`, `-f` | Skip confirmation prompt |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `maestrovault edit <name>`

Update an existing secret's value or metadata.

```bash
maestrovault edit db-password --value "new-password"
maestrovault edit api-key --metadata env=staging
```

| Flag | Description |
|------|-------------|
| `--value`, `-v` | New value (keeps existing if omitted) |
| `--metadata`, `-m` | New metadata as `key=value` (replaces all metadata; keeps existing if omitted) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `maestrovault copy <name>`

Copy a secret's value to the system clipboard.

```bash
maestrovault copy db-password
maestrovault copy db-password --clear 10s
```

| Flag | Description |
|------|-------------|
| `--clear`, `-c` | Auto-clear clipboard after duration (default: `45s`) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `maestrovault search <query>`

Search secrets by name, environment, and metadata.

```bash
maestrovault search postgres
maestrovault search prod -o json
```

---

## `maestrovault generate`

Generate a random password.

```bash
maestrovault generate
maestrovault generate --name wifi --length 24
maestrovault generate --no-symbols --no-uppercase
```

| Flag | Description |
|------|-------------|
| `--name`, `-n` | Store the generated password as a secret |
| `--length` | Password length (default: 32) |
| `--no-uppercase` | Exclude uppercase letters |
| `--no-lowercase` | Exclude lowercase letters |
| `--no-digits` | Exclude digits |
| `--no-symbols` | Exclude symbols |
| `--metadata`, `-m` | Metadata for stored secret (with `--name`, repeatable) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |
| `--clip`, `-c` | Copy to clipboard |
| `--passphrase` | Generate a passphrase instead of a password |
| `--words` | Number of words in passphrase (with `--passphrase`, default: 5) |
| `--delimiter` | Word delimiter for passphrase (with `--passphrase`, default: `-`) |

---

## `maestrovault env`

Print secrets as shell export statements.

```bash
maestrovault env
eval $(maestrovault env)
```

Secret names are converted to environment variable format: uppercase, dashes/dots/spaces become underscores. For example, `db-password` becomes `DB_PASSWORD`.

---

## `maestrovault exec -- <command>`

Run a command with all secrets injected as environment variables.

```bash
maestrovault exec -- env
maestrovault exec -- node server.js
maestrovault exec -- docker compose up
```

---

## `maestrovault export`

Export all secrets to stdout (plaintext).

```bash
maestrovault export > backup.json
maestrovault export --format env > .env
```

| Flag | Description |
|------|-------------|
| `--format` | Export format: `json` (default), `env` |

!!! warning
    Export files contain plaintext secrets. Delete them after use.

---

## `maestrovault import <file>`

Import secrets from a file.

```bash
maestrovault import backup.json
maestrovault import --format env .env
maestrovault import backup.json --force
```

| Flag | Description |
|------|-------------|
| `--format` | File format: `json` (default), `env` |
| `--force`, `-f` | Skip confirmation prompt |

---

## `maestrovault destroy`

Permanently destroy the vault, database, and master key.

```bash
maestrovault destroy
maestrovault destroy --force
```

| Flag | Description |
|------|-------------|
| `--force`, `-f` | Skip confirmation prompt |

!!! danger
    This is irreversible. All secrets and the master key are permanently deleted.

---

## `maestrovault tui`

Launch the interactive terminal UI.

```bash
maestrovault tui
maestrovault tui --vim
```

| Flag | Description |
|------|-------------|
| `--vim` | Enable vim keybinding modes (Normal/Visual/Insert) |

See the [TUI Guide](tui.md) for keyboard shortcuts.

---

## `maestrovault serve`

Start the REST API server on a Unix domain socket.

```bash
maestrovault serve
maestrovault serve --socket /tmp/maestrovault.sock
```

| Flag | Description |
|------|-------------|
| `--socket` | Custom socket path (default: `~/.maestrovault/maestrovault.sock`) |

See the [REST API reference](api.md) for endpoints.

---

## `maestrovault token`

Manage API tokens for the REST API server.

### `token create`

```bash
maestrovault token create --name "ci-read" --scope read
maestrovault token create --name "deploy" --scope read,write --expires 24h
maestrovault token create --name "admin" --scope admin
```

| Flag | Description |
|------|-------------|
| `--name` | Token name (required) |
| `--scope` | Scopes: `read`, `write`, `generate`, `admin` (required, comma-separated) |
| `--expires` | Expiry duration, e.g. `24h`, `720h` (default: no expiry) |

!!! note
    The plaintext token is shown only once at creation time. Save it somewhere safe.

### `token list`

```bash
maestrovault token list
maestrovault token list -o json
```

### `token revoke`

```bash
maestrovault token revoke <id>
maestrovault token revoke --all
```

| Flag | Description |
|------|-------------|
| `--all` | Revoke all tokens |

---

## `maestrovault touchid`

Manage TouchID biometric authentication.

### `touchid enable`

```bash
maestrovault touchid enable
```

Checks hardware availability, performs a verification prompt, then saves the setting.

### `touchid disable`

```bash
maestrovault touchid disable
```

Requires TouchID authentication to disable (prevents unauthorized disabling).

### `touchid status`

```bash
maestrovault touchid status
maestrovault touchid status -o json
```

---

## `maestrovault version`

Print build version, commit, date, Go version, and OS/arch.

```bash
maestrovault version
maestrovault version -o json
```

---

## Shell Completions

MaestroVault supports shell completions for Bash, Zsh, Fish, and PowerShell via Cobra's built-in completion system:

```bash
# Bash
maestrovault completion bash > /usr/local/etc/bash_completion.d/maestrovault

# Zsh
maestrovault completion zsh > "${fpath[1]}/_maestrovault"

# Fish
maestrovault completion fish > ~/.config/fish/completions/maestrovault.fish
```

Secret names are completed dynamically -- press Tab after `get`, `delete`, `edit`, or `copy` to see available names.
