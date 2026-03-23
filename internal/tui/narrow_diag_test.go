package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// TestDiag_NarrowTerminal_DetailedLineAnalysis examines every line of
// the rendered detail screen at width=40 to find which lines overflow.
func TestDiag_NarrowTerminal_DetailedLineAnalysis(t *testing.T) {
	const termH, termW = 40, 40

	// PEM cert with 64-char lines
	certLine := strings.Repeat("M", 64)
	pemLines := make([]string, 30)
	for i := range pemLines {
		pemLines[i] = certLine
	}
	certValue := "-----BEGIN CERTIFICATE-----\n" + strings.Join(pemLines, "\n") + "\n-----END CERTIFICATE-----"

	entry := vault.SecretEntry{
		Name:        "narrow-cert",
		Environment: "prod",
		Value:       certValue,
		Metadata:    map[string]any{"type": "x509"},
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-01T00:00:00Z",
	}

	m := Model{
		width:       termW,
		height:      termH,
		screen:      screenDetail,
		secrets:     []vault.SecretEntry{entry},
		display:     []vault.SecretEntry{entry},
		cursor:      0,
		valueMasked: false,
	}

	rendered := m.viewDetailScreen()
	rawLinesList := strings.Split(rendered, "\n")

	totalVisual := 0
	for i, line := range rawLinesList {
		plain := stripANSI(line)
		runeW := utf8.RuneCountInString(plain)
		visualForLine := 1
		if runeW > termW {
			visualForLine = (runeW + termW - 1) / termW
		}
		totalVisual += visualForLine

		if runeW > termW {
			t.Logf("OVERFLOW LINE %2d: %3d runes (wraps to %d visual): %.60s...",
				i, runeW, visualForLine, plain)
		} else {
			t.Logf("        LINE %2d: %3d runes: %.60s", i, runeW, plain)
		}
	}

	t.Logf("")
	t.Logf("Total raw lines: %d", len(rawLinesList))
	t.Logf("Total visual lines: %d (terminal height: %d)", totalVisual, termH)
	t.Logf("Overflow: %v (excess: %d)", totalVisual > termH, totalVisual-termH)
}

// TestDiag_NarrowTerminal_HelpBarWrap tests whether the help bar
// is the source of overflow by checking its width at various terminal widths.
func TestDiag_NarrowTerminal_HelpBarWrap(t *testing.T) {
	// The help bar in the detail view footer
	m := Model{width: 40, height: 40}

	// Non-vim help bar (detail screen)
	helpBarContent := m.helpBar(
		"esc", "back",
		"space", "peek",
		"c", "copy",
		"e", "edit",
		"d", "delete",
		"q", "quit",
	)
	plain := stripANSI(helpBarContent)
	runeW := utf8.RuneCountInString(plain)

	t.Logf("Detail help bar content: '%s'", plain)
	t.Logf("Detail help bar width: %d runes", runeW)

	// Check at various widths
	for _, w := range []int{40, 50, 60, 70, 80} {
		wraps := runeW > w
		extraLines := 0
		if wraps {
			extraLines = (runeW+w-1)/w - 1
		}
		t.Logf("  Width %d: wraps=%v, extra_lines=%d", w, wraps, extraLines)
	}

	// Vim mode detail help bar
	m.vimEnabled = true
	m.screen = screenDetail
	vimHelp := m.vimHelpBar()
	vimPlain := stripANSI(vimHelp)
	vimW := utf8.RuneCountInString(vimPlain)
	t.Logf("Vim detail help bar width: %d runes", vimW)
}

// TestDiag_NarrowTerminal_ListScreen tests the list screen at narrow widths.
func TestDiag_NarrowTerminal_ListScreen(t *testing.T) {
	const termH, termW = 40, 40

	entries := []vault.SecretEntry{
		{Name: "secret-1", Value: "val1", UpdatedAt: "2025-01-01T00:00:00Z"},
		{Name: "secret-2", Value: "val2", UpdatedAt: "2025-01-01T00:00:00Z"},
	}

	m := Model{
		width:   termW,
		height:  termH,
		screen:  screenList,
		secrets: entries,
		display: entries,
		cursor:  0,
	}

	rendered := m.viewListScreen()
	rawLinesList := strings.Split(rendered, "\n")

	totalVisual := 0
	for i, line := range rawLinesList {
		plain := stripANSI(line)
		runeW := utf8.RuneCountInString(plain)
		visualForLine := 1
		if runeW > termW {
			visualForLine = (runeW + termW - 1) / termW
			t.Logf("OVERFLOW LINE %2d: %3d runes (wraps to %d): %.60s", i, runeW, visualForLine, plain)
		}
		totalVisual += visualForLine
	}

	t.Logf("List screen: %d raw lines, %d visual lines at width %d", len(rawLinesList), totalVisual, termW)

	if totalVisual > termH {
		t.Errorf("List screen VISUAL OVERFLOW: %d visual lines > %d", totalVisual, termH)
	}
}
