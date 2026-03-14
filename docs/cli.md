# CLI Reference

All commands support `--format json` (or `-o json`) for machine-readable output and `--no-color` to disable colored output. When stdout is not a TTY, JSON is used automatically.

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--format` | `-o` | Output format: `json`, `table` (default: auto-detect) |
| `--no-color` | | Disable colored output (also respects `NO_COLOR` env var) |

---

## `mav init`

Initialize a new vault.

```bash
mav init
```

Creates `~/.maestrovault/`, generates a master key, and stores it in the macOS Keychain. Fails if a vault already exists.

---

## `mav set <name>`

Store or update a secret.

```bash
mav set db-password --value "s3cret"
mav set api-key --value "sk-123" --env prod --metadata service=api
echo "piped-value" | mav set token-name
```

On a TTY with no `--value` or `--generate` flag, `mav set` opens an interactive modal for entering the secret name, value, environment, and metadata.

| Flag | Description |
|------|-------------|
| `--value`, `-v` | Secret value (reads from stdin if omitted) |
| `--metadata`, `-m` | Metadata as `key=value` (repeatable) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |
| `--generate`, `-g` | Auto-generate a random password as the value |
| `--length` | Generated password length (with `--generate`, default: 32) |
| `--symbols` | Include symbols in generated password (with `--generate`) |

---

## `mav get <name>`

Retrieve and decrypt a secret.

```bash
mav get db-password
mav get db-password --env prod
mav get db-password -o json
```

On a TTY (without `-q`, `-c`, or `--print`), `mav get` opens an interactive modal showing the secret with the value masked. From the modal you can peek, copy, edit, or quit.

| Flag | Description |
|------|-------------|
| `--quiet`, `-q` | Print only the value (for piping) |
| `--clip`, `-c` | Copy value to clipboard (auto-clears in 45s) |
| `--print`, `-p` | Print the secret in plaintext (legacy behavior) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

When stdout is not a TTY (piped), the raw value is printed automatically.

Shell completion is available for secret names.

---

## `mav list`

List all secrets (values are not shown).

**Alias:** `mav ls`

```bash
mav list
mav list --env prod
mav list --metadata-key service --metadata-value postgres
mav list -o json
```

| Flag | Description |
|------|-------------|
| `--filter`, `-f` | Filter secrets by name or metadata content |
| `--metadata-key` | Filter by metadata key |
| `--metadata-value` | Filter by metadata value (used with `--metadata-key`) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `mav delete <name>`

Delete a secret. Requires confirmation unless `--force` is used.

**Alias:** `mav rm`

```bash
mav delete old-key
mav delete old-key --force
```

| Flag | Description |
|------|-------------|
| `--force`, `-f` | Skip confirmation prompt |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `mav edit <name>`

Update an existing secret's value or metadata.

```bash
mav edit db-password --value "new-password"
mav edit api-key --metadata env=staging
```

On a TTY with no flags, `mav edit` opens an interactive modal pre-populated with the secret's current values for inline editing.

| Flag | Description |
|------|-------------|
| `--value`, `-v` | New value (keeps existing if omitted) |
| `--metadata`, `-m` | New metadata as `key=value` (replaces all metadata; keeps existing if omitted) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `mav copy <name>`

Copy a secret's value to the system clipboard.

**Alias:** `mav cp`

```bash
mav copy db-password
mav copy db-password --clear 10s
```

| Flag | Description |
|------|-------------|
| `--clear`, `-c` | Auto-clear clipboard after duration (default: `45s`) |
| `--env`, `-e` | Environment (e.g. dev, staging, prod) |

---

## `mav search <query>`

Search secrets by name, environment, and metadata.

```bash
mav search postgres
mav search prod -o json
```

---

## `mav generate`

Generate a random password.

```bash
mav generate
mav generate --name wifi --length 24
mav generate --no-symbols --no-uppercase
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

## `mav env`

Print secrets as shell export statements.

```bash
mav env
eval $(mav env)
```

Secret names are converted to environment variable format: uppercase, dashes/dots/spaces become underscores. For example, `db-password` becomes `DB_PASSWORD`.

---

## `mav exec -- <command>`

Run a command with all secrets injected as environment variables.

```bash
mav exec -- env
mav exec -- node server.js
mav exec -- docker compose up
```

---

## `mav export`

Export all secrets to stdout (plaintext).

```bash
mav export > backup.json
mav export --format env > .env
```

| Flag | Description |
|------|-------------|
| `--format` | Export format: `json` (default), `env` |

!!! warning
    Export files contain plaintext secrets. Delete them after use.

---

## `mav import <file>`

Import secrets from a file.

```bash
mav import backup.json
mav import --format env .env
mav import backup.json --force
```

| Flag | Description |
|------|-------------|
| `--format` | File format: `json` (default), `env` |
| `--force`, `-f` | Skip confirmation prompt |

---

## `mav destroy`

Permanently destroy the vault, database, and master key.

```bash
mav destroy
mav destroy --force
```

| Flag | Description |
|------|-------------|
| `--force`, `-f` | Skip confirmation prompt |

!!! danger
    This is irreversible. All secrets and the master key are permanently deleted.

---

## `mav tui`

Launch the interactive terminal UI.

```bash
mav tui
mav tui --vim
```

| Flag | Description |
|------|-------------|
| `--vim` | Enable vim keybinding modes (Normal/Visual) |

See the [TUI Guide](tui.md) for keyboard shortcuts.

---

## `mav serve`

Start the REST API server on a Unix domain socket.

```bash
mav serve
mav serve --socket /tmp/mav.sock
mav serve --no-touchid
```

| Flag | Description |
|------|-------------|
| `--socket` | Custom socket path (default: `~/.maestrovault/maestrovault.sock`) |
| `--no-touchid` | Skip TouchID authentication at startup (for automation) |

See the [REST API reference](api.md) for endpoints.

---

## `mav token`

Manage API tokens for the REST API server.

### `token create`

```bash
mav token create --name "ci-read" --scope read
mav token create --name "deploy" --scope read,write --expires 24h
mav token create --name "admin" --scope admin
```

| Flag | Description |
|------|-------------|
| `--name` | Token name (required) |
| `--scope` | Scopes: `read`, `write`, `generate`, `admin` (required, comma-separated) |
| `--expires` | Expiry duration, e.g. `24h`, `720h` (default: no expiry) |

!!! note
    The plaintext token is shown only once at creation time. Save it somewhere safe.

### `token list`

**Alias:** `mav token ls`

```bash
mav token list
mav token list -o json
```

### `token revoke`

```bash
mav token revoke <id>
mav token revoke --all
```

| Flag | Description |
|------|-------------|
| `--all` | Revoke all tokens |

---

## `mav touchid`

Manage TouchID biometric authentication.

### `touchid enable`

```bash
mav touchid enable
```

Checks hardware availability, performs a verification prompt, then saves the setting.

### `touchid disable`

```bash
mav touchid disable
```

Requires TouchID authentication to disable (prevents unauthorized disabling).

### `touchid status`

```bash
mav touchid status
mav touchid status -o json
```

---

## `mav version`

Print build version, commit, date, Go version, and OS/arch.

```bash
mav version
mav version -o json
```

---

## Shell Completions

MaestroVault supports shell completions for Bash, Zsh, Fish, and PowerShell via Cobra's built-in completion system:

```bash
# Bash
mav completion bash > /usr/local/etc/bash_completion.d/mav

# Zsh
mav completion zsh > "${fpath[1]}/_mav"

# Fish
mav completion fish > ~/.config/fish/completions/mav.fish
```

Secret names are completed dynamically -- press Tab after `get`, `set`, `delete`, `edit`, or `copy` to see available names.
