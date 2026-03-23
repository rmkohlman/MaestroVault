package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── View ──────────────────────────────────────────────────────

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	// Secret modal takes priority over all other overlays.
	if m.showSecretModal {
		modal := m.secretModal.View()
		return m.centerOverlay(modal)
	}

	// Overlays render on top of the current screen.
	if m.showHelp {
		return m.viewHelpOverlay()
	}
	if m.showGenerator {
		return m.viewGeneratorOverlay()
	}
	if m.showInfo {
		return m.viewInfoOverlay()
	}
	if m.showSettings {
		return m.viewSettingsOverlay()
	}

	var s string
	switch m.screen {
	case screenList:
		s = m.viewListScreen()
	case screenDetail:
		s = m.viewDetailScreen()
	case screenConfirmDelete:
		s = m.viewConfirmDeleteScreen()
	default:
		s = m.viewListScreen()
	}

	return s
}

// ── List screen ───────────────────────────────────────────────

func (m Model) viewListScreen() string {
	var b strings.Builder
	w := m.width
	if w <= 0 {
		w = 80
	}

	// Header.
	header := TitleStyle.Render("  MaestroVault")
	if m.vimEnabled {
		header += "  " + m.modeIndicator()
	}
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(dividerLine(w))
	b.WriteString("\n")

	// Search bar.
	if m.searchActive {
		modeTag := MutedStyle.Render("[exact]")
		if m.fuzzySearch {
			modeTag = AccentStyle.Render("[fuzzy]")
		}
		searchLabel := SearchLabelStyle.Render("/")
		b.WriteString(searchLabel + m.searchInput.View() + " " + modeTag)
		b.WriteString("\n")
	} else if q := m.searchInput.Value(); q != "" {
		modeTag := MutedStyle.Render("[exact]")
		if m.fuzzySearch {
			modeTag = AccentStyle.Render("[fuzzy]")
		}
		b.WriteString(SearchLabelStyle.Render("filter: ") + SearchStyle.Render(q) + " " + modeTag)
		b.WriteString("\n")
	}

	// Column headers.
	nameW := m.nameColWidth()
	metaW := m.metadataColWidth()
	colHeader := ColumnHeaderStyle.Render(
		padRight("NAME", nameW) + "  " + padRight("METADATA", metaW) + "  " + "UPDATED",
	)
	b.WriteString(colHeader)
	b.WriteString("\n")

	// Secret list.
	if len(m.display) == 0 {
		empty := MutedStyle.Render("  (no secrets)")
		if m.searchInput.Value() != "" {
			empty = MutedStyle.Render("  no matches for ") + SearchStyle.Render(m.searchInput.Value())
		}
		b.WriteString("\n" + empty + "\n")
	} else {
		vis := m.visibleRows()
		lo := m.scrollOffset
		hi := minVal(lo+vis, len(m.display))

		for i := lo; i < hi; i++ {
			s := m.display[i]
			cursor := "  "
			style := lipgloss.NewStyle()

			selected := false
			if m.vimEnabled && m.mode == ModeVisual {
				sLo, sHi := m.selectionRange()
				if i >= sLo && i <= sHi {
					selected = true
				}
			}

			if i == m.cursor {
				cursor = AccentStyle.Render("> ")
				style = style.Foreground(ColorAccent).Bold(true)
			} else if selected {
				cursor = AccentStyle.Render("  ")
				style = style.Foreground(ColorSecondary)
			}

			// Build name with environment badge and field count.
			nameStr := s.Name
			envBadge := ""
			if s.Environment != "" {
				envBadge = " " + EnvBadgeStyle.Render("["+s.Environment+"]")
			}
			fieldBadge := ""
			if s.FieldCount > 0 {
				fieldBadge = " " + FieldCountStyle.Render(fmt.Sprintf("{%d}", s.FieldCount))
			}
			// Truncate the name portion, then append badges.
			// We need to account for the badge widths in truncation.
			badgeLen := 0
			if s.Environment != "" {
				badgeLen = len(s.Environment) + 3 // " [" + env + "]"
			}
			if s.FieldCount > 0 {
				badgeLen += len(fmt.Sprintf(" {%d}", s.FieldCount))
			}
			nameDisplay := truncate(nameStr, nameW-badgeLen) + envBadge + fieldBadge

			meta := truncate(formatMetadataPlain(s.Metadata), metaW)
			updated := ""
			if s.UpdatedAt != "" {
				// Show just the date portion.
				if len(s.UpdatedAt) >= 10 {
					updated = s.UpdatedAt[:10]
				} else {
					updated = s.UpdatedAt
				}
			}

			row := cursor + style.Render(padRight(nameDisplay, nameW)) +
				"  " + MutedStyle.Render(padRight(meta, metaW)) +
				"  " + MutedStyle.Render(updated)
			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	// Status bar.
	b.WriteString(dividerLine(w))
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	// Toast (above help bar so it never pushes help off-screen).
	if t := m.renderToast(); t != "" {
		b.WriteString(t)
		b.WriteString("\n")
	}

	// Help bar.
	if m.vimEnabled {
		b.WriteString(m.vimHelpBar())
	} else {
		b.WriteString(m.helpBar(
			"↑/↓", "move",
			"↵", "view",
			"a", "add",
			"e", "edit",
			"c", "copy",
			"d", "del",
			"/", "search",
			"s", "sort",
			"S", "settings",
			"?", "help",
			"q", "quit",
		))
	}

	return b.String()
}

// ── Detail screen ─────────────────────────────────────────────

func (m Model) viewDetailScreen() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	s := m.currentSecret()
	if s == nil {
		return MutedStyle.Render("No secret selected.")
	}

	// ── Header (fixed) ────────────────────────────────────────
	var header strings.Builder
	header.WriteString(TitleStyle.Render("  " + s.Name))
	if s.Environment != "" {
		header.WriteString("  " + EnvBadgeStyle.Render("["+s.Environment+"]"))
	}
	header.WriteString("\n")
	header.WriteString(dividerLine(w))
	headerStr := strings.TrimRight(header.String(), "\n")

	// ── Footer (fixed) ────────────────────────────────────────
	var footer strings.Builder
	footer.WriteString(dividerLine(w))
	footer.WriteString("\n")

	// Toast (above help bar so it never pushes help off-screen).
	if t := m.renderToast(); t != "" {
		footer.WriteString(t)
		footer.WriteString("\n")
	}

	// Help bar.
	if m.vimEnabled {
		footer.WriteString(m.vimHelpBar())
	} else {
		footer.WriteString(m.helpBar(
			"esc", "back",
			"space", "peek",
			"c", "copy",
			"e", "edit",
			"d", "delete",
			"q", "quit",
		))
	}
	footerStr := strings.TrimLeft(footer.String(), "\n")

	// ── Body (scrollable) ─────────────────────────────────────
	var body strings.Builder

	// Environment.
	if s.Environment != "" {
		body.WriteString(MutedStyle.Render("  Environment: "))
		body.WriteString(EnvBadgeStyle.Render(s.Environment))
		body.WriteString("\n")
	}

	// Value.
	if m.valueMasked {
		body.WriteString(MutedStyle.Render("  Value: "))
		body.WriteString(MaskedValueStyle.Render(maskValue(s.Value)))
		body.WriteString("\n")
	} else if isLargeValue(s.Value) {
		// Large or multi-line values: wrap to terminal width and render
		// each line individually so viewportRender can crop by newline.
		// Without this, a single long line (e.g., JWT, PEM on one line)
		// would count as 1 newline-delimited line but wrap to dozens of
		// visual lines in the terminal, causing overflow.
		contentWidth := w - 4 // indent
		if contentWidth < 20 {
			contentWidth = 20
		}
		body.WriteString(MutedStyle.Render("  Value:"))
		body.WriteString("\n")
		wrapped, _ := wrapAndTruncate(s.Value, contentWidth, 0)
		for _, line := range strings.Split(wrapped, "\n") {
			body.WriteString("    " + SecretValueStyle.Render(line))
			body.WriteString("\n")
		}
	} else {
		body.WriteString(MutedStyle.Render("  Value: "))
		body.WriteString(SecretValueStyle.Render(s.Value))
		body.WriteString("\n")
	}

	// Metadata.
	if len(s.Metadata) > 0 {
		body.WriteString(MutedStyle.Render("  Metadata: "))
		body.WriteString(formatMetadataInline(s.Metadata))
		body.WriteString("\n")
	}

	// Fields.
	if len(s.Fields) > 0 {
		body.WriteString(MutedStyle.Render("  Fields:"))
		body.WriteString("\n")
		fieldContentWidth := w - 6 // "      " prefix = 6 chars
		if fieldContentWidth < 20 {
			fieldContentWidth = 20
		}
		keys := sortedFieldKeys(s.Fields)
		for _, key := range keys {
			if m.valueMasked {
				body.WriteString("    " + FieldKeyStyle.Render(key) + " ")
				body.WriteString(MaskedValueStyle.Render(maskValue(s.Fields[key])))
				body.WriteString("\n")
			} else if isLargeValue(s.Fields[key]) {
				body.WriteString("    " + FieldKeyStyle.Render(key))
				body.WriteString("\n")
				wrapped, _ := wrapAndTruncate(s.Fields[key], fieldContentWidth, 0)
				for _, line := range strings.Split(wrapped, "\n") {
					body.WriteString("      " + SecretValueStyle.Render(line))
					body.WriteString("\n")
				}
			} else {
				body.WriteString("    " + FieldKeyStyle.Render(key) + " ")
				body.WriteString(SecretValueStyle.Render(s.Fields[key]))
				body.WriteString("\n")
			}
		}
	}

	// Timestamps.
	if s.CreatedAt != "" {
		body.WriteString(MutedStyle.Render("  Created: ") + s.CreatedAt)
		body.WriteString("\n")
	}
	if s.UpdatedAt != "" {
		body.WriteString(MutedStyle.Render("  Updated: ") + s.UpdatedAt)
		body.WriteString("\n")
	}

	bodyStr := strings.TrimRight(body.String(), "\n")

	// ── Compose with viewport constraint ──────────────────────
	// Use visual line count (accounting for terminal wrapping) rather
	// than raw newline count. Lines wider than the terminal width wrap
	// to extra visual rows — we must account for this or the body gets
	// too many lines and the output overflows the terminal.
	headerVisual := visualLineCount(headerStr, w)
	footerVisual := visualLineCount(footerStr, w)
	// +2 for the "\n" separators between header→body and body→footer in
	// the final composition. These separators each consume a visual row
	// because they terminate the last line of the preceding section and
	// start the first line of the next section on its own row.
	availableHeight := h - headerVisual - footerVisual - 2
	if availableHeight < 3 {
		availableHeight = 3
	}

	constrainedBody, _ := viewportRender(bodyStr, availableHeight, m.detailScroll, -1)

	var out strings.Builder
	out.WriteString(headerStr)
	out.WriteString("\n")
	out.WriteString(constrainedBody)
	out.WriteString("\n")
	out.WriteString(footerStr)

	return out.String()
}

// ── Confirm delete screen ─────────────────────────────────────

func (m Model) viewConfirmDeleteScreen() string {
	var b strings.Builder
	w := m.width
	if w <= 0 {
		w = 80
	}

	names := m.selectedNames()
	b.WriteString(WarningStyle.Render("  Delete Confirmation"))
	b.WriteString("\n")
	b.WriteString(dividerLine(w))
	b.WriteString("\n\n")

	if len(names) == 1 {
		b.WriteString(fmt.Sprintf("  Delete secret %s?\n\n",
			AccentStyle.Render(names[0])))
	} else {
		b.WriteString(fmt.Sprintf("  Delete %d secrets?\n\n",
			len(names)))
		for _, n := range names {
			b.WriteString("    " + AccentStyle.Render("• ") + n + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("  " + WarningStyle.Render("This action cannot be undone."))
	b.WriteString("\n\n")

	// Toast (above help bar so it never pushes help off-screen).
	if t := m.renderToast(); t != "" {
		b.WriteString(t)
		b.WriteString("\n")
	}

	b.WriteString(m.helpBar("y/↵", "confirm", "n/esc", "cancel"))

	return b.String()
}

// ── Help overlay ──────────────────────────────────────────────

func (m Model) viewHelpOverlay() string {
	var b strings.Builder
	w := 60

	b.WriteString(TitleStyle.Render(" Keybindings"))
	b.WriteString("\n\n")

	if m.vimEnabled {
		b.WriteString(AccentStyle.Render(" Normal Mode (List)") + "\n")
		b.WriteString(helpLine("j / k", "Move cursor down / up"))
		b.WriteString(helpLine("gg / G", "Go to top / bottom"))
		b.WriteString(helpLine("ctrl+d/u", "Half page down / up"))
		b.WriteString(helpLine("ctrl+f/b", "Full page down / up"))
		b.WriteString(helpLine("l / Enter", "View secret detail"))
		b.WriteString(helpLine("i / a / o", "Add new secret"))
		b.WriteString(helpLine("e", "Edit current secret"))
		b.WriteString(helpLine("c", "Copy to clipboard"))
		b.WriteString(helpLine("dd / x", "Delete secret(s)"))
		b.WriteString(helpLine("v / V", "Enter visual mode"))
		b.WriteString(helpLine("/", "Search / filter"))
		b.WriteString(helpLine("Tab", "Toggle fuzzy / exact search"))
		b.WriteString(helpLine("s", "Cycle sort order"))
		b.WriteString(helpLine("n", "Open password generator"))
		b.WriteString(helpLine("I", "Vault info"))
		b.WriteString(helpLine("S", "Settings"))
		b.WriteString(helpLine("r", "Refresh list"))
		b.WriteString(helpLine("?", "Toggle this help"))
		b.WriteString(helpLine("q", "Quit"))
		b.WriteString("\n")
		b.WriteString(AccentStyle.Render(" Normal Mode (Detail)") + "\n")
		b.WriteString(helpLine("h / Esc", "Back to list"))
		b.WriteString(helpLine("Space", "Toggle value mask"))
		b.WriteString(helpLine("c", "Copy value"))
		b.WriteString(helpLine("e", "Edit secret"))
		b.WriteString(helpLine("d", "Delete secret"))
		b.WriteString("\n")
		b.WriteString(AccentStyle.Render(" Visual Mode") + "\n")
		b.WriteString(helpLine("j / k", "Extend selection"))
		b.WriteString(helpLine("d / x", "Delete selected"))
		b.WriteString(helpLine("Esc", "Back to normal"))
		b.WriteString("\n")
		b.WriteString(AccentStyle.Render(" Secret Modal (Add/Edit/View)") + "\n")
		b.WriteString(helpLine("↑ / ↓ / Tab", "Navigate fields"))
		b.WriteString(helpLine("ctrl+r", "Toggle value visibility"))
		b.WriteString(helpLine("ctrl+A", "Add field (edit/add mode)"))
		b.WriteString(helpLine("ctrl+d", "Remove field (edit/add mode)"))
		b.WriteString(helpLine("Enter", "Save"))
		b.WriteString(helpLine("p / Space", "Peek value (view mode)"))
		b.WriteString(helpLine("c", "Copy value (view mode)"))
		b.WriteString(helpLine("e", "Edit (from view mode)"))
		b.WriteString(helpLine("Esc", "Cancel / close"))
		b.WriteString("\n")
		b.WriteString(AccentStyle.Render(" Generator") + "\n")
		b.WriteString(helpLine("j / k", "Navigate options"))
		b.WriteString(helpLine("h / l", "Adjust length"))
		b.WriteString(helpLine("Space", "Toggle option"))
		b.WriteString(helpLine("r", "Regenerate"))
		b.WriteString(helpLine("c", "Copy to clipboard"))
		b.WriteString(helpLine("Enter", "Save (if named)"))
		b.WriteString(helpLine("Esc", "Close"))
	} else {
		b.WriteString(AccentStyle.Render(" List") + "\n")
		b.WriteString(helpLine("↑ / ↓", "Move cursor"))
		b.WriteString(helpLine("Enter", "View secret"))
		b.WriteString(helpLine("a", "Add new secret"))
		b.WriteString(helpLine("e", "Edit secret"))
		b.WriteString(helpLine("c", "Copy to clipboard"))
		b.WriteString(helpLine("d", "Delete secret"))
		b.WriteString(helpLine("/", "Search / filter"))
		b.WriteString(helpLine("Tab", "Toggle fuzzy / exact search"))
		b.WriteString(helpLine("s", "Cycle sort order"))
		b.WriteString(helpLine("n", "Password generator"))
		b.WriteString(helpLine("S", "Settings"))
		b.WriteString(helpLine("?", "Toggle this help"))
		b.WriteString(helpLine("r", "Refresh"))
		b.WriteString(helpLine("q", "Quit"))
		b.WriteString("\n")
		b.WriteString(AccentStyle.Render(" Detail") + "\n")
		b.WriteString(helpLine("Esc", "Back to list"))
		b.WriteString(helpLine("Space", "Toggle value mask"))
		b.WriteString(helpLine("c", "Copy value"))
		b.WriteString(helpLine("e", "Edit"))
		b.WriteString(helpLine("d", "Delete"))
	}

	b.WriteString("\n")
	b.WriteString(MutedStyle.Render(" Press ? or Esc to close  ·  j/k to scroll"))

	content := b.String()

	// Constrain to terminal height: modal border/padding (4) + centering margin (2).
	maxVisible := m.height - 6
	if maxVisible < 10 {
		maxVisible = 10
	}
	content, _ = viewportRender(content, maxVisible, m.helpScroll, -1)

	modal := ModalStyle.Width(w).Render(content)

	return m.centerOverlay(modal)
}

// ── Generator overlay ─────────────────────────────────────────

func (m Model) viewGeneratorOverlay() string {
	var b strings.Builder
	w := 56

	b.WriteString(TitleStyle.Render(" Password Generator"))
	b.WriteString("\n\n")

	// Preview.
	preview := m.gen.preview
	if len(preview) > w-4 {
		preview = preview[:w-7] + "..."
	}
	b.WriteString("  " + SecretValueStyle.Render(preview))
	b.WriteString("\n\n")

	// Options.
	genOpts := []struct {
		label string
		value string
		idx   int
	}{
		{"Length", fmt.Sprintf("◄ %d ►", m.gen.length), genOptLength},
		{"Uppercase [A-Z]", m.checkMark(m.gen.uppercase), genOptUppercase},
		{"Lowercase [a-z]", m.checkMark(m.gen.lowercase), genOptLowercase},
		{"Digits [0-9]", m.checkMark(m.gen.digits), genOptDigits},
		{"Symbols [!@#...]", m.checkMark(m.gen.symbols), genOptSymbols},
	}

	for _, opt := range genOpts {
		cursor := "  "
		labelStyle := GenLabelStyle
		valueStyle := GenInactiveStyle
		if m.gen.cursor == opt.idx {
			cursor = AccentStyle.Render("▸ ")
			labelStyle = GenActiveStyle
			valueStyle = GenValueStyle
		}
		b.WriteString(cursor + labelStyle.Render(padRight(opt.label, 20)) + " " + valueStyle.Render(opt.value))
		b.WriteString("\n")
	}

	// Name input.
	b.WriteString("\n")
	nameCursor := "  "
	nameLabel := GenLabelStyle
	if m.gen.cursor == genOptName {
		nameCursor = AccentStyle.Render("▸ ")
		nameLabel = GenActiveStyle
	}
	b.WriteString(nameCursor + nameLabel.Render("Save as: ") + m.gen.nameInput.View())
	b.WriteString("\n")

	// Environment input.
	envCursor := "  "
	envLabel := GenLabelStyle
	if m.gen.cursor == genOptEnv {
		envCursor = AccentStyle.Render("▸ ")
		envLabel = GenActiveStyle
	}
	b.WriteString(envCursor + envLabel.Render("Env:     ") + m.gen.envInput.View())
	b.WriteString("\n")

	// Toast (inline error).
	if m.gen.toast != "" {
		b.WriteString("  " + ToastErrorStyle.Render(" "+m.gen.toast+" "))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Hints / save feedback.
	if m.gen.saving {
		b.WriteString("  " + ToastInfoStyle.Render(" Saving... "))
	} else if m.gen.savedMsg != "" {
		b.WriteString("  " + ToastSuccessStyle.Render(" ✓ "+m.gen.savedMsg+" "))
	} else {
		b.WriteString(m.helpBar(
			"j/k", "navigate",
			"h/l", "adjust",
			"space", "toggle",
			"r", "regen",
			"c", "copy",
			"↵", "save",
			"esc", "close",
		))
	}

	content := b.String()

	// Safety cap: constrain to terminal height for very small terminals.
	maxVisible := m.height - 6
	if maxVisible < 10 {
		maxVisible = 10
	}
	contentLines := strings.Count(content, "\n") + 1
	if contentLines > maxVisible {
		content, _ = viewportRender(content, maxVisible, 0, -1)
	}

	modal := ModalStyle.Width(w).Render(content)

	return m.centerOverlay(modal)
}

// ── Info overlay ──────────────────────────────────────────────

func (m Model) viewInfoOverlay() string {
	var b strings.Builder
	w := 50

	b.WriteString(TitleStyle.Render(" Vault Info"))
	b.WriteString("\n\n")

	if m.vaultInfo != nil {
		info := m.vaultInfo
		b.WriteString(infoLine("Directory", info.Dir))
		b.WriteString(infoLine("Database", info.DBPath))
		b.WriteString(infoLine("DB Size", humanSize(info.DBSize)))
		b.WriteString(infoLine("Secrets", fmt.Sprintf("%d", info.SecretCount)))
	} else {
		b.WriteString(MutedStyle.Render("  Loading..."))
	}

	b.WriteString("\n")
	b.WriteString(MutedStyle.Render(" Press Esc to close"))

	content := b.String()

	// Safety cap: constrain to terminal height for very small terminals.
	maxVisible := m.height - 6
	if maxVisible < 10 {
		maxVisible = 10
	}
	contentLines := strings.Count(content, "\n") + 1
	if contentLines > maxVisible {
		content, _ = viewportRender(content, maxVisible, 0, -1)
	}

	modal := ModalStyle.Width(w).Render(content)

	return m.centerOverlay(modal)
}

// ── Settings overlay ──────────────────────────────────────────

func (m Model) viewSettingsOverlay() string {
	var b strings.Builder
	w := 50

	b.WriteString(TitleStyle.Render(" Settings"))
	b.WriteString("\n\n")

	items := []struct {
		label string
		on    bool
		desc  string
		idx   int
	}{
		{"Vim Mode", m.settingsConfig.VimMode, "Vim keybindings in the TUI", settingVimMode},
		{"TouchID", m.settingsConfig.TouchID, "Biometric auth on vault open", settingTouchID},
		{"Fuzzy Search", m.settingsConfig.FuzzySearch, "Default search to fuzzy matching", settingFuzzySearch},
	}

	for _, item := range items {
		cursor := "  "
		labelStyle := GenLabelStyle
		if m.settingsCursor == item.idx {
			cursor = AccentStyle.Render("▸ ")
			labelStyle = GenActiveStyle
		}
		toggle := m.checkMark(item.on)
		b.WriteString(cursor + labelStyle.Render(padRight(item.label, 16)) + " " + toggle)
		b.WriteString("\n")
		b.WriteString("    " + MutedStyle.Render(item.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(MutedStyle.Render(" Config: ") + MutedStyle.Render("~/.maestrovault/config.json"))
	b.WriteString("\n\n")
	b.WriteString(m.helpBar(
		"j/k", "navigate",
		"space", "toggle",
		"esc", "save & close",
	))

	content := b.String()

	// Safety cap: constrain to terminal height for very small terminals.
	maxVisible := m.height - 6
	if maxVisible < 10 {
		maxVisible = 10
	}
	contentLines := strings.Count(content, "\n") + 1
	if contentLines > maxVisible {
		content, _ = viewportRender(content, maxVisible, 0, -1)
	}

	modal := ModalStyle.Width(w).Render(content)

	return m.centerOverlay(modal)
}

// ── Mode indicator ────────────────────────────────────────────

func (m Model) modeIndicator() string {
	var style lipgloss.Style
	switch m.mode {
	case ModeNormal:
		style = lipgloss.NewStyle().
			Background(ColorBlue).
			Foreground(ColorBlack).
			Bold(true).
			Padding(0, 1)
	case ModeVisual:
		style = lipgloss.NewStyle().
			Background(ColorMagenta).
			Foreground(ColorBlack).
			Bold(true).
			Padding(0, 1)
	default:
		return ""
	}
	return style.Render(m.mode.String())
}

// ── Toast ─────────────────────────────────────────────────────

func (m Model) renderToast() string {
	if m.toast == "" {
		return ""
	}
	switch m.toastKind {
	case "success":
		return ToastSuccessStyle.Render(" ✓ " + m.toast + " ")
	case "error":
		return ToastErrorStyle.Render(" ✗ " + m.toast + " ")
	default:
		return ToastInfoStyle.Render(" ℹ " + m.toast + " ")
	}
}

// ── Status bar ────────────────────────────────────────────────

func (m Model) renderStatusBar() string {
	var parts []string

	count := fmt.Sprintf("%d secret(s)", len(m.display))
	if len(m.display) != len(m.secrets) {
		count = fmt.Sprintf("%d/%d secret(s)", len(m.display), len(m.secrets))
	}
	parts = append(parts, count)

	parts = append(parts, "sort: "+m.sortOrder.String())

	if len(m.display) > 0 {
		pos := fmt.Sprintf("%d/%d", m.cursor+1, len(m.display))
		parts = append(parts, pos)
	}

	return StatusBarStyle.Render("  " + strings.Join(parts, "  ·  "))
}

// ── View helpers ──────────────────────────────────────────────

func helpLine(key, desc string) string {
	return "  " + HelpKeyStyle.Render(padRight(key, 14)) + " " + HelpDescStyle.Render(desc) + "\n"
}

func infoLine(label, value string) string {
	return "  " + MutedStyle.Render(padRight(label+":", 14)) + " " + value + "\n"
}

func (m Model) checkMark(on bool) string {
	if on {
		return GenCheckOnStyle.Render("[✓]")
	}
	return GenCheckOffStyle.Render("[ ]")
}

func (m Model) centerOverlay(content string) string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}

// Column width calculations for the list view.
func (m Model) nameColWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	// NAME gets ~35% of space, min 12.
	col := w * 35 / 100
	if col < 12 {
		col = 12
	}
	if col > 40 {
		col = 40
	}
	return col
}

func (m Model) metadataColWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	// METADATA gets ~30% of space, min 10.
	col := w * 30 / 100
	if col < 10 {
		col = 10
	}
	if col > 35 {
		col = 35
	}
	return col
}
