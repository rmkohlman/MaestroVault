// Package client_test provides integration tests for the MaestroVault Go client.
//
// These tests require a running API server:
//
//	mav serve --no-touchid
//
// And the MAV_TEST_TOKEN environment variable set to a valid admin-scoped token:
//
//	export MAV_TEST_TOKEN="mvt_..."
//	go test -tags integration ./pkg/client/
//
// The tests create, read, update, and delete secrets with a "clienttest_" prefix
// to avoid collisions with real data.

//go:build integration

package client_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rmkohlman/MaestroVault/pkg/client"
)

const testPrefix = "clienttest_"

func testClient(t *testing.T) *client.Client {
	t.Helper()
	token := os.Getenv("MAV_TEST_TOKEN")
	if token == "" {
		t.Fatal("MAV_TEST_TOKEN not set — run: export MAV_TEST_TOKEN=mvt_...")
	}
	c, err := client.New(token)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

// cleanup removes any test secrets that may have been left behind.
func cleanup(t *testing.T, c *client.Client) {
	t.Helper()
	entries, err := c.List("")
	if err != nil {
		return // best-effort
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name, testPrefix) {
			_ = c.Delete(e.Name, e.Environment)
		}
	}
}

// ── Health ────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	c := testClient(t)
	if err := c.Health(); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

// ── Info ──────────────────────────────────────────────────────

func TestInfo(t *testing.T) {
	c := testClient(t)
	info, err := c.Info()
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Dir == "" {
		t.Error("Info.Dir is empty")
	}
	if info.DBPath == "" {
		t.Error("Info.DBPath is empty")
	}
	if info.SecretCount < 0 {
		t.Errorf("Info.SecretCount = %d, want >= 0", info.SecretCount)
	}
	t.Logf("Vault: %s, secrets: %d, DB size: %d bytes", info.Dir, info.SecretCount, info.DBSize)
}

// ── Set / Get / Delete (full CRUD cycle) ──────────────────────

func TestCRUD(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	name := testPrefix + "crud"
	env := "test"
	value := "s3cret-value-123"
	meta := map[string]any{"service": "integration-test"}

	// Set
	if err := c.Set(name, env, value, meta); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get
	entry, err := c.Get(name, env)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.Name != name {
		t.Errorf("Get.Name = %q, want %q", entry.Name, name)
	}
	if entry.Environment != env {
		t.Errorf("Get.Environment = %q, want %q", entry.Environment, env)
	}
	if entry.Value != value {
		t.Errorf("Get.Value = %q, want %q", entry.Value, value)
	}
	if entry.Metadata["service"] != "integration-test" {
		t.Errorf("Get.Metadata[service] = %v, want \"integration-test\"", entry.Metadata["service"])
	}
	if entry.CreatedAt == "" {
		t.Error("Get.CreatedAt is empty")
	}
	t.Logf("Set+Get OK: %s (env=%s)", name, env)

	// Delete
	if err := c.Delete(name, env); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted
	_, err = c.Get(name, env)
	if err == nil {
		t.Fatal("Get after Delete: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Get after Delete: expected 404, got: %v", err)
	}
	t.Log("Delete verified: secret no longer exists")
}

// ── Edit ──────────────────────────────────────────────────────

func TestEdit(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	name := testPrefix + "edit"
	env := "test"

	// Create
	if err := c.Set(name, env, "original", map[string]any{"version": "1"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Edit value only (metadata preserved)
	newVal := "updated-value"
	if err := c.Edit(name, env, &newVal, nil); err != nil {
		t.Fatalf("Edit value: %v", err)
	}

	entry, err := c.Get(name, env)
	if err != nil {
		t.Fatalf("Get after edit value: %v", err)
	}
	if entry.Value != "updated-value" {
		t.Errorf("Value after edit = %q, want \"updated-value\"", entry.Value)
	}
	if entry.Metadata["version"] != "1" {
		t.Errorf("Metadata preserved = %v, want version=1", entry.Metadata)
	}

	// Edit metadata only (value preserved)
	newMeta := map[string]any{"version": "2", "edited": true}
	if err := c.Edit(name, env, nil, newMeta); err != nil {
		t.Fatalf("Edit metadata: %v", err)
	}

	entry, err = c.Get(name, env)
	if err != nil {
		t.Fatalf("Get after edit metadata: %v", err)
	}
	if entry.Value != "updated-value" {
		t.Errorf("Value preserved = %q, want \"updated-value\"", entry.Value)
	}
	if entry.Metadata["version"] != "2" {
		t.Errorf("Metadata[version] = %v, want \"2\"", entry.Metadata["version"])
	}
	t.Log("Edit OK: value and metadata updated independently")

	_ = c.Delete(name, env)
}

// ── List ──────────────────────────────────────────────────────

func TestList(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	// Create secrets in two environments
	if err := c.Set(testPrefix+"list-a", "dev", "a", nil); err != nil {
		t.Fatalf("Set list-a: %v", err)
	}
	if err := c.Set(testPrefix+"list-b", "prod", "b", nil); err != nil {
		t.Fatalf("Set list-b: %v", err)
	}

	// List all
	all, err := c.List("")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) < 2 {
		t.Errorf("List all: got %d entries, want >= 2", len(all))
	}

	// List filtered by env
	devOnly, err := c.List("dev")
	if err != nil {
		t.Fatalf("List dev: %v", err)
	}
	for _, e := range devOnly {
		if e.Environment != "dev" {
			t.Errorf("List(dev) returned entry with env=%q", e.Environment)
		}
	}
	t.Logf("List OK: all=%d, dev=%d", len(all), len(devOnly))

	_ = c.Delete(testPrefix+"list-a", "dev")
	_ = c.Delete(testPrefix+"list-b", "prod")
}

// ── ListByMetadata ────────────────────────────────────────────

func TestListByMetadata(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	if err := c.Set(testPrefix+"meta", "test", "v", map[string]any{"team": "backend"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	results, err := c.ListByMetadata("team", "backend")
	if err != nil {
		t.Fatalf("ListByMetadata: %v", err)
	}

	found := false
	for _, e := range results {
		if e.Name == testPrefix+"meta" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListByMetadata: test secret not found in results")
	}
	t.Logf("ListByMetadata OK: %d results", len(results))

	_ = c.Delete(testPrefix+"meta", "test")
}

// ── Search ────────────────────────────────────────────────────

func TestSearch(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	if err := c.Set(testPrefix+"searchable", "test", "v", nil); err != nil {
		t.Fatalf("Set: %v", err)
	}

	results, err := c.Search("clienttest_searchable")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search: no results for 'clienttest_searchable'")
	}
	t.Logf("Search OK: %d results", len(results))

	_ = c.Delete(testPrefix+"searchable", "test")
}

// ── Generate ──────────────────────────────────────────────────

func TestGenerate(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	// Generate without storing
	result, err := c.Generate(client.GenerateOpts{Length: 20})
	if err != nil {
		t.Fatalf("Generate (no store): %v", err)
	}
	if len(result.Password) != 20 {
		t.Errorf("Generate password length = %d, want 20", len(result.Password))
	}
	if result.Stored {
		t.Error("Generate (no store): Stored should be false")
	}

	// Generate and store
	result, err = c.Generate(client.GenerateOpts{
		Name:        testPrefix + "generated",
		Environment: "test",
		Length:      16,
	})
	if err != nil {
		t.Fatalf("Generate (store): %v", err)
	}
	if !result.Stored {
		t.Error("Generate (store): Stored should be true")
	}

	// Verify it was stored
	entry, err := c.Get(testPrefix+"generated", "test")
	if err != nil {
		t.Fatalf("Get generated: %v", err)
	}
	if entry.Value != result.Password {
		t.Errorf("Stored value = %q, generated = %q", entry.Value, result.Password)
	}
	t.Logf("Generate OK: password=%q, stored=%v", result.Password[:8]+"...", result.Stored)

	_ = c.Delete(testPrefix+"generated", "test")
}

// ── Token management ──────────────────────────────────────────

func TestTokenManagement(t *testing.T) {
	c := testClient(t)

	// Create a token
	created, err := c.CreateToken("client-test-token", []string{"read"}, "1h")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if created.Token == "" {
		t.Error("CreateToken: token is empty")
	}
	if created.ID == "" {
		t.Error("CreateToken: ID is empty")
	}
	if created.Name != "client-test-token" {
		t.Errorf("CreateToken: name = %q, want \"client-test-token\"", created.Name)
	}
	t.Logf("CreateToken OK: id=%s", created.ID)

	// List tokens — should include our new one
	tokens, err := c.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	found := false
	for _, tok := range tokens {
		if tok.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListTokens: created token not found")
	}
	t.Logf("ListTokens OK: %d tokens", len(tokens))

	// Revoke the token
	if err := c.RevokeToken(created.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Verify revoked — list should not include it
	tokens, err = c.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens after revoke: %v", err)
	}
	for _, tok := range tokens {
		if tok.ID == created.ID {
			t.Error("RevokeToken: token still present after revocation")
		}
	}
	t.Log("RevokeToken OK: token removed")
}

// ── Error handling ────────────────────────────────────────────

func TestErrorHandling(t *testing.T) {
	c := testClient(t)

	// Get nonexistent secret
	_, err := c.Get("clienttest_nonexistent_xyz", "")
	if err == nil {
		t.Fatal("Get nonexistent: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Get nonexistent: expected 404, got: %v", err)
	}

	// Delete nonexistent secret
	err = c.Delete("clienttest_nonexistent_xyz", "")
	if err == nil {
		t.Fatal("Delete nonexistent: expected error, got nil")
	}

	t.Logf("Error handling OK: proper errors for missing secrets")
}

// ── Environment scoping ───────────────────────────────────────

func TestEnvironmentScoping(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	name := testPrefix + "envscope"

	// Same name, different environments
	if err := c.Set(name, "dev", "dev-value", nil); err != nil {
		t.Fatalf("Set dev: %v", err)
	}
	if err := c.Set(name, "prod", "prod-value", nil); err != nil {
		t.Fatalf("Set prod: %v", err)
	}

	// Retrieve each independently
	devEntry, err := c.Get(name, "dev")
	if err != nil {
		t.Fatalf("Get dev: %v", err)
	}
	prodEntry, err := c.Get(name, "prod")
	if err != nil {
		t.Fatalf("Get prod: %v", err)
	}

	if devEntry.Value != "dev-value" {
		t.Errorf("dev value = %q, want \"dev-value\"", devEntry.Value)
	}
	if prodEntry.Value != "prod-value" {
		t.Errorf("prod value = %q, want \"prod-value\"", prodEntry.Value)
	}

	// Delete only dev — prod should remain
	if err := c.Delete(name, "dev"); err != nil {
		t.Fatalf("Delete dev: %v", err)
	}
	_, err = c.Get(name, "dev")
	if err == nil {
		t.Error("Get dev after delete: expected error")
	}
	prodEntry, err = c.Get(name, "prod")
	if err != nil {
		t.Fatalf("Get prod after dev delete: %v", err)
	}
	if prodEntry.Value != "prod-value" {
		t.Errorf("prod value after dev delete = %q", prodEntry.Value)
	}
	t.Log("Environment scoping OK: same name, independent environments")

	_ = c.Delete(name, "prod")
}

// ── Fields (full CRUD cycle) ──────────────────────────────────

func TestFieldsCRUD(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	name := testPrefix + "fields"
	env := "test"

	// SetField on a non-existent secret should auto-create the parent.
	if err := c.SetField(name, env, "host", "db.example.com"); err != nil {
		t.Fatalf("SetField (host): %v", err)
	}
	if err := c.SetField(name, env, "port", "5432"); err != nil {
		t.Fatalf("SetField (port): %v", err)
	}

	// GetField
	val, err := c.GetField(name, env, "host")
	if err != nil {
		t.Fatalf("GetField (host): %v", err)
	}
	if val != "db.example.com" {
		t.Errorf("GetField (host) = %q, want \"db.example.com\"", val)
	}

	// GetFields — should return all fields
	fields, err := c.GetFields(name, env)
	if err != nil {
		t.Fatalf("GetFields: %v", err)
	}
	if len(fields) != 2 {
		t.Errorf("GetFields: got %d fields, want 2", len(fields))
	}
	if fields["host"] != "db.example.com" {
		t.Errorf("GetFields[host] = %q, want \"db.example.com\"", fields["host"])
	}
	if fields["port"] != "5432" {
		t.Errorf("GetFields[port] = %q, want \"5432\"", fields["port"])
	}

	// Get full secret — should include fields and field_count
	entry, err := c.Get(name, env)
	if err != nil {
		t.Fatalf("Get (with fields): %v", err)
	}
	if entry.FieldCount != 2 {
		t.Errorf("Get.FieldCount = %d, want 2", entry.FieldCount)
	}
	if len(entry.Fields) != 2 {
		t.Errorf("Get.Fields has %d entries, want 2", len(entry.Fields))
	}
	t.Logf("Fields CRUD: set/get OK, count=%d", entry.FieldCount)

	// Update an existing field
	if err := c.SetField(name, env, "port", "3306"); err != nil {
		t.Fatalf("SetField (update port): %v", err)
	}
	val, err = c.GetField(name, env, "port")
	if err != nil {
		t.Fatalf("GetField (updated port): %v", err)
	}
	if val != "3306" {
		t.Errorf("Updated port = %q, want \"3306\"", val)
	}

	// DeleteField
	if err := c.DeleteField(name, env, "port"); err != nil {
		t.Fatalf("DeleteField: %v", err)
	}

	// Verify deleted — GetField should fail
	_, err = c.GetField(name, env, "port")
	if err == nil {
		t.Error("GetField after delete: expected error, got nil")
	}

	// Remaining fields should be just "host"
	fields, err = c.GetFields(name, env)
	if err != nil {
		t.Fatalf("GetFields after delete: %v", err)
	}
	if len(fields) != 1 {
		t.Errorf("GetFields after delete: got %d fields, want 1", len(fields))
	}

	t.Log("Fields CRUD: update, delete, verify OK")

	_ = c.Delete(name, env)
}

// ── SetFields (batch) ─────────────────────────────────────────

func TestSetFieldsBatch(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	name := testPrefix + "batchfields"
	env := "test"

	// Batch set multiple fields
	batch := map[string]string{
		"username": "admin",
		"password": "s3cret",
		"host":     "10.0.0.1",
	}
	if err := c.SetFields(name, env, batch); err != nil {
		t.Fatalf("SetFields: %v", err)
	}

	// Verify all were stored
	fields, err := c.GetFields(name, env)
	if err != nil {
		t.Fatalf("GetFields: %v", err)
	}
	if len(fields) != 3 {
		t.Errorf("GetFields: got %d fields, want 3", len(fields))
	}
	for k, want := range batch {
		if got := fields[k]; got != want {
			t.Errorf("Field %q = %q, want %q", k, got, want)
		}
	}
	t.Logf("SetFields batch OK: %d fields", len(fields))

	_ = c.Delete(name, env)
}

// ── Fields-only entry (no main value) ─────────────────────────

func TestFieldsOnlyEntry(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	name := testPrefix + "fieldsonly"
	env := "test"

	// Create via SetField — parent has no main value
	if err := c.SetField(name, env, "key1", "val1"); err != nil {
		t.Fatalf("SetField: %v", err)
	}

	// Get full secret — value should be empty, fields present
	entry, err := c.Get(name, env)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.Value != "" {
		t.Errorf("Value = %q, want empty (fields-only entry)", entry.Value)
	}
	if entry.FieldCount != 1 {
		t.Errorf("FieldCount = %d, want 1", entry.FieldCount)
	}
	if entry.Fields["key1"] != "val1" {
		t.Errorf("Fields[key1] = %q, want \"val1\"", entry.Fields["key1"])
	}
	t.Log("Fields-only entry OK: no main value, fields present")

	_ = c.Delete(name, env)
}

// ── Deleting parent cascades to fields ────────────────────────

func TestDeleteCascadesFields(t *testing.T) {
	c := testClient(t)
	t.Cleanup(func() { cleanup(t, c) })

	name := testPrefix + "cascade"
	env := "test"

	// Create parent with value and fields
	if err := c.Set(name, env, "main-value", nil); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := c.SetField(name, env, "f1", "v1"); err != nil {
		t.Fatalf("SetField: %v", err)
	}

	// Delete the entire secret
	if err := c.Delete(name, env); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Parent should be gone
	_, err := c.Get(name, env)
	if err == nil {
		t.Fatal("Get after cascade delete: expected error, got nil")
	}

	t.Log("Cascade delete OK: parent + fields removed")
}
