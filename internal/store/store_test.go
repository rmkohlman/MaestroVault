package store

import (
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutAndGet(t *testing.T) {
	s := testStore(t)

	labels := map[string]string{"env": "dev", "app": "myapp"}
	if err := s.Put("db-password", []byte("enc-value"), []byte("enc-key"), labels); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	secret, err := s.Get("db-password")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	if secret.Name != "db-password" {
		t.Errorf("name: want %q, got %q", "db-password", secret.Name)
	}
	if string(secret.EncryptedValue) != "enc-value" {
		t.Errorf("value: want %q, got %q", "enc-value", secret.EncryptedValue)
	}
	if string(secret.EncryptedDataKey) != "enc-key" {
		t.Errorf("data key: want %q, got %q", "enc-key", secret.EncryptedDataKey)
	}
	if secret.Labels["env"] != "dev" {
		t.Errorf("label env: want %q, got %q", "dev", secret.Labels["env"])
	}
	if secret.Labels["app"] != "myapp" {
		t.Errorf("label app: want %q, got %q", "myapp", secret.Labels["app"])
	}
}

func TestPutUpsert(t *testing.T) {
	s := testStore(t)

	_ = s.Put("api-key", []byte("v1"), []byte("k1"), nil)
	_ = s.Put("api-key", []byte("v2"), []byte("k2"), nil)

	secret, err := s.Get("api-key")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(secret.EncryptedValue) != "v2" {
		t.Errorf("expected updated value %q, got %q", "v2", secret.EncryptedValue)
	}
	if string(secret.EncryptedDataKey) != "k2" {
		t.Errorf("expected updated key %q, got %q", "k2", secret.EncryptedDataKey)
	}
}

func TestGetNotFound(t *testing.T) {
	s := testStore(t)

	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestList(t *testing.T) {
	s := testStore(t)

	_ = s.Put("charlie", []byte("v3"), []byte("k3"), nil)
	_ = s.Put("alpha", []byte("v1"), []byte("k1"), nil)
	_ = s.Put("bravo", []byte("v2"), []byte("k2"), nil)

	secrets, err := s.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(secrets) != 3 {
		t.Fatalf("expected 3 secrets, got %d", len(secrets))
	}
	// Should be sorted by name.
	if secrets[0].Name != "alpha" {
		t.Errorf("first secret: want %q, got %q", "alpha", secrets[0].Name)
	}
	if secrets[1].Name != "bravo" {
		t.Errorf("second secret: want %q, got %q", "bravo", secrets[1].Name)
	}
	if secrets[2].Name != "charlie" {
		t.Errorf("third secret: want %q, got %q", "charlie", secrets[2].Name)
	}
}

func TestListEmpty(t *testing.T) {
	s := testStore(t)

	secrets, err := s.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected 0 secrets, got %d", len(secrets))
	}
}

func TestDelete(t *testing.T) {
	s := testStore(t)

	_ = s.Put("temp", []byte("v"), []byte("k"), nil)

	if err := s.Delete("temp"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := s.Get("temp")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := testStore(t)

	err := s.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent secret")
	}
}

func TestNilLabels(t *testing.T) {
	s := testStore(t)

	if err := s.Put("no-labels", []byte("v"), []byte("k"), nil); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	secret, err := s.Get("no-labels")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(secret.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", secret.Labels)
	}
}

func TestDatabaseFileCreated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "vault.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	s.Close()
}
