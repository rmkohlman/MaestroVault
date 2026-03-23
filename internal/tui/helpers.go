package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// ── Help bar ──────────────────────────────────────────────────

func (m Model) helpBar(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts,
			HelpKeyStyle.Render(pairs[i])+" "+HelpDescStyle.Render(pairs[i+1]),
		)
	}

	// Build the full bar and check if it fits within terminal width.
	// If it doesn't, progressively remove trailing items until it fits.
	// This prevents the help bar from wrapping to a second visual line,
	// which breaks the height calculations in viewDetailScreen et al.
	sep := MutedStyle.Render("  ·  ")
	prefix := MutedStyle.Render("  ")
	maxWidth := m.width
	if maxWidth <= 0 {
		maxWidth = 80
	}

	for len(parts) > 0 {
		bar := prefix + strings.Join(parts, sep)
		// Measure visual width: strip ANSI escapes and count runes.
		if runeWidth(stripANSISeqs(bar)) <= maxWidth {
			return bar
		}
		// Drop the last item and try again.
		parts = parts[:len(parts)-1]
	}

	return prefix
}

// vimHelpBar returns context-sensitive help for the current vim mode/screen.
func (m Model) vimHelpBar() string {
	switch m.mode {
	case ModeNormal:
		switch m.screen {
		case screenList:
			return m.helpBar(
				"j/k", "move",
				"gg/G", "top/end",
				"l/↵", "view",
				"i", "add",
				"e", "edit",
				"c", "copy",
				"dd/x", "del",
				"v", "visual",
				"/", "search",
				"s", "sort",
				"n", "generate",
				"S", "settings",
				"?", "help",
			)
		case screenDetail:
			return m.helpBar(
				"h/esc", "back",
				"space", "peek",
				"c", "copy",
				"e", "edit",
				"d", "delete",
			)
		}
	case ModeVisual:
		return m.helpBar(
			"j/k", "extend",
			"d/x", "delete",
			"esc", "normal",
		)
	}
	return ""
}

// ── Selection ─────────────────────────────────────────────────

// selectionRange returns the inclusive lo..hi range of selected indices.
func (m Model) selectionRange() (int, int) {
	if !m.vimEnabled || m.mode != ModeVisual {
		return m.cursor, m.cursor
	}
	lo, hi := m.visualAnchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

// selectedRefs returns the secret name+environment pairs in the current selection.
func (m Model) selectedRefs() []secretRef {
	lo, hi := m.selectionRange()
	if lo < 0 || hi >= len(m.display) {
		if m.cursor >= 0 && m.cursor < len(m.display) {
			s := m.display[m.cursor]
			return []secretRef{{Name: s.Name, Environment: s.Environment}}
		}
		return nil
	}
	refs := make([]secretRef, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		refs = append(refs, secretRef{
			Name:        m.display[i].Name,
			Environment: m.display[i].Environment,
		})
	}
	return refs
}

// selectedNames returns the secret names in the current selection (for display).
func (m Model) selectedNames() []string {
	refs := m.selectedRefs()
	names := make([]string, len(refs))
	for i, r := range refs {
		if r.Environment != "" {
			names[i] = r.Name + " [" + r.Environment + "]"
		} else {
			names[i] = r.Name
		}
	}
	return names
}

// currentSecret returns the secret at the cursor position, or nil.
func (m Model) currentSecret() *vault.SecretEntry {
	if m.cursor >= 0 && m.cursor < len(m.display) {
		s := m.display[m.cursor]
		return &s
	}
	return nil
}

// ── Scroll ────────────────────────────────────────────────────

func (m *Model) adjustScroll() {
	vis := m.visibleRows()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+vis {
		m.scrollOffset = m.cursor - vis + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.display) {
		m.cursor = maxVal(0, len(m.display)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) halfPage() int {
	rows := m.visibleRows() / 2
	if rows < 1 {
		return 5
	}
	return rows
}

func (m Model) fullPage() int {
	rows := m.visibleRows()
	if rows < 1 {
		return 10
	}
	return rows
}

// visibleRows estimates how many list rows fit on screen.
func (m Model) visibleRows() int {
	if m.height <= 0 {
		return 15
	}
	// header(3) + search(1) + column headers(1) + footer(3) + padding(2) = 10
	overhead := 10
	// When a toast is active, it takes an extra line above the help bar.
	if m.toast != "" {
		overhead++
	}
	rows := m.height - overhead
	if rows < 1 {
		return 1
	}
	return rows
}

// ── Matching ──────────────────────────────────────────────────

func matchesQuery(s vault.SecretEntry, query string, fuzzy bool) bool {
	q := strings.ToLower(query)
	match := containsMatch
	if fuzzy {
		match = fuzzyMatch
	}
	if match(strings.ToLower(s.Name), q) {
		return true
	}
	if match(strings.ToLower(s.Environment), q) {
		return true
	}
	for k, v := range s.Metadata {
		if match(strings.ToLower(k), q) ||
			match(strings.ToLower(fmt.Sprintf("%v", v)), q) {
			return true
		}
	}
	return false
}

// containsMatch is a simple substring match.
func containsMatch(text, pattern string) bool {
	return strings.Contains(text, pattern)
}

// fuzzyMatch checks whether all characters of pattern appear in text
// in order (not necessarily contiguous). Case-insensitive comparison
// should be done by the caller (pass lowered strings).
func fuzzyMatch(text, pattern string) bool {
	ti := 0
	for pi := 0; pi < len(pattern); pi++ {
		found := false
		for ti < len(text) {
			if text[ti] == pattern[pi] {
				ti++
				found = true
				break
			}
			ti++
		}
		if !found {
			return false
		}
	}
	return true
}

// ── Formatting ────────────────────────────────────────────────

func formatMetadataInline(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(metadata))
	for _, k := range keys {
		parts = append(parts, MetadataKeyStyle.Render(k)+MutedStyle.Render("=")+MetadataValueStyle.Render(fmt.Sprintf("%v", metadata[k])))
	}
	return strings.Join(parts, MutedStyle.Render(", "))
}

func formatMetadataPlain(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(metadata))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, metadata[k]))
	}
	return strings.Join(parts, ", ")
}

// parseMetadataInput parses a comma-separated "key=value" string into a map.
func parseMetadataInput(input string) map[string]any {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	result := make(map[string]any)
	for _, pair := range strings.Split(input, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		if len(parts) == 2 {
			result[key] = strings.TrimSpace(parts[1])
		} else {
			result[key] = ""
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func updateEntry(secrets []vault.SecretEntry, entry *vault.SecretEntry) []vault.SecretEntry {
	for i, s := range secrets {
		if s.Name == entry.Name && s.Environment == entry.Environment {
			secrets[i] = *entry
			return secrets
		}
	}
	return append(secrets, *entry)
}

// ── Numeric helpers ───────────────────────────────────────────

func maxVal(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minVal(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIdx(secrets []vault.SecretEntry) int {
	if len(secrets) == 0 {
		return 0
	}
	return len(secrets) - 1
}

// maskValue returns a fixed-length masked representation regardless of the
// actual value length, so the mask never leaks secret size information.
func maskValue(_ string) string {
	return "●●●●●●●●"
}

// truncate truncates a string to maxLen and appends "..." if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// padRight pads a string with spaces to the given width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// dividerLine creates a horizontal divider line.
func dividerLine(width int) string {
	if width <= 4 {
		width = 40
	}
	return DividerStyle.Render(strings.Repeat("─", width-4))
}

// humanSize formats bytes as a human-readable string.
func humanSize(b int64) string {
	switch {
	case b >= 1<<20:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", float64(b)/float64(1<<20)), "0"), ".") + " MB"
	case b >= 1<<10:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", float64(b)/float64(1<<10)), "0"), ".") + " KB"
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// largeValueThreshold is the character count above which a value is considered
// "large" and routes to multi-line rendering / textarea editing.
const largeValueThreshold = 200

// isLargeValue returns true if the value exceeds the threshold or contains
// newlines, indicating it should use multi-line rendering.
func isLargeValue(v string) bool {
	return len(v) > largeValueThreshold || strings.Contains(v, "\n")
}

// viewportRender crops content to fit within maxVisible lines, returning
// the visible portion with scroll indicators (▲/▼) and the clamped scroll
// offset.
//
// When targetLine >= 0, auto-scrolls to keep that line centered in the
// viewport (useful for edit/view mode focus-following).
// When targetLine < 0, uses scrollOffset directly (useful for manual
// scroll like help overlay).
func viewportRender(content string, maxVisible, scrollOffset, targetLine int) (string, int) {
	if maxVisible <= 0 {
		maxVisible = 1
	}

	lines := strings.Split(content, "\n")
	total := len(lines)

	if total <= maxVisible {
		// Everything fits — no scrolling needed.
		return content, 0
	}

	// Auto-scroll to target line if specified.
	if targetLine >= 0 {
		// Center the target line in the viewport.
		scrollOffset = targetLine - maxVisible/2
	}

	// Clamp scroll offset.
	maxScroll := total - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}

	end := scrollOffset + maxVisible
	if end > total {
		end = total
	}

	visible := lines[scrollOffset:end]
	var b strings.Builder

	// Top scroll indicator — replaces the first visible line so total
	// height stays constant.
	if scrollOffset > 0 {
		aboveHidden := scrollOffset + 1 // lines before window + displaced first line
		b.WriteString(MutedStyle.Render("  ▲ " + fmt.Sprintf("%d more lines above", aboveHidden)))
		b.WriteString("\n")
		visible = visible[1:]
	}

	// Bottom scroll indicator — replaces the last visible line so total
	// height stays constant (fixes off-by-one when both ▲ and ▼ appear).
	remaining := total - end
	if remaining > 0 && len(visible) > 0 {
		visible = visible[:len(visible)-1]
		remaining++ // the displaced line is also not shown
	}

	b.WriteString(strings.Join(visible, "\n"))

	if remaining > 0 {
		if len(visible) > 0 || scrollOffset > 0 {
			b.WriteString("\n")
		}
		b.WriteString(MutedStyle.Render("  ▼ " + fmt.Sprintf("%d more lines below", remaining)))
	}

	return b.String(), scrollOffset
}

// wrapAndTruncate word-wraps value to width columns, then truncates to
// maxLines visible lines. It returns the visible text and the total number of
// wrapped lines.
func wrapAndTruncate(value string, width, maxLines int) (string, int) {
	if width <= 0 {
		width = 60
	}
	var lines []string
	for _, raw := range strings.Split(value, "\n") {
		if len(raw) == 0 {
			lines = append(lines, "")
			continue
		}
		for len(raw) > 0 {
			if len(raw) <= width {
				lines = append(lines, raw)
				break
			}
			// Try to break at the last space within width.
			cut := width
			if idx := strings.LastIndex(raw[:width], " "); idx > 0 {
				cut = idx + 1 // include the space on this line
			}
			lines = append(lines, raw[:cut])
			raw = raw[cut:]
		}
	}
	total := len(lines)
	if maxLines > 0 && total > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n"), total
}

// ── ANSI / visual width helpers ──────────────────────────────

// ansiSeqRe matches ANSI CSI escape sequences.
var ansiSeqRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSISeqs removes ANSI escape sequences from s.
func stripANSISeqs(s string) string {
	return ansiSeqRe.ReplaceAllString(s, "")
}

// runeWidth returns the number of runes in s (visual column count for
// ASCII / single-width characters).
func runeWidth(s string) int {
	return utf8.RuneCountInString(s)
}

// visualLineCount returns how many visual terminal rows the rendered text
// occupies at the given terminal width, accounting for lines that wrap.
func visualLineCount(rendered string, termWidth int) int {
	if termWidth <= 0 {
		termWidth = 80
	}
	total := 0
	for _, line := range strings.Split(rendered, "\n") {
		plain := stripANSISeqs(line)
		w := runeWidth(plain)
		if w <= termWidth {
			total++
		} else {
			total += (w + termWidth - 1) / termWidth
		}
	}
	return total
}
