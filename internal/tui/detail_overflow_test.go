package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rmkohlman/MaestroVault/internal/vault"
)

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

// ── truncateToHeight unit tests ──────────────────────────────

// TestTruncateToHeight_Unit tests the truncateToHeight function directly
// for various edge cases and normal operation.
func TestTruncateToHeight_Unit(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		maxHeight int
		termWidth int
		wantMax   int // visual line count of result must be <= this
	}{
		{
			name:      "empty string",
			content:   "",
			maxHeight: 10,
			termWidth: 80,
			wantMax:   0,
		},
		{
			name:      "content fits",
			content:   "line1\nline2\nline3",
			maxHeight: 10,
			termWidth: 80,
			wantMax:   3,
		},
		{
			name:      "content exceeds maxHeight",
			content:   "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10",
			maxHeight: 5,
			termWidth: 80,
			wantMax:   5,
		},
		{
			name:      "wrapping line counted correctly",
			content:   strings.Repeat("X", 200) + "\nshort",
			maxHeight: 3,
			termWidth: 80,
			wantMax:   3,
		},
		{
			name:      "wrapping line too tall for maxHeight",
			content:   strings.Repeat("Y", 500),
			maxHeight: 3,
			termWidth: 80,
			wantMax:   0, // single line wraps to >3 rows, dropped
		},
		{
			name:      "exact fit",
			content:   "a\nb\nc\nd\ne",
			maxHeight: 5,
			termWidth: 80,
			wantMax:   5,
		},
		{
			name:      "maxHeight zero",
			content:   "hello",
			maxHeight: 0,
			termWidth: 80,
			wantMax:   1, // returned unchanged when maxHeight <= 0
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := truncateToHeight(tc.content, tc.maxHeight, tc.termWidth)

			if result == "" {
				if tc.wantMax != 0 && tc.content != "" {
					t.Errorf("unexpected empty result")
				}
				return
			}

			visualLines := countVisualLines(result, tc.termWidth)
			t.Logf("visual lines: %d, wantMax: %d", visualLines, tc.wantMax)

			if visualLines > tc.wantMax {
				t.Errorf("truncateToHeight produced %d visual lines, want <= %d",
					visualLines, tc.wantMax)
			}

			// Also verify it doesn't exceed maxHeight when maxHeight > 0
			if tc.maxHeight > 0 && visualLines > tc.maxHeight {
				t.Errorf("truncateToHeight produced %d visual lines, exceeds maxHeight %d",
					visualLines, tc.maxHeight)
			}
		})
	}
}

// ── View() safety net integration tests ──────────────────────

// TestView_SafetyNet_DetailOverflow exercises View() (not viewDetailScreen)
// with a secret that would overflow, verifying the final output's visual
// line count == m.height. This proves the truncateToHeight safety net works
// end-to-end through the full View() call path.
func TestView_SafetyNet_DetailOverflow(t *testing.T) {
	cases := []struct {
		name  string
		termH int
		termW int
		value string
		desc  string
	}{
		{
			name:  "68-line PEM on 15-row terminal",
			termH: 15,
			termW: 60,
			value: largeSecret(68),
			desc:  "PEM cert with 68 lines should be truncated to 15 rows",
		},
		{
			name:  "2089-char blob on 24-row terminal",
			termH: 24,
			termW: 80,
			value: strings.Repeat("A", 2089),
			desc:  "Long single-line value should fit in 24 rows",
		},
		{
			name:  "100-line secret on 20-row terminal",
			termH: 20,
			termW: 80,
			value: largeSecret(100),
			desc:  "100 lines must be truncated to fit in 20 rows",
		},
		{
			name:  "PEM on tiny 10-row terminal",
			termH: 10,
			termW: 40,
			value: largeSecret(50),
			desc:  "Extreme: 50 lines on 10x40 terminal",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := vault.SecretEntry{
				Name:      "test-secret",
				Value:     tc.value,
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-01T00:00:00Z",
			}

			m := Model{
				width:        tc.termW,
				height:       tc.termH,
				sizeReceived: true, // Required for View() to render
				screen:       screenDetail,
				secrets:      []vault.SecretEntry{entry},
				display:      []vault.SecretEntry{entry},
				cursor:       0,
				valueMasked:  false, // reveal value to force overflow
			}

			rendered := m.View()

			// Skip empty renders (quitting, no size, etc.)
			if rendered == "" {
				t.Skip("View() returned empty — nothing to verify")
			}

			visualLines := countVisualLines(rendered, tc.termW)
			t.Logf("%s: visual=%d, maxHeight=%d", tc.desc, visualLines, tc.termH)

			if visualLines > tc.termH {
				t.Errorf("SAFETY NET FAILED: View() produced %d visual lines, exceeds terminal height %d (excess: %d)",
					visualLines, tc.termH, visualLines-tc.termH)
			}
		})
	}
}

// TestView_SafetyNet_OverlayOverflow verifies that the safety net catches
// overflow from overlay screens (help, generator, info, settings) which
// go through centerOverlay → lipgloss.Place that doesn't truncate.
func TestView_SafetyNet_OverlayOverflow(t *testing.T) {
	// A small terminal where overlays could potentially overflow.
	const termH, termW = 15, 50

	overlays := []struct {
		name      string
		setupFunc func(m *Model)
	}{
		{
			name: "help overlay",
			setupFunc: func(m *Model) {
				m.showHelp = true
			},
		},
		{
			name: "info overlay",
			setupFunc: func(m *Model) {
				m.showInfo = true
				m.vaultInfo = &vault.VaultInfo{
					Dir:         "/tmp/test",
					DBPath:      "/tmp/test/vault.db",
					DBSize:      1024,
					SecretCount: 5,
				}
			},
		},
		{
			name: "settings overlay",
			setupFunc: func(m *Model) {
				m.showSettings = true
			},
		},
	}

	for _, tc := range overlays {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{
				width:        termW,
				height:       termH,
				sizeReceived: true,
				screen:       screenList,
			}
			tc.setupFunc(&m)

			rendered := m.View()
			if rendered == "" {
				t.Skip("View() returned empty")
			}

			visualLines := countVisualLines(rendered, termW)
			t.Logf("%s at %dx%d: visual=%d", tc.name, termW, termH, visualLines)

			if visualLines > termH {
				t.Errorf("SAFETY NET FAILED: %s produced %d visual lines, exceeds terminal height %d",
					tc.name, visualLines, termH)
			}
		})
	}
}
