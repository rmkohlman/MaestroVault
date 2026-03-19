package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// SettingsModel is a standalone bubbletea model for the `mav settings` command.
type SettingsModel struct {
	config vault.Config
	cursor int
	width  int
	height int
	toast  string
}

// NewSettingsModel creates a SettingsModel pre-loaded with the given config.
func NewSettingsModel(cfg vault.Config) SettingsModel {
	return SettingsModel{config: cfg}
}

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case configSavedMsg:
		return m, tea.Quit
	case errMsg:
		m.toast = msg.err.Error()
		return m, nil
	}
	return m, nil
}

func (m SettingsModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch msg.String() {
	case "j", "down":
		m.cursor = (m.cursor + 1) % settingCount
	case "k", "up":
		m.cursor = (m.cursor - 1 + settingCount) % settingCount
	case " ", "enter":
		switch m.cursor {
		case settingVimMode:
			m.config.VimMode = !m.config.VimMode
		case settingTouchID:
			m.config.TouchID = !m.config.TouchID
		case settingFuzzySearch:
			m.config.FuzzySearch = !m.config.FuzzySearch
		}
	case "esc", "q":
		return m, saveConfig(m.config)
	}
	return m, nil
}

func (m SettingsModel) View() string {
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
		{"Vim Mode", m.config.VimMode, "Vim keybindings in the TUI", settingVimMode},
		{"TouchID", m.config.TouchID, "Biometric auth on vault open", settingTouchID},
		{"Fuzzy Search", m.config.FuzzySearch, "Default search to fuzzy matching", settingFuzzySearch},
	}

	for _, item := range items {
		cursor := "  "
		labelStyle := GenLabelStyle
		if m.cursor == item.idx {
			cursor = AccentStyle.Render("▸ ")
			labelStyle = GenActiveStyle
		}
		toggle := settingsCheckMark(item.on)
		b.WriteString(cursor + labelStyle.Render(padRight(item.label, 16)) + " " + toggle)
		b.WriteString("\n")
		b.WriteString("    " + MutedStyle.Render(item.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(MutedStyle.Render(" Config: ") + MutedStyle.Render(vault.ConfigPath()))
	b.WriteString("\n\n")

	// Help bar.
	helpParts := []string{
		HelpKeyStyle.Render("j/k") + " " + HelpDescStyle.Render("navigate"),
		HelpKeyStyle.Render("space") + " " + HelpDescStyle.Render("toggle"),
		HelpKeyStyle.Render("esc") + " " + HelpDescStyle.Render("save & close"),
	}
	b.WriteString("  " + strings.Join(helpParts, MutedStyle.Render("  ·  ")))

	if m.toast != "" {
		b.WriteString("\n\n")
		b.WriteString("  " + ToastErrorStyle.Render(" "+m.toast+" "))
	}

	content := b.String()
	modal := ModalStyle.Width(w).Render(content)

	tw := m.width
	if tw <= 0 {
		tw = 80
	}
	th := m.height
	if th <= 0 {
		th = 24
	}
	return lipgloss.Place(tw, th, lipgloss.Center, lipgloss.Center, modal)
}

// settingsCheckMark renders a toggle indicator.
func settingsCheckMark(on bool) string {
	if on {
		return GenCheckOnStyle.Render("[✓]")
	}
	return GenCheckOffStyle.Render("[ ]")
}
