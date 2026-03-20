package tui

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// ansiEscapeRe matches ANSI CSI escape sequences so we can measure the
// visual (printed) width of a line rather than its raw byte count.
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[mKHFABCDJsuhl]`)

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

// renderLines splits rendered output into lines after stripping ANSI codes.
func renderLines(rendered string) []string {
	return strings.Split(stripANSI(rendered), "\n")
}

// maxLineWidthRunes returns the longest visual line width in runes (not bytes).
// This is the correct measure for terminal display since multi-byte UTF-8
// characters such as box-drawing glyphs (╭, │, ─) each occupy one terminal
// column despite being 3 bytes in UTF-8.
func maxLineWidthRunes(rendered string) int {
	lines := renderLines(rendered)
	max := 0
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > max {
			max = n
		}
	}
	return max
}

// lineCount returns the number of lines in the rendered output.
func lineCount(rendered string) int {
	return len(renderLines(rendered))
}

// largeSecret builds a value that exceeds largeValueThreshold and spans
// numLines lines so it exercises both the "large value" path and viewport
// clamping.
func largeSecret(numLines int) string {
	line := strings.Repeat("x", 60)
	lines := make([]string, numLines)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// newTestEntry creates a minimal *vault.SecretEntry for use in tests.
func newTestEntry(name, value string) *vault.SecretEntry {
	return &vault.SecretEntry{
		Name:  name,
		Value: value,
	}
}

// newModalWithDims returns a SecretModal in view mode with explicit terminal
// dimensions set (bypasses the need for a real terminal / WindowSizeMsg).
func newModalWithDims(entry *vault.SecretEntry, height, width int) SecretModal {
	m := NewSecretModalView(entry, nil)
	m.height = height
	m.width = width
	// Re-initialise the viewport with the correct dimensions now that height
	// and width are set.
	m.initViewport()
	return m
}

// newEditModalWithDims returns a SecretModal in edit mode with explicit dims.
func newEditModalWithDims(entry *vault.SecretEntry, height, width int) SecretModal {
	m := NewSecretModalEdit(entry, nil)
	m.height = height
	m.width = width
	m.initViewport()
	return m
}

// ── Tests ─────────────────────────────────────────────────────

// TestSecretModal_ViewMode_SmallTerminal verifies that rendering a view-mode
// modal on a small (80×40) terminal with a 100-line secret never exceeds the
// terminal boundaries.
//
// Width is measured in runes (visual columns), not bytes, since lipgloss
// border glyphs (╭, │, ─) are 3 bytes in UTF-8 but occupy 1 terminal column.
func TestSecretModal_ViewMode_SmallTerminal(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-secret", largeSecret(100))
	m := newModalWithDims(entry, termHeight, termWidth)

	rendered := m.viewModeView()
	lines := lineCount(rendered)
	maxWidth := maxLineWidthRunes(rendered)

	if lines > termHeight {
		t.Errorf("view mode: line count %d exceeds terminal height %d", lines, termHeight)
	}
	if maxWidth > termWidth {
		t.Errorf("view mode: max line width %d runes exceeds terminal width %d", maxWidth, termWidth)
	}
}

// TestSecretModal_EditMode_SmallTerminal verifies that rendering an edit-mode
// modal on a small (80×40) terminal with a 100-line secret never exceeds the
// terminal boundaries.
func TestSecretModal_EditMode_SmallTerminal(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-secret", largeSecret(100))
	m := newEditModalWithDims(entry, termHeight, termWidth)

	rendered := m.editModeView("Editing")
	lines := lineCount(rendered)
	maxWidth := maxLineWidthRunes(rendered)

	if lines > termHeight {
		t.Errorf("edit mode: line count %d exceeds terminal height %d", lines, termHeight)
	}
	if maxWidth > termWidth {
		t.Errorf("edit mode: max line width %d runes exceeds terminal width %d", maxWidth, termWidth)
	}
}

// TestSecretModal_ViewMode_LargeTerminal verifies the same invariant holds on
// a wider (200×60) terminal.
func TestSecretModal_ViewMode_LargeTerminal(t *testing.T) {
	const termHeight = 60
	const termWidth = 200

	entry := newTestEntry("my-secret", largeSecret(100))
	m := newModalWithDims(entry, termHeight, termWidth)

	rendered := m.viewModeView()
	lines := lineCount(rendered)
	maxWidth := maxLineWidthRunes(rendered)

	if lines > termHeight {
		t.Errorf("view mode (large terminal): line count %d exceeds terminal height %d", lines, termHeight)
	}
	if maxWidth > termWidth {
		t.Errorf("view mode (large terminal): max line width %d runes exceeds terminal width %d", maxWidth, termWidth)
	}
}

// TestSecretModal_EditMode_LargeTerminal verifies the same invariant holds on
// a wider (200×60) terminal in edit mode.
func TestSecretModal_EditMode_LargeTerminal(t *testing.T) {
	const termHeight = 60
	const termWidth = 200

	entry := newTestEntry("my-secret", largeSecret(100))
	m := newEditModalWithDims(entry, termHeight, termWidth)

	rendered := m.editModeView("Editing")
	lines := lineCount(rendered)
	maxWidth := maxLineWidthRunes(rendered)

	if lines > termHeight {
		t.Errorf("edit mode (large terminal): line count %d exceeds terminal height %d", lines, termHeight)
	}
	if maxWidth > termWidth {
		t.Errorf("edit mode (large terminal): max line width %d runes exceeds terminal width %d", maxWidth, termWidth)
	}
}

// TestSecretModal_TextareaHeight_Clamping verifies that textareaHeight() returns
// a value that, when combined with the fixed form overhead and modal frame, stays
// within the terminal height.
func TestSecretModal_TextareaHeight_Clamping(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
		termWidth  int
		numPairs   int // number of dynamic field pairs
		hasToast   bool
	}{
		{"small terminal no fields", 40, 80, 0, false},
		{"small terminal with fields", 40, 80, 3, false},
		{"small terminal with toast", 40, 80, 0, true},
		{"large terminal no fields", 60, 200, 0, false},
		{"large terminal many fields", 60, 200, 10, false},
		{"tiny terminal", 20, 80, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := newTestEntry("test-secret", largeSecret(100))
			m := newEditModalWithDims(entry, tc.termHeight, tc.termWidth)

			// Add field pairs if requested.
			for i := 0; i < tc.numPairs; i++ {
				m.fieldPairs = append(m.fieldPairs, newFieldPair("key", "val"))
			}

			// Add toast if requested.
			if tc.hasToast {
				m.toast = "something went wrong"
				m.toastKind = "error"
			}

			taHeight := m.textareaHeight()

			// textareaHeight must be at least the enforced minimum (3).
			if taHeight < 3 {
				t.Errorf("textareaHeight() = %d, want >= 3", taHeight)
			}

			// The textarea height + fixed overhead + modal frame must not exceed
			// the terminal height.
			//
			// Fixed overhead (from textareaHeight() docstring):
			//   title(1) + blank(1) + name(1) + env(1) + value label(1) + meta(1)
			//   + blank before help(1) + help bar(1) = 8
			// Dynamic overhead: 2+pairs*2 (if any pairs)
			// Toast overhead: 2 (if toast set)
			// Modal frame: border(2) + padding(2) = 4
			overhead := 8
			if tc.numPairs > 0 {
				overhead += 2 + tc.numPairs*2
			}
			if tc.hasToast {
				overhead += 2
			}
			modalFrame := 4

			total := taHeight + overhead + modalFrame
			if total > tc.termHeight {
				t.Errorf(
					"textareaHeight()=%d + overhead=%d + frame=%d = %d exceeds terminal height %d",
					taHeight, overhead, modalFrame, total, tc.termHeight,
				)
			}
		})
	}
}

// TestSecretModal_ViewportHeight_Clamping verifies that viewportHeight() is
// always positive and fits within the terminal height when the modal frame is
// accounted for.
//
// Note: for terminals with height < 9, viewportHeight() enforces a minimum
// of 3 rows (to keep the component usable), which mathematically cannot fit
// within the total overhead of 6 rows. This is intentional — the minimum
// clamp preserves usability over the strict fit guarantee. Tests only verify
// the fit invariant for height >= 9.
func TestSecretModal_ViewportHeight_Clamping(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
		termWidth  int
		expectFit  bool // whether vpHeight+6 should fit within termHeight
	}{
		{"standard terminal", 40, 80, true},
		{"large terminal", 60, 200, true},
		{"minimal fitting terminal", 9, 80, true}, // 9-6=3, exactly at minimum
		{"tiny terminal height 10", 10, 80, true},
		// Below 9, the minimum-3 clamp means overflow is expected/intentional.
		{"sub-minimum terminal h=8", 8, 80, false},
		{"sub-minimum terminal h=6", 6, 80, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := newTestEntry("test-secret", "some value")
			m := newModalWithDims(entry, tc.termHeight, tc.termWidth)

			vpHeight := m.viewportHeight()

			if vpHeight < 3 {
				t.Errorf("viewportHeight() = %d, want >= 3 (enforced minimum)", vpHeight)
			}

			// viewport + non-viewport overhead (help bar=1, newline=1) + frame(4)
			// must fit within terminal height — except in degenerate sub-9-height
			// terminals where the minimum clamp intentionally takes priority.
			total := vpHeight + 6
			fits := total <= tc.termHeight
			if tc.expectFit && !fits {
				t.Errorf(
					"viewportHeight()=%d + overhead=6 = %d exceeds terminal height %d",
					vpHeight, total, tc.termHeight,
				)
			}
		})
	}
}

// TestSecretModal_ModalWidth_Bounds verifies modalWidth() respects its floor
// (56) and ceiling (120).
func TestSecretModal_ModalWidth_Bounds(t *testing.T) {
	tests := []struct {
		name      string
		termWidth int
		wantMin   int
		wantMax   int
	}{
		{"very narrow terminal", 20, 56, 56},
		{"narrow terminal", 60, 56, 56},   // 60-6=54, clamped to floor 56
		{"standard terminal", 80, 74, 74}, // 80-6=74
		{"wide terminal", 130, 120, 120},  // 130-6=124, clamped to ceiling 120
		{"ultra-wide terminal", 200, 120, 120},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := SecretModal{width: tc.termWidth}
			got := m.modalWidth()

			if got < 56 {
				t.Errorf("modalWidth() = %d, want >= 56 (floor)", got)
			}
			if got > 120 {
				t.Errorf("modalWidth() = %d, want <= 120 (ceiling)", got)
			}
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("modalWidth() = %d, want in [%d, %d]", got, tc.wantMin, tc.wantMax)
			}
		})
	}
}
