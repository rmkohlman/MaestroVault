# REST API Reference

MaestroVault includes a REST API server that listens on a Unix domain socket. This allows other tools, scripts, and services on the same machine to interact with secrets programmatically.

## Starting the Server

```bash
mav serve
```

Default socket: `~/.maestrovault/maestrovault.sock`

Custom socket:

```bash
mav serve --socket /tmp/mav.sock
```

The server runs in the foreground and shuts down gracefully on `Ctrl+C` (SIGINT/SIGTERM). The socket file is cleaned up on exit.

## Authentication

All endpoints except `/v1/health` require a Bearer token in the `Authorization` header:

```
Authorization: Bearer mvt_abc123...
```

Create tokens with:

```bash
mav token create --name "my-token" --scope read,write
```

### Scopes

| Scope | Grants |
|-------|--------|
| `read` | Get, list, search, info |
| `write` | Set, edit, delete |
| `generate` | Password generation |
| `admin` | Token management (implicitly grants all other scopes) |

## Endpoints

### Health Check

```
GET /v1/health
```

No authentication required.

**Response:**

```json
{
  "status": "ok",
  "time": "2026-03-13T12:00:00Z"
}
```

---

### List Secrets

```
GET /v1/secrets
```

**Scope:** `read`

**Query Parameters:**

| Parameter | Description |
|-----------|-------------|
| `env` | Filter by environment |
| `metadata_key` | Filter by metadata key |
| `metadata_value` | Filter by metadata value (requires `metadata_key`) |

**Response:**

```json
[
  {
    "name": "db-password",
    "environment": "prod",
    "metadata": {"service": "postgres"},
    "created_at": "2026-03-13T10:00:00Z",
    "updated_at": "2026-03-13T10:00:00Z"
  }
]
```

!!! note
    List does not decrypt values. Use GET by name to retrieve the plaintext.

---

### Get Secret

```
GET /v1/secrets/{name}
```

**Scope:** `read`

**Query Parameters:**

| Parameter | Description |
|-----------|-------------|
| `env` | Environment (default: empty string) |

**Response:**

```json
{
  "name": "db-password",
  "environment": "prod",
  "value": "s3cret",
  "metadata": {"service": "postgres"},
  "created_at": "2026-03-13T10:00:00Z",
  "updated_at": "2026-03-13T10:00:00Z"
}
```

---

### Set Secret

```
PUT /v1/secrets/{name}
```

**Scope:** `write`

**Query Parameters:**

| Parameter | Description |
|-----------|-------------|
| `env` | Environment (default: empty string) |

**Request Body:**

```json
{
  "value": "my-secret-value",
  "metadata": {"service": "api", "team": "backend"}
}
```

**Response (201):**

```json
{
  "status": "stored",
  "name": "my-secret"
}
```

---

### Edit Secret

```
PATCH /v1/secrets/{name}
```

**Scope:** `write`

Partial update -- omitted fields are preserved.

**Query Parameters:**

| Parameter | Description |
|-----------|-------------|
| `env` | Environment (default: empty string) |

**Request Body:**

```json
{
  "value": "new-value",
  "metadata": {"service": "api"}
}
```

**Response:**

```json
{
  "status": "updated",
  "name": "my-secret"
}
```

---

### Delete Secret

```
DELETE /v1/secrets/{name}
```

**Scope:** `write`

**Query Parameters:**

| Parameter | Description |
|-----------|-------------|
| `env` | Environment (default: empty string) |

**Response:**

```json
{
  "status": "deleted",
  "name": "my-secret"
}
```

---

### Search

```
GET /v1/search?q={query}
```

**Scope:** `read`

Searches secret names, environments, and metadata.

**Response:** Same format as List Secrets.

---

### Generate Password

```
POST /v1/generate
```

**Scope:** `generate`

**Request Body:**

```json
{
  "name": "wifi-password",
  "environment": "home",
  "length": 24,
  "uppercase": true,
  "lowercase": true,
  "digits": true,
  "symbols": false,
  "metadata": {"type": "wifi"}
}
```

All fields are optional. Defaults: length 32, all character sets enabled. If `name` is provided, the password is stored as a secret. Include `environment` to scope the stored secret.

**Response:**

```json
{
  "password": "xK9mP2vL...",
  "name": "wifi-password",
  "stored": true
}
```

---

### Vault Info

```
GET /v1/info
```

**Scope:** `read`

**Response:**

```json
{
  "dir": "/Users/you/.maestrovault",
  "db_path": "/Users/you/.maestrovault/vault.db",
  "db_size_bytes": 32768,
  "secret_count": 42
}
```

---

### List Tokens

```
GET /v1/tokens
```

**Scope:** `admin`

**Response:**

```json
[
  {
    "id": "a1b2c3d4e5f6a7b8",
    "name": "ci-read",
    "scopes": ["read"],
    "created_at": "2026-03-13T10:00:00Z",
    "expires_at": null,
    "last_used_at": "2026-03-13T11:30:00Z"
  }
]
```

---

### Create Token

```
POST /v1/tokens
```

**Scope:** `admin`

**Request Body:**

```json
{
  "name": "deploy-token",
  "scopes": ["read", "write"],
  "expires_in": "720h"
}
```

`expires_in` is optional. Use Go duration format (`24h`, `720h`, etc.) or omit for no expiry.

**Response (201):**

```json
{
  "token": "mvt_abc123...",
  "id": "a1b2c3d4e5f6a7b8",
  "name": "deploy-token",
  "scopes": ["read", "write"],
  "expires_at": "2026-04-12T10:00:00Z"
}
```

!!! warning
    The plaintext token is only returned once. Store it securely.

---

### Revoke Token

```
DELETE /v1/tokens/{id}
```

**Scope:** `admin`

**Response:**

```json
{
  "status": "revoked",
  "id": "a1b2c3d4e5f6a7b8"
}
```

---

## Error Responses

All errors return JSON:

```json
{
  "error": "description of the problem"
}
```

| Status | Meaning |
|--------|---------|
| 400 | Bad request (missing/invalid parameters) |
| 401 | Unauthorized (missing/invalid token) |
| 403 | Forbidden (insufficient scope) |
| 404 | Not found |
| 500 | Internal server error |

## Using with curl

Since the API uses a Unix socket, use curl's `--unix-socket` flag:

```bash
# Health check
curl --unix-socket ~/.maestrovault/maestrovault.sock \
  http://localhost/v1/health

# Get a secret (with environment)
curl --unix-socket ~/.maestrovault/maestrovault.sock \
  -H "Authorization: Bearer mvt_your_token_here" \
  http://localhost/v1/secrets/db-password?env=prod

# Store a secret
curl --unix-socket ~/.maestrovault/maestrovault.sock \
  -H "Authorization: Bearer mvt_your_token_here" \
  -X PUT \
  -d '{"value": "s3cret", "metadata": {"service": "db"}}' \
  http://localhost/v1/secrets/my-key?env=dev
```

## Socket Security

The Unix socket is created with `0600` permissions (owner read/write only). Only the user who started the server can connect. This provides OS-level access control without network exposure.
