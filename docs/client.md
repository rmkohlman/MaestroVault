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

    // Store a secret
    err = c.Set("api-key", "sk-abc123", map[string]string{"env": "dev"})
    if err != nil {
        log.Fatal(err)
    }

    // Retrieve it
    secret, err := c.Get("api-key")
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
secret, err := c.Get("db-password")
// secret.Name, secret.Value, secret.Labels, secret.CreatedAt, secret.UpdatedAt
```

### List

```go
secrets, err := c.List()
for _, s := range secrets {
    fmt.Println(s.Name)
}
```

### List by Label

```go
secrets, err := c.ListByLabel("env", "prod")
```

### Set

```go
err := c.Set("name", "value", map[string]string{"key": "val"})
```

### Edit

```go
newVal := "updated-value"
err := c.Edit("name", &newVal, nil) // nil labels = keep existing
```

### Delete

```go
err := c.Delete("name")
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
    Name      string
    Value     string
    Labels    map[string]string
    CreatedAt string
    UpdatedAt string
}
```

### VaultInfo

```go
type VaultInfo struct {
    Dir         string
    DBPath      string
    DBSize      int64
    SecretCount int
}
```

### GenerateOpts

```go
type GenerateOpts struct {
    Name      string
    Length    int
    Uppercase *bool
    Lowercase *bool
    Digits    *bool
    Symbols   *bool
    Labels    map[string]string
}
```

### TokenInfo

```go
type TokenInfo struct {
    ID         string
    Name       string
    Scopes     []string
    CreatedAt  time.Time
    ExpiresAt  *time.Time
    LastUsedAt *time.Time
}
```
