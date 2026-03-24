package tui

import (
	"strings"
	"testing"

	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// generateFakePEMChain creates a realistic fake PEM certificate chain
// with 4 sections, totalling exactly 100 lines — simulating a large cert.
func generateFakePEMChain() string {
	// Each section is 23 lines of base64 content + 2 header/footer lines = 25 lines.
	// Four sections = 100 lines total.
	section := func(n int) string {
		header := "-----BEGIN CERTIFICATE-----"
		footer := "-----END CERTIFICATE-----"
		// 23 lines of realistic base64 (64 chars each)
		b64lines := make([]string, 23)
		for i := range b64lines {
			// Vary the content slightly per line/section to look realistic
			b64lines[i] = strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwxyz+/", 1)[:64]
		}
		_ = n // section number unused for content, used for clarity
		lines := []string{header}
		lines = append(lines, b64lines...)
		lines = append(lines, footer)
		return strings.Join(lines, "\n")
	}

	sections := []string{
		section(1),
		section(2),
		section(3),
		section(4),
	}
	return strings.Join(sections, "\n")
}

// TestReproduceModalOverflow verifies that large PEM cert values
// render within terminal bounds. Values start masked (constructor
// does NOT call initValueTextarea); textarea is lazily initialized
// via Ctrl+R reveal.
func TestReproduceModalOverflow(t *testing.T) {
	certValue := generateFakePEMChain()
	certLineCount := strings.Count(certValue, "\n") + 1
	t.Logf("PEM cert: %d lines, %d bytes", certLineCount, len(certValue))

	entry := &vault.SecretEntry{
		Name:        "my-full-cert",
		Environment: "production",
		Value:       certValue,
		CreatedAt:   "2026-03-23T00:00:00Z",
		UpdatedAt:   "2026-03-23T00:00:00Z",
	}

	// ── Path 1: NewSecretModalEdit (constructor) — values start masked ──
	modal := NewSecretModalEdit(entry, nil)
	modal.width = 80
	modal.height = 24
	modal.initViewport()

	if modal.useTextarea {
		t.Error("expected useTextarea=false for constructor — values start masked")
	}

	output := modal.View()
	rawLineCount := len(strings.Split(output, "\n"))

	t.Logf("Constructor path (masked): %d lines (terminal 80x24, useTextarea=%v)", rawLineCount, modal.useTextarea)

	if rawLineCount > 24 {
		t.Errorf("OVERFLOW (NewSecretModalEdit path, masked): %d lines for 24-row terminal (excess: %d)",
			rawLineCount, rawLineCount-24)
	}

	// ── Path 2: view→edit transition (pressing "e") — also starts masked ──
	viewModal := NewSecretModalView(entry, nil)
	viewModal.width = 80
	viewModal.height = 24
	viewModal.initViewport()

	// Simulate pressing "e" — mirrors handleViewKey()
	viewModal.mode = modalEdit
	viewModal.origName = entry.Name
	viewModal.origEnv = entry.Environment
	viewModal.nameInput.SetValue(entry.Name)
	viewModal.envInput.SetValue(entry.Environment)
	viewModal.metadataInput.SetValue(formatMetadataPlain(entry.Metadata))
	// Value starts masked — just set textinput, no textarea.
	viewModal.valueInput.SetValue(entry.Value)
	viewModal.useTextarea = false
	viewModal.valueRevealed = false
	viewModal.focusField = fieldName
	viewModal.focusCurrentField()

	viewEditOutput := viewModal.View()
	viewEditLines := len(strings.Split(viewEditOutput, "\n"))

	t.Logf("View→Edit path (masked): %d lines (terminal 80x24, useTextarea=%v)", viewEditLines, viewModal.useTextarea)

	if viewEditLines > 24 {
		t.Errorf("OVERFLOW (view→edit path, masked): %d lines for 24-row terminal (excess: %d)",
			viewEditLines, viewEditLines-24)
	}

	// ── Multi-size scan (masked) ──
	for _, size := range []struct{ w, h int }{
		{80, 24},
		{120, 40},
		{60, 15},
		{80, 50},
	} {
		m2 := NewSecretModalEdit(entry, nil)
		m2.width = size.w
		m2.height = size.h
		m2.initViewport()
		out := m2.View()
		outLines := len(strings.Split(out, "\n"))
		if outLines > size.h {
			t.Errorf("OVERFLOW at %dx%d (masked): %d lines (excess: %d)", size.w, size.h, outLines, outLines-size.h)
		}
	}
}
