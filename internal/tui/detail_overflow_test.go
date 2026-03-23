package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// ── Diagnostic tests for Issue #15 ───────────────────────────
//
// These tests trace the EXACT dimensions and arithmetic used by
// viewDetailScreen() to expose the root cause of terminal overflow.

// TestDiag_A_ZeroUnsetDimensions tests what happens when Model has
// height=0, width=0 (simulating no WindowSizeMsg received yet).
func TestDiag_A_ZeroUnsetDimensions(t *testing.T) {
	// 68-line PEM cert
	certValue := largeSecret(68)

	entry := vault.SecretEntry{
		Name:      "tls-cert",
		Value:     certValue,
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
	}

	m := Model{
		width:       0, // NO WindowSizeMsg received!
		height:      0,
		screen:      screenDetail,
		secrets:     []vault.SecretEntry{entry},
		display:     []vault.SecretEntry{entry},
		cursor:      0,
		valueMasked: false, // revealed
	}

	rendered := m.viewDetailScreen()
	rawLines := strings.Count(rendered, "\n") + 1

	// When height=0, viewDetailScreen falls back to h=24, w=80.
	// The output should fit in 24 lines.
	t.Logf("m.height=%d, m.width=%d", m.height, m.width)
	t.Logf("Rendered lines (raw): %d", rawLines)
	t.Logf("Fallback height used: 24, fallback width: 80")

	if rawLines > 24 {
		t.Errorf("OVERFLOW: %d raw lines exceeds fallback height 24 (excess: %d)", rawLines, rawLines-24)
	}

	// Now check VISUAL lines — lines that wrap at width 80
	visualLines := countVisualLines(rendered, 80)
	t.Logf("Visual lines at width 80: %d", visualLines)

	if visualLines > 24 {
		t.Errorf("VISUAL OVERFLOW: %d visual lines exceeds fallback height 24", visualLines)
	}
}

// TestDiag_B_TinyTerminal tests with height=15, width=60.
// A 68-line PEM cert revealed. Should fit in 15 lines.
func TestDiag_B_TinyTerminal(t *testing.T) {
	const termH, termW = 15, 60

	certValue := largeSecret(68)

	entry := vault.SecretEntry{
		Name:      "tls-cert",
		Value:     certValue,
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
	}

	m := Model{
		width:       termW,
		height:      termH,
		screen:      screenDetail,
		secrets:     []vault.SecretEntry{entry},
		display:     []vault.SecretEntry{entry},
		cursor:      0,
		valueMasked: false, // revealed
	}

	rendered := m.viewDetailScreen()
	rawLines := strings.Count(rendered, "\n") + 1

	t.Logf("m.height=%d, m.width=%d", m.height, m.width)
	t.Logf("Rendered lines (raw): %d", rawLines)

	if rawLines > termH {
		t.Errorf("OVERFLOW: %d raw lines exceeds terminal height %d (excess: %d)",
			rawLines, termH, rawLines-termH)
	}

	// Visual line check
	visualLines := countVisualLines(rendered, termW)
	t.Logf("Visual lines at width %d: %d", termW, visualLines)

	if visualLines > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines exceeds terminal height %d", visualLines, termH)
	}
}

// TestDiag_C_SingleLongLine tests with height=24, width=80.
// A 2089-char paragraph (0 newlines). Should fit in 24 lines.
func TestDiag_C_SingleLongLine(t *testing.T) {
	const termH, termW = 24, 80

	longValue := strings.Repeat("A", 2089)

	entry := vault.SecretEntry{
		Name:      "api-key",
		Value:     longValue,
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
	}

	m := Model{
		width:       termW,
		height:      termH,
		screen:      screenDetail,
		secrets:     []vault.SecretEntry{entry},
		display:     []vault.SecretEntry{entry},
		cursor:      0,
		valueMasked: false, // revealed
	}

	rendered := m.viewDetailScreen()
	rawLines := strings.Count(rendered, "\n") + 1

	t.Logf("m.height=%d, m.width=%d", m.height, m.width)
	t.Logf("Rendered lines (raw): %d", rawLines)

	if rawLines > termH {
		t.Errorf("OVERFLOW: %d raw lines exceeds terminal height %d", rawLines, termH)
	}

	// Visual line check — THIS is the critical one for long lines
	visualLines := countVisualLines(rendered, termW)
	t.Logf("Visual lines at width %d: %d", termW, visualLines)

	if visualLines > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines exceeds terminal height %d", visualLines, termH)
	}
}

// TestDiag_D_NarrowTerminal tests with height=40, width=40 (narrow terminal).
// A PEM cert with 64-char lines needs wrapping at width 40.
func TestDiag_D_NarrowTerminal(t *testing.T) {
	const termH, termW = 40, 40

	// Real PEM-like lines: 64 chars per line
	certLine := strings.Repeat("M", 64)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = certLine
	}
	certValue := "-----BEGIN CERTIFICATE-----\n" + strings.Join(lines, "\n") + "\n-----END CERTIFICATE-----"

	entry := vault.SecretEntry{
		Name:      "narrow-cert",
		Value:     certValue,
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
	}

	m := Model{
		width:       termW,
		height:      termH,
		screen:      screenDetail,
		secrets:     []vault.SecretEntry{entry},
		display:     []vault.SecretEntry{entry},
		cursor:      0,
		valueMasked: false, // revealed
	}

	rendered := m.viewDetailScreen()
	rawLines := strings.Count(rendered, "\n") + 1

	t.Logf("m.height=%d, m.width=%d", m.height, m.width)
	t.Logf("Rendered lines (raw): %d", rawLines)

	if rawLines > termH {
		t.Errorf("OVERFLOW: %d raw lines exceeds terminal height %d", rawLines, termH)
	}

	// Visual line check — lines wider than 40 cols will wrap
	visualLines := countVisualLines(rendered, termW)
	t.Logf("Visual lines at width %d: %d", termW, visualLines)

	if visualLines > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines exceeds terminal height %d (excess: %d)",
			visualLines, termH, visualLines-termH)
	}

	// Also check the max raw line width
	maxW := maxVisualLineWidth(rendered)
	t.Logf("Max visual line width (runes): %d", maxW)

	if maxW > termW {
		t.Errorf("LINE TOO WIDE: max line width %d exceeds terminal width %d", maxW, termW)
	}
}

// TestDiag_E_DetailScreenMathTrace traces the exact arithmetic used
// in viewDetailScreen() for a 24-row terminal.
func TestDiag_E_DetailScreenMathTrace(t *testing.T) {
	const termH, termW = 24, 80

	certValue := largeSecret(68)

	entry := vault.SecretEntry{
		Name:        "tls-cert",
		Environment: "prod",
		Value:       certValue,
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
		valueMasked: false, // revealed
	}

	// Manually trace the math from viewDetailScreen():

	// Header
	var header strings.Builder
	header.WriteString(TitleStyle.Render("  " + entry.Name))
	header.WriteString("  " + EnvBadgeStyle.Render("["+entry.Environment+"]"))
	header.WriteString("\n")
	header.WriteString(dividerLine(termW))
	headerStr := header.String()
	headerLines := strings.Count(headerStr, "\n") + 1

	// Footer
	var footer strings.Builder
	footer.WriteString(dividerLine(termW))
	footer.WriteString("\n")
	footer.WriteString(m.helpBar("esc", "back", "space", "peek", "c", "copy", "e", "edit", "d", "delete", "q", "quit"))
	footerStr := footer.String()
	footerLines := strings.Count(footerStr, "\n") + 1

	// Available height
	availableHeight := termH - headerLines - footerLines - 2
	if availableHeight < 3 {
		availableHeight = 3
	}

	t.Logf("=== Math Trace for %d-row terminal ===", termH)
	t.Logf("headerLines:     %d", headerLines)
	t.Logf("footerLines:     %d", footerLines)
	t.Logf("blank lines:     2 (between header/body and body/footer)")
	t.Logf("availableHeight: %d - %d - %d - 2 = %d", termH, headerLines, footerLines, availableHeight)

	// Now render and check
	rendered := m.viewDetailScreen()
	rawLines := strings.Count(rendered, "\n") + 1
	visualLines := countVisualLines(rendered, termW)

	t.Logf("Rendered raw lines: %d", rawLines)
	t.Logf("Rendered visual lines at width %d: %d", termW, visualLines)
	t.Logf("Fits in terminal (raw): %v", rawLines <= termH)
	t.Logf("Fits in terminal (visual): %v", visualLines <= termH)

	// Check the body itself
	// Build body to see how many lines the value generates
	contentWidth := termW - 4
	wrapped, totalWrapped := wrapAndTruncate(certValue, contentWidth, 0)
	wrappedLines := strings.Count(wrapped, "\n") + 1
	t.Logf("Value: %d chars, %d original lines", len(certValue), strings.Count(certValue, "\n")+1)
	t.Logf("After wrapAndTruncate(width=%d): %d wrapped lines (total=%d)", contentWidth, wrappedLines, totalWrapped)
	t.Logf("viewportRender will crop to %d lines", availableHeight)

	if rawLines > termH {
		t.Errorf("OVERFLOW: %d raw lines > %d terminal height", rawLines, termH)
	}
	if visualLines > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines > %d terminal height", visualLines, termH)
	}
}

// TestDiag_F_DetailScreen_DividerLineWidth checks if dividerLine()
// produces a line that exceeds the terminal width, causing wrapping.
func TestDiag_F_DetailScreen_DividerLineWidth(t *testing.T) {
	widths := []int{40, 60, 80, 120}

	for _, w := range widths {
		t.Run(strings.Repeat("w", 1)+string(rune('0'+w/10))+string(rune('0'+w%10)), func(t *testing.T) {
			div := dividerLine(w)
			plain := stripANSI(div)
			runeCount := utf8.RuneCountInString(plain)

			t.Logf("dividerLine(%d): %d runes, raw bytes: %d", w, runeCount, len(div))

			if runeCount > w {
				t.Errorf("dividerLine(%d) produces %d runes, exceeds width", w, runeCount)
			}
		})
	}
}

// TestDiag_G_HelpBarWidth checks if helpBar() output exceeds terminal width
// at various widths, including narrow terminals where truncation kicks in.
func TestDiag_G_HelpBarWidth(t *testing.T) {
	helpPairs := []string{
		"esc", "back",
		"space", "peek",
		"c", "copy",
		"e", "edit",
		"d", "delete",
		"q", "quit",
	}

	for _, w := range []int{40, 60, 80, 120} {
		t.Run(fmt.Sprintf("width_%d", w), func(t *testing.T) {
			m := Model{width: w, height: 24}
			bar := m.helpBar(helpPairs...)
			plain := stripANSI(bar)
			runeCount := utf8.RuneCountInString(plain)

			t.Logf("Help bar at width %d: %d runes, content: '%s'", w, runeCount, plain)

			if runeCount > w {
				t.Errorf("Help bar %d runes exceeds terminal width %d — would cause wrapping!", runeCount, w)
			}
		})
	}
}

// TestDiag_H_VisualLineOverflow_EveryLine examines each line of the
// rendered detail screen and reports which lines exceed terminal width,
// causing visual wrapping.
func TestDiag_H_VisualLineOverflow_EveryLine(t *testing.T) {
	const termH, termW = 24, 80

	certValue := largeSecret(68)

	entry := vault.SecretEntry{
		Name:        "tls-cert",
		Environment: "prod",
		Value:       certValue,
		Metadata:    map[string]any{"type": "x509", "issuer": "Let's Encrypt"},
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

	overflowCount := 0
	totalVisual := 0

	for i, line := range rawLinesList {
		plain := stripANSI(line)
		runeW := utf8.RuneCountInString(plain)
		visualForLine := 1
		if runeW > termW {
			visualForLine = (runeW + termW - 1) / termW
			overflowCount++
			t.Logf("LINE %d: %d runes > %d width (wraps to %d visual lines): %s",
				i, runeW, termW, visualForLine, truncateStr(plain, 100))
		}
		totalVisual += visualForLine
	}

	t.Logf("Total raw lines: %d", len(rawLinesList))
	t.Logf("Total visual lines: %d", totalVisual)
	t.Logf("Lines that overflow width: %d", overflowCount)

	if totalVisual > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines > %d terminal height", totalVisual, termH)
	}
}

// ── Helpers ───────────────────────────────────────────────────

// countVisualLines counts how many visual terminal rows the rendered output
// would occupy at a given terminal width, accounting for line wrapping.
func countVisualLines(rendered string, termWidth int) int {
	rawLines := strings.Split(rendered, "\n")
	total := 0
	for _, line := range rawLines {
		plain := stripANSI(line)
		w := utf8.RuneCountInString(plain)
		if w <= termWidth || termWidth <= 0 {
			total++
		} else {
			total += (w + termWidth - 1) / termWidth
		}
	}
	return total
}

// maxVisualLineWidth returns the widest line in runes after stripping ANSI.
func maxVisualLineWidth(rendered string) int {
	lines := strings.Split(rendered, "\n")
	max := 0
	for _, l := range lines {
		plain := stripANSI(l)
		w := utf8.RuneCountInString(plain)
		if w > max {
			max = w
		}
	}
	return max
}

// truncateStr truncates a string for display purposes.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ── viewportRender off-by-one regression tests ───────────────
//
// These tests verify that viewportRender ALWAYS produces exactly
// maxVisible lines, regardless of scroll position and indicator
// combination.

// TestViewportRender_BothIndicators_ExactLineCount verifies that when
// both ▲ and ▼ scroll indicators are shown, the output is exactly
// maxVisible lines (not maxVisible+1, which was the original bug).
func TestViewportRender_BothIndicators_ExactLineCount(t *testing.T) {
	cases := []struct {
		name         string
		totalLines   int
		maxVisible   int
		scrollOffset int
	}{
		{"20 lines, view 10, scroll 5", 20, 10, 5},
		{"50 lines, view 15, scroll 10", 50, 15, 10},
		{"100 lines, view 20, scroll 50", 100, 20, 50},
		{"30 lines, view 8, scroll 1", 30, 8, 1},
		{"30 lines, view 8, scroll 20", 30, 8, 20},
		{"15 lines, view 5, scroll 3", 15, 5, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build content with tc.totalLines lines.
			lines := make([]string, tc.totalLines)
			for i := range lines {
				lines[i] = fmt.Sprintf("line-%03d", i)
			}
			content := strings.Join(lines, "\n")

			result, offset := viewportRender(content, tc.maxVisible, tc.scrollOffset, -1)
			outputLines := strings.Count(result, "\n") + 1

			// Both indicators should be present.
			hasUp := strings.Contains(result, "▲")
			hasDown := strings.Contains(result, "▼")

			t.Logf("offset=%d, outputLines=%d, maxVisible=%d, hasUp=%v, hasDown=%v",
				offset, outputLines, tc.maxVisible, hasUp, hasDown)

			if !hasUp {
				t.Errorf("expected ▲ indicator (scrollOffset=%d)", offset)
			}
			if !hasDown {
				t.Errorf("expected ▼ indicator")
			}
			if outputLines != tc.maxVisible {
				t.Errorf("output has %d lines, want exactly %d (off by %d)",
					outputLines, tc.maxVisible, outputLines-tc.maxVisible)
			}
		})
	}
}

// TestViewportRender_OnlyTopIndicator verifies line count with only ▲.
func TestViewportRender_OnlyTopIndicator(t *testing.T) {
	// 15 lines, view 10, scroll to the end (offset = 5).
	// At the end: ▲ shown, no ▼.
	lines := make([]string, 15)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i)
	}
	content := strings.Join(lines, "\n")

	result, _ := viewportRender(content, 10, 999, -1) // 999 will be clamped to 5
	outputLines := strings.Count(result, "\n") + 1

	hasUp := strings.Contains(result, "▲")
	hasDown := strings.Contains(result, "▼")

	if !hasUp {
		t.Error("expected ▲ indicator")
	}
	if hasDown {
		t.Error("expected no ▼ indicator (scrolled to bottom)")
	}
	if outputLines != 10 {
		t.Errorf("output has %d lines, want exactly 10", outputLines)
	}
}

// TestViewportRender_OnlyBottomIndicator verifies line count with only ▼.
func TestViewportRender_OnlyBottomIndicator(t *testing.T) {
	// 15 lines, view 10, scroll 0: no ▲, ▼ shown.
	lines := make([]string, 15)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i)
	}
	content := strings.Join(lines, "\n")

	result, _ := viewportRender(content, 10, 0, -1)
	outputLines := strings.Count(result, "\n") + 1

	hasUp := strings.Contains(result, "▲")
	hasDown := strings.Contains(result, "▼")

	if hasUp {
		t.Error("expected no ▲ indicator (scrollOffset=0)")
	}
	if !hasDown {
		t.Error("expected ▼ indicator")
	}
	if outputLines != 10 {
		t.Errorf("output has %d lines, want exactly 10", outputLines)
	}
}

// TestViewportRender_NoIndicators verifies line count when content fits.
func TestViewportRender_NoIndicators(t *testing.T) {
	lines := make([]string, 5)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}
	content := strings.Join(lines, "\n")

	result, _ := viewportRender(content, 10, 0, -1)
	outputLines := strings.Count(result, "\n") + 1

	if outputLines != 5 {
		t.Errorf("output has %d lines, want 5 (content fits, no cropping)", outputLines)
	}
}

// TestViewportRender_MaxVisibleZeroOrNegative verifies the guard for
// maxVisible <= 0 (previously could panic).
func TestViewportRender_MaxVisibleZeroOrNegative(t *testing.T) {
	content := "line-0\nline-1\nline-2"

	for _, maxVis := range []int{0, -1, -5} {
		t.Run(fmt.Sprintf("maxVisible=%d", maxVis), func(t *testing.T) {
			// Should not panic.
			result, _ := viewportRender(content, maxVis, 0, -1)
			outputLines := strings.Count(result, "\n") + 1

			// maxVisible is clamped to 1, so output should be 1 line.
			if outputLines != 1 {
				t.Errorf("output has %d lines, want 1 (maxVisible clamped to 1)", outputLines)
			}
		})
	}
}

// ── Detail screen overflow with scrolling ─────────────────────
//
// These tests exercise viewDetailScreen with detailScroll > 0
// to confirm the fix prevents overflow when both scroll indicators
// appear.

// TestDetailScreen_ScrolledDown_PEM_NoOverflow verifies that scrolling
// down through a 68-line PEM cert at height=24 never overflows.
func TestDetailScreen_ScrolledDown_PEM_NoOverflow(t *testing.T) {
	const termH, termW = 24, 80

	certValue := largeSecret(68)

	entry := vault.SecretEntry{
		Name:        "tls-cert",
		Environment: "prod",
		Value:       certValue,
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-01T00:00:00Z",
	}

	// Test at various scroll offsets including the middle where both
	// ▲ and ▼ indicators appear.
	for _, scroll := range []int{0, 1, 5, 10, 20, 50, 100} {
		t.Run(fmt.Sprintf("scroll_%d", scroll), func(t *testing.T) {
			m := Model{
				width:        termW,
				height:       termH,
				screen:       screenDetail,
				secrets:      []vault.SecretEntry{entry},
				display:      []vault.SecretEntry{entry},
				cursor:       0,
				valueMasked:  false,
				detailScroll: scroll,
			}

			rendered := m.viewDetailScreen()
			rawLines := strings.Count(rendered, "\n") + 1
			visualLines := countVisualLines(rendered, termW)

			t.Logf("scroll=%d: raw=%d, visual=%d (max=%d)", scroll, rawLines, visualLines, termH)

			if rawLines > termH {
				t.Errorf("RAW OVERFLOW at scroll=%d: %d lines > %d", scroll, rawLines, termH)
			}
			if visualLines > termH {
				t.Errorf("VISUAL OVERFLOW at scroll=%d: %d lines > %d", scroll, visualLines, termH)
			}
		})
	}
}

// TestDetailScreen_ScrolledDown_LongLine_NoOverflow verifies that
// scrolling down through a 2089-char single line at height=24 never
// overflows.
func TestDetailScreen_ScrolledDown_LongLine_NoOverflow(t *testing.T) {
	const termH, termW = 24, 80

	longValue := strings.Repeat("A", 2089)

	entry := vault.SecretEntry{
		Name:      "api-key",
		Value:     longValue,
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
	}

	for _, scroll := range []int{0, 1, 5, 10, 15, 20, 50} {
		t.Run(fmt.Sprintf("scroll_%d", scroll), func(t *testing.T) {
			m := Model{
				width:        termW,
				height:       termH,
				screen:       screenDetail,
				secrets:      []vault.SecretEntry{entry},
				display:      []vault.SecretEntry{entry},
				cursor:       0,
				valueMasked:  false,
				detailScroll: scroll,
			}

			rendered := m.viewDetailScreen()
			rawLines := strings.Count(rendered, "\n") + 1
			visualLines := countVisualLines(rendered, termW)

			t.Logf("scroll=%d: raw=%d, visual=%d (max=%d)", scroll, rawLines, visualLines, termH)

			if rawLines > termH {
				t.Errorf("RAW OVERFLOW at scroll=%d: %d lines > %d", scroll, rawLines, termH)
			}
			if visualLines > termH {
				t.Errorf("VISUAL OVERFLOW at scroll=%d: %d lines > %d", scroll, visualLines, termH)
			}
		})
	}
}

// TestDetailScreen_TinyTerminal_ScrolledDown verifies no overflow at
// height=15 with scrolling active.
func TestDetailScreen_TinyTerminal_ScrolledDown(t *testing.T) {
	const termH, termW = 15, 60

	certValue := largeSecret(68)

	entry := vault.SecretEntry{
		Name:      "tls-cert",
		Value:     certValue,
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:00:00Z",
	}

	for _, scroll := range []int{0, 1, 3, 5, 10, 30, 60} {
		t.Run(fmt.Sprintf("scroll_%d", scroll), func(t *testing.T) {
			m := Model{
				width:        termW,
				height:       termH,
				screen:       screenDetail,
				secrets:      []vault.SecretEntry{entry},
				display:      []vault.SecretEntry{entry},
				cursor:       0,
				valueMasked:  false,
				detailScroll: scroll,
			}

			rendered := m.viewDetailScreen()
			rawLines := strings.Count(rendered, "\n") + 1
			visualLines := countVisualLines(rendered, termW)

			t.Logf("scroll=%d: raw=%d, visual=%d (max=%d)", scroll, rawLines, visualLines, termH)

			if rawLines > termH {
				t.Errorf("RAW OVERFLOW: %d > %d", rawLines, termH)
			}
			if visualLines > termH {
				t.Errorf("VISUAL OVERFLOW: %d > %d", visualLines, termH)
			}
		})
	}
}

// ── wrapAllLines tests ───────────────────────────────────────

// TestWrapAllLines_Basic verifies that wrapAllLines correctly splits
// long lines at the maxWidth boundary while preserving short lines.
func TestWrapAllLines_Basic(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		maxWidth int
		wantMax  int // max visual width of any output line
	}{
		{
			"short line unchanged",
			"hello world",
			80,
			11,
		},
		{
			"exact width unchanged",
			strings.Repeat("A", 80),
			80,
			80,
		},
		{
			"long line wrapped",
			strings.Repeat("B", 150),
			80,
			80,
		},
		{
			"multi-line mixed",
			"short\n" + strings.Repeat("C", 200) + "\nalso short",
			80,
			80,
		},
		{
			"empty lines preserved",
			"hello\n\nworld",
			80,
			5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := wrapAllLines(tc.input, tc.maxWidth)
			for i, line := range strings.Split(result, "\n") {
				plain := stripANSI(line)
				w := utf8.RuneCountInString(plain)
				if w > tc.maxWidth {
					t.Errorf("line %d: visual width %d exceeds maxWidth %d: %s",
						i, w, tc.maxWidth, truncateStr(plain, 60))
				}
			}
		})
	}
}

// TestDetailScreen_NonLargeValue_150Chars_Width80 is the specific test
// required by Issue #15: a 150-character secret value (no newlines,
// under largeValueThreshold) on an 80-column terminal. Before the fix,
// the line "  Value: " + 150 chars = 159 runes would wrap to 2 visual
// lines but viewportRender counted it as 1, causing overflow.
func TestDetailScreen_NonLargeValue_150Chars_Width80(t *testing.T) {
	const termH, termW = 24, 80

	// 150-char value — NOT large (< largeValueThreshold=200), no newlines.
	value150 := strings.Repeat("X", 150)
	if isLargeValue(value150) {
		t.Fatalf("150-char value should NOT be classified as large (threshold=%d)", largeValueThreshold)
	}

	entry := vault.SecretEntry{
		Name:      "api-key-medium",
		Value:     value150,
		CreatedAt: "2025-01-15T10:30:00Z",
		UpdatedAt: "2025-01-15T10:30:00Z",
	}

	m := Model{
		width:       termW,
		height:      termH,
		screen:      screenDetail,
		secrets:     []vault.SecretEntry{entry},
		display:     []vault.SecretEntry{entry},
		cursor:      0,
		valueMasked: false, // value revealed
	}

	rendered := m.viewDetailScreen()
	rawLinesList := strings.Split(rendered, "\n")
	rawLines := len(rawLinesList)

	// Assertion 1: No line has visual width > 80.
	for i, line := range rawLinesList {
		plain := stripANSI(line)
		w := utf8.RuneCountInString(plain)
		if w > termW {
			t.Errorf("LINE %d: visual width %d exceeds terminal width %d: %.80s...",
				i, w, termW, plain)
		}
	}

	// Assertion 2: Total visual lines <= m.height.
	visualLines := countVisualLines(rendered, termW)
	t.Logf("150-char value at %dx%d: raw=%d, visual=%d", termW, termH, rawLines, visualLines)

	if visualLines > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines exceeds terminal height %d", visualLines, termH)
	}

	// Assertion 3: Raw line count <= m.height (since we pre-wrap, raw==visual).
	if rawLines > termH {
		t.Errorf("RAW OVERFLOW: %d raw lines exceeds terminal height %d", rawLines, termH)
	}
}

// TestDetailScreen_NonLargeValue_WithMetadata_Width80 tests wrapping
// of metadata lines and non-large field values that can also exceed
// terminal width.
func TestDetailScreen_NonLargeValue_WithMetadata_Width80(t *testing.T) {
	const termH, termW = 24, 80

	// 150-char value (non-large) + long metadata + fields
	value := strings.Repeat("V", 150)

	entry := vault.SecretEntry{
		Name:        "complex-secret",
		Environment: "production",
		Value:       value,
		Metadata: map[string]any{
			"description": "This is a very long description that goes on and on and might exceed terminal width easily",
			"team":        "platform-engineering",
			"region":      "us-west-2",
		},
		Fields: map[string]string{
			"username": strings.Repeat("U", 100), // 100-char username, "    username: " + 100 = 114 chars
			"host":     "short",
		},
		CreatedAt: "2025-01-15T10:30:00Z",
		UpdatedAt: "2025-01-15T10:30:00Z",
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

	overflowCount := 0
	for i, line := range rawLinesList {
		plain := stripANSI(line)
		w := utf8.RuneCountInString(plain)
		if w > termW {
			overflowCount++
			t.Errorf("LINE %d: visual width %d exceeds terminal width %d: %.80s...",
				i, w, termW, plain)
		}
	}

	visualLines := countVisualLines(rendered, termW)
	t.Logf("Complex secret at %dx%d: raw=%d, visual=%d, overflow_lines=%d",
		termW, termH, len(rawLinesList), visualLines, overflowCount)

	if visualLines > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines exceeds terminal height %d", visualLines, termH)
	}
}

// TestDetailScreen_NarrowTerminal_NonLargeValue tests a non-large value
// on a very narrow terminal (width=40) where even moderate-length values
// would wrap without pre-wrapping.
func TestDetailScreen_NarrowTerminal_NonLargeValue(t *testing.T) {
	const termH, termW = 30, 40

	// 80-char value — not large, but exceeds width=40 with "  Value: " prefix (89 chars).
	value := strings.Repeat("N", 80)

	entry := vault.SecretEntry{
		Name:      "narrow-test",
		Value:     value,
		CreatedAt: "2025-01-15T10:30:00Z",
		UpdatedAt: "2025-01-15T10:30:00Z",
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

	for i, line := range rawLinesList {
		plain := stripANSI(line)
		w := utf8.RuneCountInString(plain)
		if w > termW {
			t.Errorf("LINE %d: visual width %d exceeds terminal width %d: %.40s...",
				i, w, termW, plain)
		}
	}

	visualLines := countVisualLines(rendered, termW)
	t.Logf("Non-large value at %dx%d: raw=%d, visual=%d", termW, termH, len(rawLinesList), visualLines)

	if visualLines > termH {
		t.Errorf("VISUAL OVERFLOW: %d visual lines exceeds terminal height %d", visualLines, termH)
	}
}
