package tui

import (
	"strings"

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
	return MutedStyle.Render("  ") + strings.Join(parts, MutedStyle.Render("  ·  "))
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
				"g", "generate",
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
	case ModeInsert:
		return m.helpBar(
			"↵", "next/save",
			"esc", "cancel",
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

// selectedNames returns the secret names in the current selection.
func (m Model) selectedNames() []string {
	lo, hi := m.selectionRange()
	if lo < 0 || hi >= len(m.display) {
		if m.cursor >= 0 && m.cursor < len(m.display) {
			return []string{m.display[m.cursor].Name}
		}
		return nil
	}
	names := make([]string, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		names = append(names, m.display[i].Name)
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
	rows := m.height - 10
	if rows < 1 {
		return 1
	}
	return rows
}

// ── Matching ──────────────────────────────────────────────────

func matchesQuery(s vault.SecretEntry, query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(s.Name), q) {
		return true
	}
	for k, v := range s.Labels {
		if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

// ── Formatting ────────────────────────────────────────────────

func formatLabelsInline(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, LabelKeyStyle.Render(k)+MutedStyle.Render("=")+LabelValueStyle.Render(v))
	}
	return strings.Join(parts, MutedStyle.Render(", "))
}

func formatLabelsPlain(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ", ")
}

func updateEntry(secrets []vault.SecretEntry, entry *vault.SecretEntry) []vault.SecretEntry {
	for i, s := range secrets {
		if s.Name == entry.Name {
			secrets[i] = *entry
			return secrets
		}
	}
	return append(secrets, *entry)
}

// ── Numeric helpers ───────────────────────────────────────────

func clamp(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

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

// maskValue returns a masked representation of a value.
func maskValue(v string) string {
	n := len(v)
	if n > 40 {
		n = 40
	}
	if n < 8 {
		n = 8
	}
	return strings.Repeat("●", n)
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
		return strings.TrimRight(strings.TrimRight(
			strings.Replace(
				strings.Replace(
					formatFloat(float64(b)/float64(1<<20)), ".", ".", 1,
				), ",", "", -1,
			), "0"), ".") + " MB"
	case b >= 1<<10:
		return strings.TrimRight(strings.TrimRight(
			formatFloat(float64(b)/float64(1<<10)), "0"), ".") + " KB"
	default:
		return formatInt(b) + " B"
	}
}

func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(
		strings.Replace(
			strings.Replace(
				formatFloatRaw(f), ".", ".", 1,
			), ",", "", -1,
		), "0"), ".")
}

func formatFloatRaw(f float64) string {
	s := ""
	whole := int64(f)
	frac := int64((f - float64(whole)) * 10)
	s = formatInt(whole) + "." + formatInt(frac)
	return s
}

func formatInt(n int64) string {
	if n < 0 {
		return "-" + formatInt(-n)
	}
	s := ""
	for n > 0 || s == "" {
		digit := n % 10
		s = string(rune('0'+digit)) + s
		n /= 10
	}
	return s
}
