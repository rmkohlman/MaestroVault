package api

import "github.com/rmkohlman/MaestroVault/internal/vault"

// SecretResponse is the API representation of a secret.
// We don't leak vault.SecretEntry directly in API responses.
type SecretResponse struct {
	Name        string            `json:"name"`
	Environment string            `json:"environment"`
	Value       string            `json:"value,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
	FieldCount  int               `json:"field_count,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// FieldResponse is the API representation of a single field value.
type FieldResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// secretToResponse converts a vault.SecretEntry to an API SecretResponse.
func secretToResponse(e vault.SecretEntry) SecretResponse {
	return SecretResponse{
		Name:        e.Name,
		Environment: e.Environment,
		Value:       e.Value,
		Fields:      e.Fields,
		FieldCount:  e.FieldCount,
		Metadata:    e.Metadata,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// secretsToResponse converts a slice of vault.SecretEntry to API SecretResponses.
func secretsToResponse(entries []vault.SecretEntry) []SecretResponse {
	resp := make([]SecretResponse, len(entries))
	for i, e := range entries {
		resp[i] = secretToResponse(e)
	}
	return resp
}
