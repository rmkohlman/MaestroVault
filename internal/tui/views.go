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

	var s string
	switch m.screen {
	case screenList:
		s = m.viewListScreen()
	case screenDetail:
		s = m.viewDetailScreen()
	case screenSetName, screenSetEnv, screenSetValue, screenSetMetadata:
		s = m.viewSetScreen()
	case screenConfirmDelete:
		s = m.viewConfirmDeleteScreen()
	default:
		s = m.viewListScreen()
	}

	// Append toast if active.
	if m.toast != "" {
		s += "\n" + m.renderToast()
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
		searchLabel := SearchLabelStyle.Render("/")
		b.WriteString(searchLabel + m.searchInput.View())
		b.WriteString("\n")
	} else if q := m.searchInput.Value(); q != "" {
		b.WriteString(SearchLabelStyle.Render("filter: ") + SearchStyle.Render(q))
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

			// Build name with environment badge.
			nameStr := s.Name
			envBadge := ""
			if s.Environment != "" {
				envBadge = " " + EnvBadgeStyle.Render("["+s.Environment+"]")
			}
			// Truncate the name portion, then append badge.
			// We need to account for the badge width in truncation.
			badgeLen := 0
			if s.Environment != "" {
				badgeLen = len(s.Environment) + 3 // " [" + env + "]"
			}
			nameDisplay := truncate(nameStr, nameW-badgeLen) + envBadge

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
			"?", "help",
			"q", "quit",
		))
	}

	return b.String()
}

// ── Detail screen ─────────────────────────────────────────────

func (m Model) viewDetailScreen() string {
	var b strings.Builder
	w := m.width
	if w <= 0 {
		w = 80
	}

	s := m.currentSecret()
	if s == nil {
		return MutedStyle.Render("No secret selected.")
	}

	b.WriteString(TitleStyle.Render("  " + s.Name))
	if s.Environment != "" {
		b.WriteString("  " + EnvBadgeStyle.Render("["+s.Environment+"]"))
	}
	b.WriteString("\n")
	b.WriteString(dividerLine(w))
	b.WriteString("\n\n")

	// Environment.
	if s.Environment != "" {
		b.WriteString(MutedStyle.Render("  Environment: "))
		b.WriteString(EnvBadgeStyle.Render(s.Environment))
		b.WriteString("\n\n")
	}

	// Value.
	b.WriteString(MutedStyle.Render("  Value: "))
	if m.valueMasked {
		b.WriteString(MaskedValueStyle.Render(maskValue(s.Value)))
	} else {
		b.WriteString(SecretValueStyle.Render(s.Value))
	}
	b.WriteString("\n\n")

	// Metadata.
	if len(s.Metadata) > 0 {
		b.WriteString(MutedStyle.Render("  Metadata: "))
		b.WriteString(formatMetadataInline(s.Metadata))
		b.WriteString("\n\n")
	}

	// Timestamps.
	if s.CreatedAt != "" {
		b.WriteString(MutedStyle.Render("  Created: ") + s.CreatedAt)
		b.WriteString("\n")
	}
	if s.UpdatedAt != "" {
		b.WriteString(MutedStyle.Render("  Updated: ") + s.UpdatedAt)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dividerLine(w))
	b.WriteString("\n")

	// Help bar.
	if m.vimEnabled {
		b.WriteString(m.vimHelpBar())
	} else {
		b.WriteString(m.helpBar(
			"esc", "back",
			"space", "peek",
			"c", "copy",
			"e", "edit",
			"d", "delete",
			"q", "quit",
		))
	}

	return b.String()
}

// ── Set screen (add / edit) ───────────────────────────────────

func (m Model) viewSetScreen() string {
	var b strings.Builder
	w := m.width
	if w <= 0 {
		w = 80
	}

	title := "  Add Secret"
	if m.editing {
		title = "  Edit Secret"
	}
	b.WriteString(TitleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(dividerLine(w))
	b.WriteString("\n\n")

	// Step indicator.
	steps := []struct {
		label  string
		screen screen
	}{
		{"1 Name", screenSetName},
		{"2 Environment", screenSetEnv},
		{"3 Value", screenSetValue},
		{"4 Metadata", screenSetMetadata},
	}
	var stepParts []string
	for _, step := range steps {
		if m.screen == step.screen {
			stepParts = append(stepParts, AccentStyle.Render(step.label))
		} else {
			stepParts = append(stepParts, MutedStyle.Render(step.label))
		}
	}
	b.WriteString("  " + strings.Join(stepParts, MutedStyle.Render(" → ")) + "\n\n")

	// Name field.
	nameLabel := MutedStyle.Render("  Name:     ")
	if m.screen == screenSetName {
		nameLabel = AccentStyle.Render("▸ Name:     ")
	}
	b.WriteString(nameLabel + m.nameInput.View())
	b.WriteString("\n\n")

	// Environment field.
	envLabel := MutedStyle.Render("  Env:      ")
	if m.screen == screenSetEnv {
		envLabel = AccentStyle.Render("▸ Env:      ")
	}
	b.WriteString(envLabel + m.envInput.View())
	b.WriteString("\n\n")

	// Value field.
	valueLabel := MutedStyle.Render("  Value:    ")
	if m.screen == screenSetValue {
		if m.valueRevealed {
			valueLabel = AccentStyle.Render("▸ Value:    ") + MutedStyle.Render("(visible) ")
		} else {
			valueLabel = AccentStyle.Render("▸ Value:    ")
		}
	}
	b.WriteString(valueLabel + m.valueInput.View())
	b.WriteString("\n\n")

	// Metadata field.
	metaLabel := MutedStyle.Render("  Metadata: ")
	if m.screen == screenSetMetadata {
		metaLabel = AccentStyle.Render("▸ Metadata: ")
	}
	b.WriteString(metaLabel + m.metadataInput.View())
	b.WriteString("\n\n")

	b.WriteString(dividerLine(w))
	b.WriteString("\n")

	if m.vimEnabled {
		if m.screen == screenSetValue {
			b.WriteString(m.helpBar("↵", "next/save", "ctrl+r", "peek", "esc", "cancel"))
		} else {
			b.WriteString(m.vimHelpBar())
		}
	} else {
		if m.screen == screenSetValue {
			b.WriteString(m.helpBar("↵", "next/save", "ctrl+r", "peek", "esc", "cancel"))
		} else {
			b.WriteString(m.helpBar("↵", "next/save", "esc", "cancel"))
		}
	}

	return b.String()
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
		b.WriteString(helpLine("Ctrl+d/u", "Half page down / up"))
		b.WriteString(helpLine("Ctrl+f/b", "Full page down / up"))
		b.WriteString(helpLine("l / Enter", "View secret detail"))
		b.WriteString(helpLine("i / a / o", "Add new secret"))
		b.WriteString(helpLine("e", "Edit current secret"))
		b.WriteString(helpLine("c", "Copy to clipboard"))
		b.WriteString(helpLine("dd / x", "Delete secret(s)"))
		b.WriteString(helpLine("v / V", "Enter visual mode"))
		b.WriteString(helpLine("/", "Search / filter"))
		b.WriteString(helpLine("s", "Cycle sort order"))
		b.WriteString(helpLine("n", "Open password generator"))
		b.WriteString(helpLine("I", "Vault info"))
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
		b.WriteString(AccentStyle.Render(" Insert Mode") + "\n")
		b.WriteString(helpLine("Enter", "Next field / save"))
		b.WriteString(helpLine("Esc", "Cancel"))
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
		b.WriteString(helpLine("s", "Cycle sort order"))
		b.WriteString(helpLine("n", "Password generator"))
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
	b.WriteString(MutedStyle.Render(" Press ? or Esc to close"))

	content := b.String()
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
	b.WriteString("\n\n")

	// Hints.
	b.WriteString(m.helpBar(
		"j/k", "navigate",
		"h/l", "adjust",
		"space", "toggle",
		"r", "regen",
		"c", "copy",
		"↵", "save",
		"esc", "close",
	))

	content := b.String()
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
	case ModeInsert:
		style = lipgloss.NewStyle().
			Background(ColorGreen).
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
	// Vertically center the modal.
	contentHeight := strings.Count(content, "\n") + 1
	topPad := 0
	if m.height > contentHeight+2 {
		topPad = (m.height - contentHeight) / 3
	}
	return strings.Repeat("\n", topPad) + content
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
