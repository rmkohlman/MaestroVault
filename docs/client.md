# Go Client Library

MaestroVault ships a Go client library at `pkg/client` for programmatic access to the REST API. It communicates over the Unix domain socket using standard `net/http`.

## Installation

```bash
go get github.com/rmkohlman/MaestroVault/pkg/client
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    "github.com/rmkohlman/MaestroVault/pkg/client"
)

func main() {
    c, err := client.New("mvt_your_token_here")
    if err != nil {
        log.Fatal(err)
    }

    // Store a secret in the "dev" environment
    err = c.Set("api-key", "dev", "sk-abc123", map[string]any{"service": "api"})
    if err != nil {
        log.Fatal(err)
    }

    // Retrieve it
    secret, err := c.Get("api-key", "dev")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Value: %s\n", secret.Value)
}
```

## Creating a Client

```go
// Default socket path (~/.maestrovault/maestrovault.sock)
c, err := client.New("mvt_token")

// Custom socket path
c, err := client.New("mvt_token", client.WithSocketPath("/tmp/mv.sock"))

// Custom timeout
c, err := client.New("mvt_token", client.WithTimeout(10 * time.Second))
```

## Secrets

### Get

```go
secret, err := c.Get("db-password", "prod")
// secret.Name, secret.Environment, secret.Value, secret.Metadata, secret.CreatedAt, secret.UpdatedAt
```

### List

```go
secrets, err := c.List("prod") // pass "" for all environments
for _, s := range secrets {
    fmt.Println(s.Name, s.Environment)
}
```

### List by Metadata

```go
secrets, err := c.ListByMetadata("service", "postgres")
```

### Set

```go
err := c.Set("name", "prod", "value", map[string]any{"key": "val"})
```

### Edit

```go
newVal := "updated-value"
err := c.Edit("name", "prod", &newVal, nil) // nil metadata = keep existing
```

### Delete

```go
err := c.Delete("name", "prod")
```

## Search

```go
results, err := c.Search("postgres")
```

## Generate Password

```go
result, err := c.Generate(client.GenerateOpts{
    Name:   "wifi-password",
    Length: 24,
})
fmt.Println(result.Password)
fmt.Println(result.Stored) // true if Name was provided
```

## Vault Info

```go
info, err := c.Info()
fmt.Printf("Secrets: %d\n", info.SecretCount)
fmt.Printf("DB size: %d bytes\n", info.DBSize)
```

## Health Check

```go
err := c.Health()
if err != nil {
    fmt.Println("Server is down")
}
```

## Token Management

Requires a token with `admin` scope.

### List Tokens

```go
tokens, err := c.ListTokens()
for _, t := range tokens {
    fmt.Printf("%s: %s (%v)\n", t.ID, t.Name, t.Scopes)
}
```

### Create Token

```go
result, err := c.CreateToken("ci-reader", []string{"read"}, "720h")
fmt.Println(result.Token) // Save this -- shown only once
```

### Revoke Token

```go
err := c.RevokeToken("token-id")
```

## Error Handling

All methods return errors for HTTP 4xx/5xx responses. The error message includes the status code and server error message:

```go
secret, err := c.Get("nonexistent")
if err != nil {
    // err: "API error (404): secret \"nonexistent\" not found"
    fmt.Println(err)
}
```

## Types

### SecretEntry

```go
type SecretEntry struct {
    Name        string         `json:"name"`
    Environment string         `json:"environment"`
    Value       string         `json:"value,omitempty"`
    Metadata    map[string]any `json:"metadata,omitempty"`
    CreatedAt   string         `json:"created_at"`
    UpdatedAt   string         `json:"updated_at"`
}
```

### VaultInfo

```go
type VaultInfo struct {
    Dir         string `json:"dir"`
    DBPath      string `json:"db_path"`
    DBSize      int64  `json:"db_size_bytes"`
    SecretCount int    `json:"secret_count"`
}
```

### GenerateOpts

```go
type GenerateOpts struct {
    Name        string         `json:"name,omitempty"`
    Environment string         `json:"environment,omitempty"`
    Length      int            `json:"length,omitempty"`
    Uppercase   *bool          `json:"uppercase,omitempty"`
    Lowercase   *bool          `json:"lowercase,omitempty"`
    Digits      *bool          `json:"digits,omitempty"`
    Symbols     *bool          `json:"symbols,omitempty"`
    Metadata    map[string]any `json:"metadata,omitempty"`
}
```

### TokenInfo

```go
type TokenInfo struct {
    ID         string     `json:"id"`
    Name       string     `json:"name"`
    Scopes     []string   `json:"scopes"`
    CreatedAt  time.Time  `json:"created_at"`
    ExpiresAt  *time.Time `json:"expires_at,omitempty"`
    LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}
```
