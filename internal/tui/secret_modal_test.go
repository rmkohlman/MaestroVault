package tui

import (
	"fmt"
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

// ── Overflow trace tests ──────────────────────────────────────

// TestSecretModal_EditMode_LargeValue_CenterOverlay tests the FULL render path
// including centerOverlay (lipgloss.Place), which is what the user actually sees.
// This is the exact scenario: large PEM cert -> edit mode -> centered in terminal.
func TestSecretModal_EditMode_LargeValue_CenterOverlay(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-pem-cert", largeSecret(100))
	m := newEditModalWithDims(entry, termHeight, termWidth)

	// This is what editModeView returns (the modal box itself)
	modalOnly := m.editModeView("Editing")
	modalLines := lineCount(modalOnly)

	// This is what the user actually sees — the modal centered via lipgloss.Place
	// (simulating what views.go centerOverlay does)
	centeredOutput := m.centerStandalone(modalOnly) // Uses lipgloss.Place
	centeredLines := lineCount(centeredOutput)

	t.Logf("editModeView() output: %d lines", modalLines)
	t.Logf("After centering (lipgloss.Place): %d lines", centeredLines)
	t.Logf("Terminal height: %d", termHeight)

	if modalLines > termHeight {
		t.Errorf("editModeView() produces %d lines, exceeds terminal height %d", modalLines, termHeight)
	}
	if centeredLines > termHeight {
		t.Errorf("Centered output produces %d lines, exceeds terminal height %d", centeredLines, termHeight)
	}
}

// TestSecretModal_EditMode_TextareaRenderedHeight checks that the textarea
// view itself doesn't produce more lines than textareaHeight() + a small margin.
// NOTE: NewSecretModalEdit does NOT call initValueTextarea — we must do it manually
// to test the textarea path (simulating the view→edit transition).
func TestSecretModal_EditMode_TextareaRenderedHeight(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-pem-cert", largeSecret(100))
	m := newEditModalWithDims(entry, termHeight, termWidth)

	// Simulate the view→edit transition which checks isLargeValue and inits textarea.
	if isLargeValue(entry.Value) {
		m.initValueTextarea(entry.Value)
	}

	if !m.useTextarea {
		t.Fatal("expected useTextarea=true for large value after initValueTextarea")
	}

	taHeight := m.textareaHeight()
	taView := m.valueArea.View()
	taViewLines := strings.Count(taView, "\n") + 1

	t.Logf("textareaHeight() = %d", taHeight)
	t.Logf("textarea.View() produces %d lines", taViewLines)

	// The textarea's View() output should roughly match its configured height.
	// A small deviation (±5) is expected due to internal padding/chrome.
	if taViewLines > taHeight+5 {
		t.Errorf("textarea View() produces %d lines but textareaHeight()=%d (delta=%d)",
			taViewLines, taHeight, taViewLines-taHeight)
	}
}

// TestSecretModal_EditMode_ConstructorMissingTextarea verifies that
// NewSecretModalEdit does NOT initialize textarea for large values — which
// means Path 1 (list→edit) always uses textinput regardless of value size.
func TestSecretModal_EditMode_ConstructorMissingTextarea(t *testing.T) {
	entry := newTestEntry("my-pem-cert", largeSecret(100))

	// Verify isLargeValue detects this correctly.
	if !isLargeValue(entry.Value) {
		t.Fatal("expected isLargeValue to return true for 100-line secret")
	}

	m := NewSecretModalEdit(entry, nil)
	m.height = 40
	m.width = 80

	t.Logf("useTextarea = %v (expected false — constructor doesn't init textarea)", m.useTextarea)
	t.Logf("valueInput value length = %d bytes", len(m.valueInput.Value()))

	if m.useTextarea {
		t.Error("NewSecretModalEdit unexpectedly initialized textarea")
	}
}

// TestSecretModal_EditMode_ViewToEditPath_Textarea tests the view→edit transition
// path which DOES initialize textarea. This simulates: user views secret, presses
// "e" to switch to edit mode.
func TestSecretModal_EditMode_ViewToEditPath_Textarea(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-pem-cert", largeSecret(100))

	// Start in view mode (as the user would)
	m := newModalWithDims(entry, termHeight, termWidth)

	// Simulate pressing "e" — the handleViewKey code path
	// This is what handleViewKey does at line 486-509:
	m.mode = modalEdit
	m.origName = entry.Name
	m.origEnv = entry.Environment
	m.nameInput.SetValue(entry.Name)
	m.envInput.SetValue(entry.Environment)
	m.metadataInput.SetValue(formatMetadataPlain(entry.Metadata))
	if isLargeValue(entry.Value) {
		m.initValueTextarea(entry.Value)
	} else {
		m.valueInput.SetValue(entry.Value)
	}
	m.focusField = fieldName
	m.focusCurrentField()

	// Now render the edit mode view
	rendered := m.editModeView("Editing")
	lines := lineCount(rendered)

	t.Logf("View→Edit path (textarea): useTextarea=%v, rendered lines=%d", m.useTextarea, lines)

	if lines > termHeight {
		t.Errorf("View→Edit path: %d lines exceeds terminal height %d", lines, termHeight)
	}

	// Also test the full render path through centerOverlay
	centeredOutput := m.centerStandalone(rendered)
	centeredLines := lineCount(centeredOutput)

	t.Logf("View→Edit path centered: %d lines", centeredLines)

	if centeredLines > termHeight {
		t.Errorf("View→Edit centered: %d lines exceeds terminal height %d", centeredLines, termHeight)
	}
}

// TestSecretModal_FullRenderPath_ThroughModelView tests the complete render
// path: SecretModal.View() -> Model.View() -> centerOverlay
func TestSecretModal_FullRenderPath_ThroughModelView(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-pem-cert", largeSecret(100))

	// Build parent Model with the secret modal open
	parentModel := Model{
		width:           termWidth,
		height:          termHeight,
		showSecretModal: true,
	}

	// Build the edit modal
	sm := NewSecretModalEdit(entry, nil)
	sm.width = termWidth
	sm.height = termHeight
	sm.initViewport()
	parentModel.secretModal = sm

	// Render through the parent Model.View()
	fullOutput := parentModel.View()
	lines := lineCount(fullOutput)

	// Also count RAW lines (before ANSI stripping) to catch any discrepancy.
	rawLines := strings.Count(fullOutput, "\n") + 1

	t.Logf("Full Model.View() output: %d lines (ANSI-stripped), %d raw lines", lines, rawLines)

	if lines > termHeight {
		t.Errorf("Full render path produces %d lines, exceeds terminal height %d", lines, termHeight)
	}
	if rawLines > termHeight {
		t.Errorf("Full render path (raw) produces %d lines, exceeds terminal height %d", rawLines, termHeight)
	}
}

// TestSecretModal_EditMode_RawLineCount verifies the raw (non-ANSI-stripped)
// line count, which is what the terminal actually renders. ANSI stripping
// can collapse lines if escape codes span newlines.
func TestSecretModal_EditMode_RawLineCount(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
		termWidth  int
		numLines   int
		useTA      bool // whether to simulate textarea init
	}{
		{"small term no textarea", 40, 80, 100, false},
		{"small term with textarea", 40, 80, 100, true},
		{"large term no textarea", 60, 200, 100, false},
		{"large term with textarea", 60, 200, 100, true},
		{"24-line terminal", 24, 80, 100, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := newTestEntry("my-pem-cert", largeSecret(tc.numLines))
			m := newEditModalWithDims(entry, tc.termHeight, tc.termWidth)

			if tc.useTA && isLargeValue(entry.Value) {
				m.initValueTextarea(entry.Value)
			}

			rendered := m.editModeView("Editing")
			rawLines := strings.Count(rendered, "\n") + 1
			strippedLines := lineCount(rendered)

			t.Logf("useTextarea=%v raw=%d stripped=%d termH=%d",
				m.useTextarea, rawLines, strippedLines, tc.termHeight)

			if rawLines > tc.termHeight {
				t.Errorf("raw line count %d exceeds terminal height %d", rawLines, tc.termHeight)
			}
		})
	}
}

// TestSecretModal_EditMode_ManyFieldPairs_NoTextarea tests with many field pairs
// to verify the non-textarea viewportRender path constrains correctly.
func TestSecretModal_EditMode_ManyFieldPairs_NoTextarea(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-secret", "short-value")
	m := newEditModalWithDims(entry, termHeight, termWidth)

	// Add many field pairs
	for i := 0; i < 20; i++ {
		m.fieldPairs = append(m.fieldPairs, newFieldPair(
			fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i)))
	}

	rendered := m.editModeView("Editing")
	lines := lineCount(rendered)

	t.Logf("edit mode with 20 field pairs (no textarea): %d lines", lines)

	if lines > termHeight {
		t.Errorf("edit mode with many fields: %d lines exceeds terminal height %d", lines, termHeight)
	}
}

// TestSecretModal_ViewMode_Reveal_LargeValue tests the EXACT user scenario:
// View mode with a large value, user presses "p" to reveal (unmask) the value.
// In view mode, revealing a large value shows a truncated multi-line preview.
// The viewport should constrain this to terminal bounds.
func TestSecretModal_ViewMode_Reveal_LargeValue(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
		termWidth  int
		numLines   int
	}{
		{"40-line terminal 100-line secret", 40, 80, 100},
		{"24-line terminal 100-line secret", 24, 80, 100},
		{"40-line terminal 500-line secret", 40, 80, 500},
		{"60-line terminal 100-line secret", 60, 200, 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := newTestEntry("my-pem-cert", largeSecret(tc.numLines))
			m := newModalWithDims(entry, tc.termHeight, tc.termWidth)

			// Default state: value is masked.
			maskedRender := m.viewModeView()
			maskedLines := strings.Count(maskedRender, "\n") + 1
			t.Logf("Masked: %d raw lines", maskedLines)

			// Reveal the value (toggle viewMasks[0] to false).
			m.viewMasks[0] = false

			// Now render with revealed value.
			revealedRender := m.viewModeView()
			revealedLines := strings.Count(revealedRender, "\n") + 1
			revealedStrippedLines := lineCount(revealedRender)
			t.Logf("Revealed: %d raw lines, %d stripped lines", revealedLines, revealedStrippedLines)

			if revealedLines > tc.termHeight {
				t.Errorf("View mode (revealed): %d raw lines exceeds terminal height %d",
					revealedLines, tc.termHeight)
			}

			// Also test through centerOverlay.
			centered := m.centerStandalone(revealedRender)
			centeredLines := strings.Count(centered, "\n") + 1
			t.Logf("Revealed + centered: %d raw lines", centeredLines)

			if centeredLines > tc.termHeight {
				t.Errorf("View mode (revealed, centered): %d lines exceeds terminal height %d",
					centeredLines, tc.termHeight)
			}
		})
	}
}

// TestSecretModal_ViewMode_Reveal_AllPaths tests all intermediate outputs
// through the view-mode reveal render path, to pinpoint where overflow occurs.
func TestSecretModal_ViewMode_Reveal_AllPaths(t *testing.T) {
	const termHeight = 40
	const termWidth = 80

	entry := newTestEntry("my-pem-cert", largeSecret(100))

	// Test the FULL path: SecretModal.View() → Model.centerOverlay()
	parentModel := Model{
		width:           termWidth,
		height:          termHeight,
		showSecretModal: true,
	}

	sm := NewSecretModalView(entry, nil)
	sm.width = termWidth
	sm.height = termHeight
	sm.initViewport()
	// Reveal the value.
	sm.viewMasks[0] = false
	parentModel.secretModal = sm

	// SecretModal.View() calls viewModeView()
	modalView := sm.View()
	modalLines := strings.Count(modalView, "\n") + 1
	t.Logf("SecretModal.View() (view mode, revealed): %d raw lines", modalLines)

	// Model.View() wraps it in centerOverlay
	fullView := parentModel.View()
	fullLines := strings.Count(fullView, "\n") + 1
	t.Logf("Model.View() (full path): %d raw lines", fullLines)

	if modalLines > termHeight {
		t.Errorf("SecretModal.View() overflow: %d lines > %d", modalLines, termHeight)
	}
	if fullLines > termHeight {
		t.Errorf("Model.View() overflow: %d lines > %d", fullLines, termHeight)
	}
}

// TestDetailScreen_LargeValue_Revealed tests the NON-modal detail screen
// (screenDetail) when a large value is revealed. This is the screen shown
// when the user presses Enter on a secret in the list, then Space to peek.
func TestDetailScreen_LargeValue_Revealed(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
		termWidth  int
		numLines   int
	}{
		{"40-line terminal 100-line secret", 40, 80, 100},
		{"24-line terminal 50-line secret", 24, 80, 50},
		{"60-line terminal 200-line secret", 60, 200, 200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := vault.SecretEntry{
				Name:      "my-pem-cert",
				Value:     largeSecret(tc.numLines),
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-01T00:00:00Z",
			}

			m := Model{
				width:       tc.termWidth,
				height:      tc.termHeight,
				screen:      screenDetail,
				secrets:     []vault.SecretEntry{entry},
				display:     []vault.SecretEntry{entry},
				cursor:      0,
				valueMasked: false, // VALUE IS REVEALED
			}

			rendered := m.viewDetailScreen()
			rawLines := strings.Count(rendered, "\n") + 1

			t.Logf("Detail screen (revealed, %d-line value): %d raw lines, terminal height %d",
				tc.numLines, rawLines, tc.termHeight)

			if rawLines > tc.termHeight {
				t.Errorf("OVERFLOW: viewDetailScreen() produces %d lines, exceeds terminal height %d (excess: %d lines)",
					rawLines, tc.termHeight, rawLines-tc.termHeight)
			}
		})
	}
}

// TestDetailScreen_SingleLongLine_Revealed tests with a single very long line
// (e.g., JWT token, API key, base64 blob) that has NO newlines. This was the
// root cause of the v0.10.3 overflow: viewportRender counts \n-delimited lines,
// but a 5000-char single line wraps to ~63 visual lines at 80 columns.
// The fix wraps long lines via wrapAndTruncate before passing to viewportRender.
func TestDetailScreen_SingleLongLine_Revealed(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
		termWidth  int
		valueLen   int
	}{
		{"5000 chars at 80x40", 40, 80, 5000},
		{"10000 chars at 80x24", 24, 80, 10000},
		{"2000 chars at 120x30", 30, 120, 2000},
		{"500 chars at 80x40", 40, 80, 500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			longValue := strings.Repeat("A", tc.valueLen)

			entry := vault.SecretEntry{
				Name:      "my-jwt-token",
				Value:     longValue,
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-01T00:00:00Z",
			}

			m := Model{
				width:       tc.termWidth,
				height:      tc.termHeight,
				screen:      screenDetail,
				secrets:     []vault.SecretEntry{entry},
				display:     []vault.SecretEntry{entry},
				cursor:      0,
				valueMasked: false,
			}

			rendered := m.viewDetailScreen()
			rawLines := strings.Count(rendered, "\n") + 1

			// Also measure visual lines (accounting for terminal wrapping).
			visualLines := 0
			for _, line := range strings.Split(rendered, "\n") {
				plain := stripANSI(line)
				w := len([]rune(plain))
				if w <= tc.termWidth {
					visualLines++
				} else {
					visualLines += (w + tc.termWidth - 1) / tc.termWidth
				}
			}

			t.Logf("Detail screen (single %d-char line): raw=%d, visual=%d, terminal %dx%d",
				tc.valueLen, rawLines, visualLines, tc.termWidth, tc.termHeight)

			if rawLines > tc.termHeight {
				t.Errorf("OVERFLOW (raw): %d lines exceeds terminal height %d",
					rawLines, tc.termHeight)
			}
			if visualLines > tc.termHeight {
				t.Errorf("OVERFLOW (visual): %d visual lines exceeds terminal height %d",
					visualLines, tc.termHeight)
			}
		})
	}
}

// TestDetailScreen_RealCertChain_3Section tests the exact user scenario from
// issue testing: a real OpenSSL-generated 3-section PEM certificate chain
// (root CA → intermediate CA → leaf cert) stored in mav and rendered in the
// detail screen with value revealed. This is a 63-line, ~3800-char PEM file.
func TestDetailScreen_RealCertChain_3Section(t *testing.T) {
	// Real 3-section PEM certificate chain (leaf + intermediate + root).
	certChain := `-----BEGIN CERTIFICATE-----
MIIDfDCCAmSgAwIBAgIUHSqasWOWU8l2tFEljEcv21Ggv18wDQYJKoZIhvcNAQEL
BQAwXjELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExGzAZBgNVBAoM
ElRlc3RJbnRlcm1lZGlhdGVDQTEdMBsGA1UEAwwUVGVzdCBJbnRlcm1lZGlhdGUg
Q0EwHhcNMjYwMzIzMTYxMTU4WhcNMjcwMzIzMTYxMTU4WjBOMQswCQYDVQQGEwJV
UzETMBEGA1UECAwKQ2FsaWZvcm5pYTEQMA4GA1UECgwHVGVzdEFwcDEYMBYGA1UE
AwwPYXBwLmV4YW1wbGUuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEAyyyPTQjG+PwBQIauabLx5Pu/5Dt3SSvTbdmdyjwZ+dS2HQtlPxeiRBLcHR1H
u+bfPKK33DKLRrlpyw6KWbx9j9G1oe1vNmDf0MqtM+Cayh9NK//F0D8LPMPfrEOM
CEMcyYCIl82MpF06HD178qRfVv/z9R0s7pLO8U/tpJBa5DHVCBZbD4YBIA/6OZCI
tJ+kx1oJsIU+7pUGjgtmbDf9Ijp7KHl9HsU2jgGmQox988Mftqyg79REeqhLXc5w
Q7TMrA2Vb6haR9MphoSLC2ZFARRjiXMkN9o8rpifBuUDf4EPamYfbfWBTqWmNmxa
nPV4jiCaczJI/S4O+J6Btij8EwIDAQABo0IwQDAdBgNVHQ4EFgQUpQG3Ul8mKanS
yqpXUb6d/CMfFwUwHwYDVR0jBBgwFoAUomoPkqWwXwjD5Rt6K6QgBAj4eacwDQYJ
KoZIhvcNAQELBQADggEBAB+X0avSryRlbLtW8QNNJi219R5K8+AnFNeQh8noScMD
viInMk0D325grP2J9xchXN0rrPP2WNXlFJH1l72JqxVl1b5DMPn5yC9uSkF2wSJb
zD92cEmqqJ3mGmJBSO4Xkepfcz3iNLnE6+aQbVc/eBvLpt9N3TZxIgve9FrUten7
UktYeES6JUGNaAvQZbGus2OhFZ98wSIn+sdL75soR63/LwziJV0MKcpGUKq+zgNb
l46hzh8Gy5Zp2q+QTIiqBBIQ923Fnl2Y016KNqaYp2kBl+7VJsbNpe1xKvI86s2J
+hpxY3ULq7gbdi64UbvN2thqWT0GKXxR/HG4/07RQKQ=
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIDfDCCAmSgAwIBAgIUZXTspEGdy2jNYl7P7wnQwK6zLTUwDQYJKoZIhvcNAQEL
BQAwTjELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExEzARBgNVBAoM
ClRlc3RSb290Q0ExFTATBgNVBAMMDFRlc3QgUm9vdCBDQTAeFw0yNjAzMjMxNjEx
NThaFw0yNzAzMjMxNjExNThaMF4xCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxp
Zm9ybmlhMRswGQYDVQQKDBJUZXN0SW50ZXJtZWRpYXRlQ0ExHTAbBgNVBAMMFFRl
c3QgSW50ZXJtZWRpYXRlIENBMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEAxehoULi6PPhwHJirPbr4la7KfjCnHci+EAMOZKX65N/CXDRE73rBY04zQEWI
hsybMrsnGjEgFrUHBaoiMLhEThyzQY4im6sbX9W1X0d4+JPXPguDf7zCGmmhYVtw
VMkVMMXf1cb8D/VRnnUoKc7wYqKKpwKmPdka0qeAxh0rdP8u0abH/AnrxbYeBcHb
ifOoZKBSyYQpB/OtZ5ix1lWynK7MH/Rp4KbRLyCcniV2bDZQRyRsUdVePv4k4HLS
UE7s9kbGTosBRtzDxfbv56h6kmmsiQCHhd+lIQ/6W0Nm8WbMhwFm5pU2sgITKegb
P/LFJzBPXQD1xdL3bNeDZ9HMHwIDAQABo0IwQDAdBgNVHQ4EFgQUomoPkqWwXwjD
5Rt6K6QgBAj4eacwHwYDVR0jBBgwFoAUufvrSDY8wwLQ9QXXxpzWEkRkeKEwDQYJ
KoZIhvcNAQELBQADggEBAGozUJDte/iEuWWsDyx0AXNEU5x4UZqbm/d+CgudZ8oA
qS6zKkmXhF25unDMLdZJRCuMmFQdS9MxsDvzcH14tDVJi7up5GoY6m1uFokRvXLP
cb/mYdweSUrLjDvfdyAVpt2TGSqUr+aJRtKtPkq0V/dRFNxO/+wlGdb0wLdweQYT
uzeXDrj5/peHELWQDwqv4gVd1IyLv5aIgyfKrui/uUhxRCzbAemZOPxzDb9l7qwH
Stzi6Kv9fzGQpkFs5WYRVLKSKjmcalcUknRrdDIdfe6uzg3w0TpAPYrZ7bKll5Sb
ug6hs+ooDwBd/dlIUeqfNglvXRfuliJUiNZIEGUdT/I=
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIDfTCCAmWgAwIBAgIURcYbsC7gRiggTAy2hvjp1COIVEkwDQYJKoZIhvcNAQEL
BQAwTjELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExEzARBgNVBAoM
ClRlc3RSb290Q0ExFTATBgNVBAMMDFRlc3QgUm9vdCBDQTAeFw0yNjAzMjMxNjEx
NThaFw0yNzAzMjMxNjExNThaME4xCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxp
Zm9ybmlhMRMwEQYDVQQKDApUZXN0Um9vdENBMRUwEwYDVQQDDAxUZXN0IFJvb3Qg
Q0EwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCoCdqyJJhxASTSzhA4
Mkjk7w7t40GfiSEfFSztJ4BQR2csMs2pi4KCzskhMT3L3Uj03yfoMYLe2+87KSQb
HchTELQF7jIuPgoEk9HUqhR7hmn+K05FEtt0XefwnoBgm4zQw49mEknfezaVvjQ4
E0NJ4y39HO5M74Jz2x4FlAOCQ1VQcEdiu/yVE/xp0khF/Secq6NpXabEazkQsTp9
ZXAp50b97uM5ZwB3D2EuEx2GhnZal9LgTB4KxzC+iYUDDoLjI85HiyvmCA4SRA6U
wP1XGyygiqpcLqkIA42CwFaulsMFZbLmQjLGGV8Fij7kcBnh3LKaAf3kkks1uz8p
1w2xAgMBAAGjUzBRMB0GA1UdDgQWBBS5++tINjzDAtD1BdfGnNYSRGR4oTAfBgNV
HSMEGDAWgBS5++tINjzDAtD1BdfGnNYSRGR4oTAPBgNVHRMBAf8EBTADAQH/MA0G
CSqGSIb3DQEBCwUAA4IBAQAZzQn2eUcqoG8kUqD/gHhOwOUujjMPgMPA7Fa5RnL0
liPcf/alLVnJ/YLX54bdJe5KR66C5KWFf1jH3mFS8lPWjANZNSkG6TC7jE96Bzn/
59WlinRoE26axnoYsp2wi1WdXhBQCkpga4ozsEbvC6xXzf7OT9Vu6fPvhkqc9miJ
6CXAmLNIsG94ZP/FP6W0+2yScKO/Nf1wXdQq3VFjdd+ywwtBUrstqF5nYxMWtk6p
coB5oJhehTBc8+R0pMXL4WZ9Z8KuWrm9qvhJKfb3FBOq0ZIw/27/jF0JxWeKshjX
y8S9L9EIiVV7JIjfAjWJTr1pF7VQxyLmun9CYSF/TUE8
-----END CERTIFICATE-----`

	certLines := strings.Count(certChain, "\n") + 1
	certChars := len(certChain)
	certSections := strings.Count(certChain, "-----BEGIN CERTIFICATE-----")
	t.Logf("Certificate chain: %d lines, %d chars, %d sections", certLines, certChars, certSections)

	if certSections != 3 {
		t.Fatalf("expected 3 certificate sections, got %d", certSections)
	}

	tests := []struct {
		name       string
		termHeight int
		termWidth  int
	}{
		{"40 rows x 80 cols (small terminal)", 40, 80},
		{"24 rows x 80 cols (minimal terminal)", 24, 80},
		{"60 rows x 120 cols (large terminal)", 60, 120},
		{"30 rows x 100 cols (medium terminal)", 30, 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := vault.SecretEntry{
				Name:      "test-cert-chain",
				Value:     certChain,
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-01T00:00:00Z",
			}

			// ── Detail screen (screenDetail) with revealed value ──
			m := Model{
				width:       tc.termWidth,
				height:      tc.termHeight,
				screen:      screenDetail,
				secrets:     []vault.SecretEntry{entry},
				display:     []vault.SecretEntry{entry},
				cursor:      0,
				valueMasked: false, // VALUE REVEALED
			}

			rendered := m.viewDetailScreen()
			rawLines := strings.Count(rendered, "\n") + 1

			t.Logf("Detail screen (revealed cert chain): %d raw lines, terminal %dx%d",
				rawLines, tc.termWidth, tc.termHeight)

			if rawLines > tc.termHeight {
				t.Errorf("OVERFLOW: viewDetailScreen() produces %d lines, exceeds terminal height %d (excess: %d)",
					rawLines, tc.termHeight, rawLines-tc.termHeight)
			}

			// ── Full Model.View() path ──
			fullRendered := m.View()
			fullRawLines := strings.Count(fullRendered, "\n") + 1

			t.Logf("Full Model.View(): %d raw lines", fullRawLines)

			if fullRawLines > tc.termHeight {
				t.Errorf("OVERFLOW: Model.View() produces %d lines, exceeds terminal height %d",
					fullRawLines, tc.termHeight)
			}

			// ── Detail screen with masked value (should be trivially small) ──
			m.valueMasked = true
			maskedRendered := m.viewDetailScreen()
			maskedLines := strings.Count(maskedRendered, "\n") + 1

			t.Logf("Detail screen (masked): %d raw lines", maskedLines)

			if maskedLines > tc.termHeight {
				t.Errorf("OVERFLOW: masked viewDetailScreen() produces %d lines, exceeds terminal height %d",
					maskedLines, tc.termHeight)
			}
		})
	}
}

// TestSecretModal_ViewMode_RealCertChain tests the SecretModal view mode
// with a real 3-section PEM cert chain — both masked and revealed states.
func TestSecretModal_ViewMode_RealCertChain(t *testing.T) {
	certChain := `-----BEGIN CERTIFICATE-----
MIIDfDCCAmSgAwIBAgIUHSqasWOWU8l2tFEljEcv21Ggv18wDQYJKoZIhvcNAQEL
BQAwXjELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExGzAZBgNVBAoM
ElRlc3RJbnRlcm1lZGlhdGVDQTEdMBsGA1UEAwwUVGVzdCBJbnRlcm1lZGlhdGUg
Q0EwHhcNMjYwMzIzMTYxMTU4WhcNMjcwMzIzMTYxMTU4WjBOMQswCQYDVQQGEwJV
UzETMBEGA1UECAwKQ2FsaWZvcm5pYTEQMA4GA1UECgwHVGVzdEFwcDEYMBYGA1UE
AwwPYXBwLmV4YW1wbGUuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEAyyyPTQjG+PwBQIauabLx5Pu/5Dt3SSvTbdmdyjwZ+dS2HQtlPxeiRBLcHR1H
u+bfPKK33DKLRrlpyw6KWbx9j9G1oe1vNmDf0MqtM+Cayh9NK//F0D8LPMPfrEOM
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIDfDCCAmSgAwIBAgIUZXTspEGdy2jNYl7P7wnQwK6zLTUwDQYJKoZIhvcNAQEL
BQAwTjELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExEzARBgNVBAoM
ClRlc3RSb290Q0ExFTATBgNVBAMMDFRlc3QgUm9vdCBDQTAeFw0yNjAzMjMxNjEx
NThaFw0yNzAzMjMxNjExNThaMF4xCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxp
Zm9ybmlhMRswGQYDVQQKDBJUZXN0SW50ZXJtZWRpYXRlQ0ExHTAbBgNVBAMMFFRl
c3QgSW50ZXJtZWRpYXRlIENBMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIDfTCCAmWgAwIBAgIURcYbsC7gRiggTAy2hvjp1COIVEkwDQYJKoZIhvcNAQEL
BQAwTjELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExEzARBgNVBAoM
ClRlc3RSb290Q0ExFTATBgNVBAMMDFRlc3QgUm9vdCBDQTAeFw0yNjAzMjMxNjEx
NThaFw0yNzAzMjMxNjExNThaME4xCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxp
Zm9ybmlhMRMwEQYDVQQKDApUZXN0Um9vdENBMRUwEwYDVQQDDAxUZXN0IFJvb3Qg
Q0EwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCoCdqyJJhxASTSzhA4
CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=
-----END CERTIFICATE-----`

	tests := []struct {
		name       string
		termHeight int
		termWidth  int
	}{
		{"40x80", 40, 80},
		{"24x80", 24, 80},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := newTestEntry("test-cert-chain", certChain)
			entry.CreatedAt = "2025-01-01T00:00:00Z"
			entry.UpdatedAt = "2025-01-01T00:00:00Z"

			// View mode modal — masked (default).
			m := newModalWithDims(entry, tc.termHeight, tc.termWidth)
			rendered := m.viewModeView()
			rawLines := strings.Count(rendered, "\n") + 1

			t.Logf("SecretModal view (masked): %d raw lines, terminal %dx%d",
				rawLines, tc.termWidth, tc.termHeight)

			if rawLines > tc.termHeight {
				t.Errorf("OVERFLOW (masked modal): %d lines > %d", rawLines, tc.termHeight)
			}

			// View mode modal — revealed.
			m.viewMasks[0] = false
			revealedRender := m.viewModeView()
			revealedLines := strings.Count(revealedRender, "\n") + 1

			t.Logf("SecretModal view (revealed): %d raw lines", revealedLines)

			if revealedLines > tc.termHeight {
				t.Errorf("OVERFLOW (revealed modal): %d lines > %d", revealedLines, tc.termHeight)
			}

			// Centered (standalone) path.
			centered := m.centerStandalone(revealedRender)
			centeredLines := strings.Count(centered, "\n") + 1

			t.Logf("SecretModal centered (revealed): %d raw lines", centeredLines)

			if centeredLines > tc.termHeight {
				t.Errorf("OVERFLOW (centered modal): %d lines > %d", centeredLines, tc.termHeight)
			}
		})
	}
}

// TestDetailScreen_FullModelView_AllValueTypes tests the complete Model.View()
// path for screenDetail with various value shapes: multi-line, single long line,
// short value, and fields with large values.
func TestDetailScreen_FullModelView_AllValueTypes(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
		termWidth  int
		value      string
		fields     map[string]string
	}{
		{
			"multi-line PEM cert",
			40, 80,
			largeSecret(100),
			nil,
		},
		{
			"single long JWT token",
			40, 80,
			strings.Repeat("eyJhbGciOiJSUzI1NiJ9.", 200),
			nil,
		},
		{
			"short value with large field",
			40, 80,
			"short-password",
			map[string]string{
				"cert": strings.Repeat("MIIB", 1000),
			},
		},
		{
			"multiple large fields",
			40, 80,
			"",
			map[string]string{
				"private_key": largeSecret(50),
				"public_key":  largeSecret(50),
				"cert_chain":  strings.Repeat("Z", 3000),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := vault.SecretEntry{
				Name:      "test-secret",
				Value:     tc.value,
				Fields:    tc.fields,
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-01T00:00:00Z",
			}

			m := Model{
				width:       tc.termWidth,
				height:      tc.termHeight,
				screen:      screenDetail,
				secrets:     []vault.SecretEntry{entry},
				display:     []vault.SecretEntry{entry},
				cursor:      0,
				valueMasked: false,
			}

			// Test through the full Model.View() path
			rendered := m.View()
			rawLines := strings.Count(rendered, "\n") + 1

			t.Logf("%s: %d raw lines, terminal %dx%d", tc.name, rawLines, tc.termWidth, tc.termHeight)

			if rawLines > tc.termHeight {
				t.Errorf("OVERFLOW: Model.View() produces %d lines, exceeds terminal height %d",
					rawLines, tc.termHeight)
			}

			// Also measure visual lines
			visualLines := 0
			for _, line := range strings.Split(rendered, "\n") {
				plain := stripANSI(line)
				w := len([]rune(plain))
				if w <= tc.termWidth {
					visualLines++
				} else {
					visualLines += (w + tc.termWidth - 1) / tc.termWidth
				}
			}

			if visualLines > tc.termHeight {
				t.Errorf("OVERFLOW (visual): %d visual lines exceeds terminal height %d",
					visualLines, tc.termHeight)
			}
		})
	}
}
