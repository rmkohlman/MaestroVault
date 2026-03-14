# Security & Architecture

MaestroVault is designed with a simple threat model: protect developer secrets on a local machine from unauthorized access, with minimal attack surface and no network exposure.

## Encryption

### Envelope Encryption

MaestroVault uses **envelope encryption** with two layers of keys:

```
Master Key (256-bit AES)         <- stored in macOS Keychain
    |
    +-- encrypts Data Key A      <- random, per-secret
    |       |
    |       +-- encrypts Secret A (AES-256-GCM)
    |
    +-- encrypts Data Key B
            |
            +-- encrypts Secret B (AES-256-GCM)
```

**Why envelope encryption?**

- Each secret has a unique data key, so compromising one key does not expose others
- Rotating the master key only requires re-encrypting data keys, not all secret values
- The master key never touches disk

### AES-256-GCM

All encryption uses AES-256 in GCM (Galois/Counter Mode):

- **256-bit keys** (32 bytes) generated via `crypto/rand`
- **96-bit nonces** generated randomly for each encryption
- **Authenticated encryption** -- GCM provides both confidentiality and integrity
- Nonce is prepended to ciphertext for storage

### Key Generation

Keys are generated using Go's `crypto/rand`, which reads from `/dev/urandom` on macOS. This is a cryptographically secure pseudorandom number generator (CSPRNG).

## Key Storage

### macOS Keychain

The master key is stored in the macOS Keychain using `github.com/keybase/go-keychain`:

- **Service:** `MaestroVault`
- **Account:** `master-key`
- **Accessible:** `WhenUnlockedThisDeviceOnly` -- the key is only available when the device is unlocked and cannot be synced to other devices via iCloud Keychain
- **Synchronizable:** `No`

The Keychain is protected by your macOS login password and the Secure Enclave (on Apple Silicon Macs).

### What this means

- The master key is never written to the filesystem
- The key is protected by macOS at the OS level
- Even if someone copies `~/.maestrovault/vault.db`, they cannot decrypt secrets without access to your Keychain

## TouchID

When enabled, MaestroVault uses the macOS `LocalAuthentication` framework to require biometric authentication before opening the vault.

- Uses `LAPolicyDeviceOwnerAuthenticationWithBiometrics` (biometrics only, no passcode fallback)
- Authentication happens once at vault open (session-based, low friction)
- Requires TouchID to disable (prevents unauthorized disabling)
- Configuration stored in `~/.maestrovault/config.json`

TouchID adds a hardware-backed factor: even if someone has your macOS login password, they need your fingerprint to access secrets.

## Database

### SQLite

Secrets are stored in a SQLite database at `~/.maestrovault/vault.db`:

- Uses `modernc.org/sqlite` (pure Go, no CGo dependency for the database)
- WAL mode enabled for better concurrent read performance
- Foreign keys enabled

### Schema

```sql
CREATE TABLE secrets (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    name               TEXT NOT NULL,
    environment        TEXT NOT NULL DEFAULT '',
    encrypted_secret   BLOB NOT NULL,
    encrypted_data_key BLOB NOT NULL,
    metadata           TEXT NOT NULL DEFAULT '{}',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, environment)
);

CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    token_hash   TEXT UNIQUE NOT NULL,
    salt         TEXT NOT NULL DEFAULT '',
    scopes       TEXT NOT NULL DEFAULT '[]',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   DATETIME,
    last_used_at DATETIME
);
```

**What is stored in the database:**

- Encrypted secret values (AES-256-GCM ciphertext)
- Encrypted data keys (wrapped with the master key)
- Secret metadata (name, environment, metadata, timestamps)
- API token hashes (HMAC-SHA256 with salt; plaintext tokens are never stored)

**What is NOT stored in the database:**

- Plaintext secret values
- Plaintext data keys
- The master key
- Plaintext API tokens

## API Tokens

API tokens for the REST server use:

- **Format:** `mvt_` prefix + 64 hex characters (32 random bytes)
- **Storage:** HMAC-SHA256 hash with per-token salt is stored in the database (legacy tokens validated with plain SHA-256 for backward compatibility)
- **Scopes:** `read`, `write`, `generate`, `admin` (admin grants all)
- **Expiry:** Optional, checked on each request
- **Last used:** Updated asynchronously on each successful authentication

The plaintext token is shown exactly once at creation time.

## REST API Security

- **Unix domain socket** -- no network exposure, no TCP/IP, no TLS needed
- **Socket permissions:** `0600` (owner read/write only)
- **Bearer token auth** on all endpoints except health
- **Scoped access** -- tokens are restricted to specific operations
- **Request logging** -- all requests are logged with method and path

## Threat Model

### What MaestroVault protects against

| Threat | Protection |
|--------|------------|
| Secrets in plaintext on disk | AES-256-GCM encryption |
| Database file stolen | Useless without master key (in Keychain) |
| Master key on disk | Never written to disk; lives in Keychain only |
| Unauthorized vault access | TouchID biometric gate |
| API token stolen | Scoped permissions, expiry, revocation |
| Network sniffing | Unix socket; no network exposure |
| Process injection | macOS SIP and app sandbox protections |

### What MaestroVault does NOT protect against

| Threat | Explanation |
|--------|-------------|
| Root/admin access to your Mac | A root user can access Keychain and memory |
| Physical access with your fingerprint | TouchID can be bypassed with physical coercion |
| Memory forensics | Decrypted values exist in process memory briefly |
| Malware with Keychain access | If malware is granted Keychain access, the master key is exposed |

MaestroVault is a developer convenience tool, not a hardware security module. It raises the bar significantly above plaintext `.env` files, but it is not designed to resist nation-state adversaries with physical access to your machine.

## File Permissions

| Path | Permissions | Contents |
|------|-------------|----------|
| `~/.maestrovault/` | `0700` | Vault directory |
| `~/.maestrovault/vault.db` | Created by SQLite | Encrypted secrets |
| `~/.maestrovault/config.json` | `0600` | TouchID settings |
| `~/.maestrovault/maestrovault.sock` | `0600` | API Unix socket |
