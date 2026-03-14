package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rmkohlman/MaestroVault/internal/clipboard"
	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// ── Secret modal modes ────────────────────────────────────────

type secretModalMode int

const (
	modalView secretModalMode = iota
	modalEdit
	modalAdd
)

// ── Field indices for edit/add form ───────────────────────────

const (
	fieldName     = 0
	fieldEnv      = 1
	fieldValue    = 2
	fieldMetadata = 3
	fieldCount    = 4
)

// ── Messages ──────────────────────────────────────────────────

// secretModalResultMsg is sent when the modal wants to close and report a result.
type secretModalResultMsg struct {
	saved bool   // true if a secret was saved
	toast string // toast text to show after close
	kind  string // "success", "error", "info"
}

// secretModalCloseMsg requests the modal be closed with no side effects.
type secretModalCloseMsg struct{}

// secretModalDoneMsg is sent after the brief "saved" flash to trigger modal close.
type secretModalDoneMsg struct {
	toast string
	kind  string
	saved bool
}

// ── SecretModal ───────────────────────────────────────────────

// SecretModal is a unified bubbletea model for viewing, editing, and adding secrets.
// It can be rendered as an overlay inside the TUI or wrapped in tea.NewProgram
// for standalone CLI usage.
type SecretModal struct {
	vault vault.Vault
	mode  secretModalMode

	// Entry being viewed/edited (nil for add mode).
	entry *vault.SecretEntry

	// Original values for edit mode (to detect rename).
	origName string
	origEnv  string

	// Form inputs (edit + add modes).
	nameInput     textinput.Model
	envInput      textinput.Model
	valueInput    textinput.Model
	metadataInput textinput.Model
	focusField    int  // which field has focus (fieldName..fieldMetadata)
	valueRevealed bool // true when Ctrl+R has been pressed to peek

	// View mode state.
	valueMasked bool // value masked in view mode

	// Dimensions.
	width  int
	height int

	// Standalone mode (when used from CLI).
	standalone bool

	// Toast for inline error display.
	toast     string
	toastKind string

	// Save feedback state.
	saving    bool   // true while save cmd is in-flight
	savedMsg  string // set briefly after successful save
	savedKind string // "success"
}

// ── Constructors ──────────────────────────────────────────────

// NewSecretModalView creates a modal in view mode for the given secret.
func NewSecretModalView(entry *vault.SecretEntry, v vault.Vault) SecretModal {
	m := SecretModal{
		vault:       v,
		mode:        modalView,
		entry:       entry,
		valueMasked: true,
	}
	m.initInputs()
	return m
}

// NewSecretModalEdit creates a modal in edit mode, pre-populated with the given secret.
func NewSecretModalEdit(entry *vault.SecretEntry, v vault.Vault) SecretModal {
	m := SecretModal{
		vault:    v,
		mode:     modalEdit,
		entry:    entry,
		origName: entry.Name,
		origEnv:  entry.Environment,
	}
	m.initInputs()
	m.nameInput.SetValue(entry.Name)
	m.envInput.SetValue(entry.Environment)
	m.valueInput.SetValue(entry.Value)
	m.metadataInput.SetValue(formatMetadataPlain(entry.Metadata))
	m.focusField = fieldName
	m.focusCurrentField()
	return m
}

// NewSecretModalAdd creates a modal in add mode with empty fields.
func NewSecretModalAdd(v vault.Vault) SecretModal {
	m := SecretModal{
		vault: v,
		mode:  modalAdd,
	}
	m.initInputs()
	m.focusField = fieldName
	m.focusCurrentField()
	return m
}

func (m *SecretModal) initInputs() {
	ni := textinput.New()
	ni.Placeholder = "secret-name"
	ni.CharLimit = 128

	ei := textinput.New()
	ei.Placeholder = "environment (optional)"
	ei.CharLimit = 128

	vi := textinput.New()
	vi.Placeholder = "secret value"
	vi.CharLimit = 4096
	vi.EchoMode = textinput.EchoPassword

	mi := textinput.New()
	mi.Placeholder = "key=value, key=value (optional)"
	mi.CharLimit = 4096

	m.nameInput = ni
	m.envInput = ei
	m.valueInput = vi
	m.metadataInput = mi
}

func (m *SecretModal) focusCurrentField() {
	m.nameInput.Blur()
	m.envInput.Blur()
	m.valueInput.Blur()
	m.metadataInput.Blur()

	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldEnv:
		m.envInput.Focus()
	case fieldValue:
		m.valueInput.Focus()
	case fieldMetadata:
		m.metadataInput.Focus()
	}
}

// SetStandalone marks this modal as running standalone (CLI alt-screen mode).
func (m *SecretModal) SetStandalone(b bool) {
	m.standalone = b
}

// SetName pre-fills the name input field (useful when launching add mode from CLI).
func (m *SecretModal) SetName(name string) {
	m.nameInput.SetValue(name)
}

// SetEnv pre-fills the environment input field.
func (m *SecretModal) SetEnv(env string) {
	m.envInput.SetValue(env)
}

// ── Init ──────────────────────────────────────────────────────

func (m SecretModal) Init() tea.Cmd {
	if m.mode == modalEdit || m.mode == modalAdd {
		return textinput.Blink
	}
	return nil
}

// ── Update ────────────────────────────────────────────────────

func (m SecretModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case secretModalResultMsg:
		// Reached only in standalone mode (parent intercepts in overlay mode).
		m.saving = false
		if msg.saved {
			m.savedMsg = msg.toast
			m.savedKind = msg.kind
			return m, tea.Tick(700*time.Millisecond, func(t time.Time) tea.Msg {
				return secretModalDoneMsg{}
			})
		}
		m.toast = msg.toast
		m.toastKind = msg.kind
		return m, nil

	case secretModalDoneMsg:
		// Reached only in standalone mode.
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward non-key messages to the focused text input.
	if m.mode == modalEdit || m.mode == modalAdd {
		return m.forwardToInput(msg)
	}

	return m, nil
}

func (m SecretModal) forwardToInput(msg tea.Msg) (SecretModal, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusField {
	case fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case fieldEnv:
		m.envInput, cmd = m.envInput.Update(msg)
	case fieldValue:
		m.valueInput, cmd = m.valueInput.Update(msg)
	case fieldMetadata:
		m.metadataInput, cmd = m.metadataInput.Update(msg)
	}
	return m, cmd
}

func (m SecretModal) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		if m.standalone {
			return m, tea.Quit
		}
		return m, func() tea.Msg { return secretModalCloseMsg{} }
	}

	switch m.mode {
	case modalView:
		return m.handleViewKey(msg)
	case modalEdit:
		return m.handleEditKey(msg)
	case modalAdd:
		return m.handleAddKey(msg)
	}
	return m, nil
}

// ── View mode key handling ────────────────────────────────────

func (m SecretModal) handleViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "p", " ":
		m.valueMasked = !m.valueMasked
	case "c":
		if m.entry != nil && m.entry.Value != "" {
			return m, m.copyValue()
		}
	case "e":
		if m.entry != nil {
			m.mode = modalEdit
			m.origName = m.entry.Name
			m.origEnv = m.entry.Environment
			m.nameInput.SetValue(m.entry.Name)
			m.envInput.SetValue(m.entry.Environment)
			m.valueInput.SetValue(m.entry.Value)
			m.metadataInput.SetValue(formatMetadataPlain(m.entry.Metadata))
			m.focusField = fieldName
			m.focusCurrentField()
			return m, textinput.Blink
		}
	case "q", "esc":
		return m.closeModal()
	}
	return m, nil
}

// ── Edit mode key handling ────────────────────────────────────

func (m SecretModal) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block input during save or saved flash.
	if m.saving || m.savedMsg != "" {
		return m, nil
	}

	switch {
	case msg.Type == tea.KeyCtrlR:
		m.valueRevealed = !m.valueRevealed
		if m.valueRevealed {
			m.valueInput.EchoMode = textinput.EchoNormal
		} else {
			m.valueInput.EchoMode = textinput.EchoPassword
		}
		return m, nil

	case msg.Type == tea.KeyUp || msg.String() == "shift+tab":
		m.focusField = (m.focusField - 1 + fieldCount) % fieldCount
		m.focusCurrentField()
		return m, textinput.Blink

	case msg.Type == tea.KeyDown || msg.Type == tea.KeyTab:
		m.focusField = (m.focusField + 1) % fieldCount
		m.focusCurrentField()
		return m, textinput.Blink

	case msg.Type == tea.KeyEnter:
		return m.saveSecret()

	case msg.Type == tea.KeyEscape:
		// If we came from view mode, go back to view.
		if m.entry != nil {
			m.mode = modalView
			m.valueMasked = true
			m.valueRevealed = false
			m.valueInput.EchoMode = textinput.EchoPassword
			m.blurAll()
			return m, nil
		}
		return m.closeModal()

	default:
		var cmd tea.Cmd
		switch m.focusField {
		case fieldName:
			m.nameInput, cmd = m.nameInput.Update(msg)
		case fieldEnv:
			m.envInput, cmd = m.envInput.Update(msg)
		case fieldValue:
			m.valueInput, cmd = m.valueInput.Update(msg)
		case fieldMetadata:
			m.metadataInput, cmd = m.metadataInput.Update(msg)
		}
		return m, cmd
	}
}

// ── Add mode key handling ─────────────────────────────────────

func (m SecretModal) handleAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block input during save or saved flash.
	if m.saving || m.savedMsg != "" {
		return m, nil
	}

	switch {
	case msg.Type == tea.KeyCtrlR:
		m.valueRevealed = !m.valueRevealed
		if m.valueRevealed {
			m.valueInput.EchoMode = textinput.EchoNormal
		} else {
			m.valueInput.EchoMode = textinput.EchoPassword
		}
		return m, nil

	case msg.Type == tea.KeyUp || msg.String() == "shift+tab":
		m.focusField = (m.focusField - 1 + fieldCount) % fieldCount
		m.focusCurrentField()
		return m, textinput.Blink

	case msg.Type == tea.KeyDown || msg.Type == tea.KeyTab:
		m.focusField = (m.focusField + 1) % fieldCount
		m.focusCurrentField()
		return m, textinput.Blink

	case msg.Type == tea.KeyEnter:
		return m.saveSecret()

	case msg.Type == tea.KeyEscape:
		return m.closeModal()

	default:
		var cmd tea.Cmd
		switch m.focusField {
		case fieldName:
			m.nameInput, cmd = m.nameInput.Update(msg)
		case fieldEnv:
			m.envInput, cmd = m.envInput.Update(msg)
		case fieldValue:
			m.valueInput, cmd = m.valueInput.Update(msg)
		case fieldMetadata:
			m.metadataInput, cmd = m.metadataInput.Update(msg)
		}
		return m, cmd
	}
}

// ── Actions ───────────────────────────────────────────────────

func (m SecretModal) saveSecret() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.toast = "Name cannot be empty"
		m.toastKind = "error"
		return m, nil
	}
	value := m.valueInput.Value()
	if value == "" {
		m.toast = "Value cannot be empty"
		m.toastKind = "error"
		return m, nil
	}

	env := strings.TrimSpace(m.envInput.Value())
	metadata := parseMetadataInput(m.metadataInput.Value())

	m.saving = true
	m.toast = ""

	v := m.vault

	isEdit := m.mode == modalEdit
	origName := m.origName
	origEnv := m.origEnv

	return m, func() tea.Msg {
		ctx := context.Background()

		if isEdit {
			// If the name or env changed, this is a rename: delete old, create new.
			if name != origName || env != origEnv {
				if err := v.Delete(ctx, origName, origEnv); err != nil {
					return secretModalResultMsg{toast: err.Error(), kind: "error"}
				}
				if err := v.Set(ctx, name, env, value, metadata); err != nil {
					return secretModalResultMsg{toast: err.Error(), kind: "error"}
				}
			} else {
				// In-place edit: update value and metadata.
				newValue := &value
				if err := v.Edit(ctx, name, env, newValue, metadata); err != nil {
					return secretModalResultMsg{toast: err.Error(), kind: "error"}
				}
			}
			return secretModalResultMsg{saved: true, toast: fmt.Sprintf("Secret %q updated.", name), kind: "success"}
		}

		// Add mode.
		if err := v.Set(ctx, name, env, value, metadata); err != nil {
			return secretModalResultMsg{toast: err.Error(), kind: "error"}
		}
		return secretModalResultMsg{saved: true, toast: fmt.Sprintf("Secret %q stored.", name), kind: "success"}
	}
}

func (m SecretModal) copyValue() tea.Cmd {
	value := m.entry.Value
	name := m.entry.Name
	return func() tea.Msg {
		if _, err := clipboard.CopyWithClear(value, 45*time.Second); err != nil {
			return secretModalResultMsg{toast: fmt.Sprintf("clipboard: %s", err), kind: "error"}
		}
		return toastMsg{text: fmt.Sprintf("Copied %q to clipboard (clears in 45s)", name), kind: "success"}
	}
}

func (m SecretModal) closeModal() (tea.Model, tea.Cmd) {
	m.blurAll()
	m.valueRevealed = false
	m.valueInput.EchoMode = textinput.EchoPassword
	if m.standalone {
		return m, tea.Quit
	}
	return m, func() tea.Msg { return secretModalCloseMsg{} }
}

func (m *SecretModal) blurAll() {
	m.nameInput.Blur()
	m.envInput.Blur()
	m.valueInput.Blur()
	m.metadataInput.Blur()
}

// ── View ──────────────────────────────────────────────────────

func (m SecretModal) View() string {
	switch m.mode {
	case modalView:
		return m.viewModeView()
	case modalEdit:
		return m.editModeView("Editing")
	case modalAdd:
		return m.editModeView("Add Secret")
	}
	return ""
}

func (m SecretModal) viewModeView() string {
	var b strings.Builder
	w := 56

	// Title.
	title := " " + m.entry.Name
	if m.entry.Environment != "" {
		title += " " + EnvBadgeStyle.Render("["+m.entry.Environment+"]")
	}
	b.WriteString(TitleStyle.Render(title))
	b.WriteString("\n\n")

	// Value.
	b.WriteString("  " + MutedStyle.Render("Value       "))
	if m.valueMasked {
		b.WriteString(MaskedValueStyle.Render(maskValue(m.entry.Value)))
	} else {
		b.WriteString(SecretValueStyle.Render(m.entry.Value))
	}
	b.WriteString("\n")

	// Timestamps.
	if m.entry.CreatedAt != "" {
		created := m.entry.CreatedAt
		if len(created) > 16 {
			created = created[:16]
		}
		b.WriteString("  " + MutedStyle.Render("Created     ") + created)
		b.WriteString("\n")
	}
	if m.entry.UpdatedAt != "" {
		updated := m.entry.UpdatedAt
		if len(updated) > 16 {
			updated = updated[:16]
		}
		b.WriteString("  " + MutedStyle.Render("Updated     ") + updated)
		b.WriteString("\n")
	}

	// Metadata.
	if len(m.entry.Metadata) > 0 {
		b.WriteString("  " + MutedStyle.Render("Metadata    ") + formatMetadataInline(m.entry.Metadata))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Help bar.
	var helpParts []string
	helpParts = append(helpParts,
		HelpKeyStyle.Render("p")+" "+HelpDescStyle.Render("peek"),
		HelpKeyStyle.Render("c")+" "+HelpDescStyle.Render("copy"),
		HelpKeyStyle.Render("e")+" "+HelpDescStyle.Render("edit"),
		HelpKeyStyle.Render("q")+" "+HelpDescStyle.Render("close"),
	)
	b.WriteString("  " + strings.Join(helpParts, MutedStyle.Render("  ·  ")))

	content := b.String()
	modal := ModalStyle.Width(w).Render(content)

	if m.standalone {
		return m.centerStandalone(modal)
	}
	return modal
}

func (m SecretModal) editModeView(titleText string) string {
	var b strings.Builder
	w := 56

	// Title.
	title := " " + titleText
	if m.mode == modalEdit && m.entry != nil {
		title = " Editing: " + m.origName
		if m.origEnv != "" {
			title += " " + EnvBadgeStyle.Render("["+m.origEnv+"]")
		}
	}
	b.WriteString(TitleStyle.Render(title))
	b.WriteString("\n\n")

	// Fields.
	fields := []struct {
		label string
		input textinput.Model
		idx   int
	}{
		{"Name        ", m.nameInput, fieldName},
		{"Environment ", m.envInput, fieldEnv},
		{"Value       ", m.valueInput, fieldValue},
		{"Metadata    ", m.metadataInput, fieldMetadata},
	}

	for _, f := range fields {
		cursor := "  "
		labelStyle := MutedStyle
		if m.focusField == f.idx {
			cursor = AccentStyle.Render("▸ ")
			labelStyle = AccentStyle
		}
		extra := ""
		if f.idx == fieldValue && m.focusField == fieldValue && m.valueRevealed {
			extra = MutedStyle.Render(" (visible)")
		}
		b.WriteString(cursor + labelStyle.Render(f.label) + extra + " " + f.input.View())
		b.WriteString("\n")
	}

	// Toast (inline error).
	if m.toast != "" {
		b.WriteString("\n")
		switch m.toastKind {
		case "error":
			b.WriteString("  " + ToastErrorStyle.Render(" "+m.toast+" "))
		case "success":
			b.WriteString("  " + ToastSuccessStyle.Render(" "+m.toast+" "))
		default:
			b.WriteString("  " + ToastInfoStyle.Render(" "+m.toast+" "))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Help bar / save feedback.
	if m.saving {
		b.WriteString("  " + ToastInfoStyle.Render(" Saving... "))
	} else if m.savedMsg != "" {
		b.WriteString("  " + ToastSuccessStyle.Render(" ✓ "+m.savedMsg+" "))
	} else {
		var helpParts []string
		helpParts = append(helpParts,
			HelpKeyStyle.Render("↑/↓")+" "+HelpDescStyle.Render("navigate"),
			HelpKeyStyle.Render("ctrl+r")+" "+HelpDescStyle.Render("peek"),
			HelpKeyStyle.Render("enter")+" "+HelpDescStyle.Render("save"),
			HelpKeyStyle.Render("esc")+" "+HelpDescStyle.Render("cancel"),
		)
		b.WriteString("  " + strings.Join(helpParts, MutedStyle.Render("  ·  ")))
	}

	content := b.String()
	modal := ModalStyle.Width(w).Render(content)

	if m.standalone {
		return m.centerStandalone(modal)
	}
	return modal
}

func (m SecretModal) centerStandalone(content string) string {
	contentHeight := strings.Count(content, "\n") + 1
	h := m.height
	if h <= 0 {
		h = 24
	}
	topPad := 0
	if h > contentHeight+2 {
		topPad = (h - contentHeight) / 3
	}
	return strings.Repeat("\n", topPad) + content
}
