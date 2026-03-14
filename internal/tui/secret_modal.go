package tui

import (
	"context"
	"fmt"
	"sort"
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

// ── Fixed field indices for edit/add form ─────────────────────
//
// The form has 4 fixed inputs (name, env, value, metadata) followed by
// zero or more dynamic field pairs.  focusField 0..3 = fixed fields,
// focusField >= fixedFieldCount targets a field pair.

const (
	fieldName     = 0
	fieldEnv      = 1
	fieldValue    = 2
	fieldMetadata = 3
	// Number of fixed inputs before dynamic field pairs.
	fixedFieldCount = 4
)

// fieldPair is one key=value row in the dynamic fields section.
type fieldPair struct {
	keyInput   textinput.Model
	valueInput textinput.Model
}

func newFieldPair(key, value string) fieldPair {
	ki := textinput.New()
	ki.Placeholder = "field key"
	ki.CharLimit = 256
	ki.SetValue(key)

	vi := textinput.New()
	vi.Placeholder = "field value"
	vi.CharLimit = 4096
	vi.EchoMode = textinput.EchoPassword
	vi.SetValue(value)

	return fieldPair{keyInput: ki, valueInput: vi}
}

// totalInputCount returns the total number of focusable inputs
// (4 fixed + 2 per field pair).
func totalInputCount(pairs int) int {
	return fixedFieldCount + pairs*2
}

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
	focusField    int  // which input has focus (0..fixedFieldCount-1 for fixed, then pairs)
	valueRevealed bool // true when Ctrl+R has been pressed to peek

	// Dynamic field pairs (edit + add modes).
	fieldPairs []fieldPair

	// View mode state.
	valueMasked bool         // value masked in view mode
	viewCursor  int          // 0 = value, 1..N = fields (sorted by key)
	viewMasks   map[int]bool // per-item mask state in view mode (true = masked)

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
	masks := make(map[int]bool)
	// Mask the main value and all fields by default.
	masks[0] = true
	if entry != nil {
		for i := range sortedFieldKeys(entry.Fields) {
			masks[i+1] = true
		}
	}
	m := SecretModal{
		vault:       v,
		mode:        modalView,
		entry:       entry,
		valueMasked: true,
		viewMasks:   masks,
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
	// Populate field pairs from entry.
	if len(entry.Fields) > 0 {
		keys := sortedFieldKeys(entry.Fields)
		for _, k := range keys {
			m.fieldPairs = append(m.fieldPairs, newFieldPair(k, entry.Fields[k]))
		}
	}
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
	vi.Placeholder = "secret value (optional if fields set)"
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
	for i := range m.fieldPairs {
		m.fieldPairs[i].keyInput.Blur()
		m.fieldPairs[i].valueInput.Blur()
	}

	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldEnv:
		m.envInput.Focus()
	case fieldValue:
		m.valueInput.Focus()
	case fieldMetadata:
		m.metadataInput.Focus()
	default:
		// Dynamic field pair focus.
		idx := m.focusField - fixedFieldCount
		pairIdx := idx / 2
		isValue := idx%2 == 1
		if pairIdx >= 0 && pairIdx < len(m.fieldPairs) {
			if isValue {
				m.fieldPairs[pairIdx].valueInput.Focus()
			} else {
				m.fieldPairs[pairIdx].keyInput.Focus()
			}
		}
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
	switch {
	case m.focusField == fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case m.focusField == fieldEnv:
		m.envInput, cmd = m.envInput.Update(msg)
	case m.focusField == fieldValue:
		m.valueInput, cmd = m.valueInput.Update(msg)
	case m.focusField == fieldMetadata:
		m.metadataInput, cmd = m.metadataInput.Update(msg)
	case m.focusField >= fixedFieldCount:
		idx := m.focusField - fixedFieldCount
		pairIdx := idx / 2
		isValue := idx%2 == 1
		if pairIdx >= 0 && pairIdx < len(m.fieldPairs) {
			if isValue {
				m.fieldPairs[pairIdx].valueInput, cmd = m.fieldPairs[pairIdx].valueInput.Update(msg)
			} else {
				m.fieldPairs[pairIdx].keyInput, cmd = m.fieldPairs[pairIdx].keyInput.Update(msg)
			}
		}
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

	// Count navigable items: value (if non-empty) + fields.
	itemCount := m.viewItemCount()

	switch key {
	case "j", "down":
		if m.viewCursor < itemCount-1 {
			m.viewCursor++
		}
	case "k", "up":
		if m.viewCursor > 0 {
			m.viewCursor--
		}
	case "p", " ":
		// Toggle peek for the focused item.
		if m.viewMasks == nil {
			m.viewMasks = make(map[int]bool)
		}
		m.viewMasks[m.viewCursor] = !m.viewMasks[m.viewCursor]
	case "c":
		// Copy focused item value to clipboard.
		return m, m.copyFocusedValue()
	case "e":
		if m.entry != nil {
			m.mode = modalEdit
			m.origName = m.entry.Name
			m.origEnv = m.entry.Environment
			m.nameInput.SetValue(m.entry.Name)
			m.envInput.SetValue(m.entry.Environment)
			m.valueInput.SetValue(m.entry.Value)
			m.metadataInput.SetValue(formatMetadataPlain(m.entry.Metadata))
			// Populate field pairs.
			m.fieldPairs = nil
			if len(m.entry.Fields) > 0 {
				keys := sortedFieldKeys(m.entry.Fields)
				for _, k := range keys {
					m.fieldPairs = append(m.fieldPairs, newFieldPair(k, m.entry.Fields[k]))
				}
			}
			m.focusField = fieldName
			m.focusCurrentField()
			return m, textinput.Blink
		}
	case "q", "esc":
		return m.closeModal()
	}
	return m, nil
}

// viewItemCount returns the number of navigable items in view mode.
func (m SecretModal) viewItemCount() int {
	count := 0
	if m.entry != nil {
		if m.entry.Value != "" {
			count++ // main value
		}
		count += len(m.entry.Fields)
	}
	if count == 0 {
		count = 1 // always at least 1 (even if empty)
	}
	return count
}

// copyFocusedValue returns a tea.Cmd that copies the currently focused view item.
func (m SecretModal) copyFocusedValue() tea.Cmd {
	if m.entry == nil {
		return nil
	}

	var value, label string
	hasValue := m.entry.Value != ""

	if hasValue && m.viewCursor == 0 {
		value = m.entry.Value
		label = m.entry.Name
	} else {
		// Field value — cursor offset by 1 if main value exists.
		fieldIdx := m.viewCursor
		if hasValue {
			fieldIdx--
		}
		keys := sortedFieldKeys(m.entry.Fields)
		if fieldIdx >= 0 && fieldIdx < len(keys) {
			key := keys[fieldIdx]
			value = m.entry.Fields[key]
			label = fmt.Sprintf("%s.%s", m.entry.Name, key)
		}
	}

	if value == "" {
		return nil
	}

	return func() tea.Msg {
		if _, err := clipboard.CopyWithClear(value, 45*time.Second); err != nil {
			return secretModalResultMsg{toast: fmt.Sprintf("clipboard: %s", err), kind: "error"}
		}
		return toastMsg{text: fmt.Sprintf("Copied %q to clipboard (clears in 45s)", label), kind: "success"}
	}
}

// ── Edit/Add shared key handling ──────────────────────────────

func (m SecretModal) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block input during save or saved flash.
	if m.saving || m.savedMsg != "" {
		return m, nil
	}

	total := totalInputCount(len(m.fieldPairs))

	switch {
	case msg.Type == tea.KeyCtrlR:
		// Toggle value visibility for the main value input or focused field value.
		if m.focusField == fieldValue {
			m.valueRevealed = !m.valueRevealed
			if m.valueRevealed {
				m.valueInput.EchoMode = textinput.EchoNormal
			} else {
				m.valueInput.EchoMode = textinput.EchoPassword
			}
		} else if m.focusField >= fixedFieldCount {
			idx := m.focusField - fixedFieldCount
			pairIdx := idx / 2
			isValue := idx%2 == 1
			if isValue && pairIdx >= 0 && pairIdx < len(m.fieldPairs) {
				fp := &m.fieldPairs[pairIdx]
				if fp.valueInput.EchoMode == textinput.EchoPassword {
					fp.valueInput.EchoMode = textinput.EchoNormal
				} else {
					fp.valueInput.EchoMode = textinput.EchoPassword
				}
			}
		}
		return m, nil

	case msg.Type == tea.KeyCtrlA:
		// Add new field pair.
		m.fieldPairs = append(m.fieldPairs, newFieldPair("", ""))
		// Focus the new key input.
		m.focusField = fixedFieldCount + (len(m.fieldPairs)-1)*2
		m.focusCurrentField()
		return m, textinput.Blink

	case msg.Type == tea.KeyCtrlD:
		// Delete focused field pair (only if focus is on a field pair).
		if m.focusField >= fixedFieldCount && len(m.fieldPairs) > 0 {
			idx := m.focusField - fixedFieldCount
			pairIdx := idx / 2
			if pairIdx >= 0 && pairIdx < len(m.fieldPairs) {
				m.fieldPairs = append(m.fieldPairs[:pairIdx], m.fieldPairs[pairIdx+1:]...)
				// Adjust focus.
				newTotal := totalInputCount(len(m.fieldPairs))
				if m.focusField >= newTotal {
					m.focusField = newTotal - 1
					if m.focusField < 0 {
						m.focusField = fieldMetadata
					}
				}
				m.focusCurrentField()
				return m, textinput.Blink
			}
		}
		return m, nil

	case msg.Type == tea.KeyUp || msg.String() == "shift+tab":
		m.focusField = (m.focusField - 1 + total) % total
		m.focusCurrentField()
		return m, textinput.Blink

	case msg.Type == tea.KeyDown || msg.Type == tea.KeyTab:
		m.focusField = (m.focusField + 1) % total
		m.focusCurrentField()
		return m, textinput.Blink

	case msg.Type == tea.KeyEnter:
		return m.saveSecret()

	default:
		return m.forwardKeyToInput(msg)
	}
}

func (m SecretModal) forwardKeyToInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case m.focusField == fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case m.focusField == fieldEnv:
		m.envInput, cmd = m.envInput.Update(msg)
	case m.focusField == fieldValue:
		m.valueInput, cmd = m.valueInput.Update(msg)
	case m.focusField == fieldMetadata:
		m.metadataInput, cmd = m.metadataInput.Update(msg)
	case m.focusField >= fixedFieldCount:
		idx := m.focusField - fixedFieldCount
		pairIdx := idx / 2
		isValue := idx%2 == 1
		if pairIdx >= 0 && pairIdx < len(m.fieldPairs) {
			if isValue {
				m.fieldPairs[pairIdx].valueInput, cmd = m.fieldPairs[pairIdx].valueInput.Update(msg)
			} else {
				m.fieldPairs[pairIdx].keyInput, cmd = m.fieldPairs[pairIdx].keyInput.Update(msg)
			}
		}
	}
	return m, cmd
}

// ── Edit mode key handling ────────────────────────────────────

func (m SecretModal) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block input during save or saved flash.
	if m.saving || m.savedMsg != "" {
		return m, nil
	}

	if msg.Type == tea.KeyEscape {
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
	}

	return m.handleFormKey(msg)
}

// ── Add mode key handling ─────────────────────────────────────

func (m SecretModal) handleAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block input during save or saved flash.
	if m.saving || m.savedMsg != "" {
		return m, nil
	}

	if msg.Type == tea.KeyEscape {
		return m.closeModal()
	}

	return m.handleFormKey(msg)
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

	// Collect field pairs.
	fields := make(map[string]string)
	for _, fp := range m.fieldPairs {
		k := strings.TrimSpace(fp.keyInput.Value())
		v := fp.valueInput.Value()
		if k != "" {
			fields[k] = v
		}
	}

	// Must have at least a value or fields.
	if value == "" && len(fields) == 0 {
		m.toast = "Value or at least one field required"
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
				if value != "" {
					if err := v.Set(ctx, name, env, value, metadata); err != nil {
						return secretModalResultMsg{toast: err.Error(), kind: "error"}
					}
				} else {
					// Fields-only entry.
					if err := v.SetField(ctx, name, env, "_placeholder", ""); err != nil {
						return secretModalResultMsg{toast: err.Error(), kind: "error"}
					}
					// Delete the placeholder — SetField auto-creates the parent.
					_ = v.DeleteField(ctx, name, env, "_placeholder")
				}
			} else {
				// In-place edit: update value and metadata.
				if value != "" {
					newValue := &value
					if err := v.Edit(ctx, name, env, newValue, metadata); err != nil {
						return secretModalResultMsg{toast: err.Error(), kind: "error"}
					}
				}
			}

			// Sync fields: get current fields, delete removed, set new/changed.
			if err := syncFields(ctx, v, name, env, fields); err != nil {
				return secretModalResultMsg{toast: err.Error(), kind: "error"}
			}

			return secretModalResultMsg{saved: true, toast: fmt.Sprintf("Secret %q updated.", name), kind: "success"}
		}

		// Add mode.
		if value != "" {
			if err := v.Set(ctx, name, env, value, metadata); err != nil {
				return secretModalResultMsg{toast: err.Error(), kind: "error"}
			}
		}

		// Save fields.
		if len(fields) > 0 {
			if err := v.SetFields(ctx, name, env, fields); err != nil {
				return secretModalResultMsg{toast: err.Error(), kind: "error"}
			}
		}

		return secretModalResultMsg{saved: true, toast: fmt.Sprintf("Secret %q stored.", name), kind: "success"}
	}
}

// syncFields reconciles the desired fields map with what's in the vault.
func syncFields(ctx context.Context, v vault.Vault, name, env string, desired map[string]string) error {
	// Get current fields.
	current, err := v.GetFields(ctx, name, env)
	if err != nil {
		// Not found is OK — entry may have no fields yet.
		current = nil
	}

	// Delete fields that were removed.
	for key := range current {
		if _, exists := desired[key]; !exists {
			if err := v.DeleteField(ctx, name, env, key); err != nil {
				return err
			}
		}
	}

	// Set new/changed fields.
	if len(desired) > 0 {
		if err := v.SetFields(ctx, name, env, desired); err != nil {
			return err
		}
	}

	return nil
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
	for i := range m.fieldPairs {
		m.fieldPairs[i].keyInput.Blur()
		m.fieldPairs[i].valueInput.Blur()
	}
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

	// Build navigable items: value (if present) + fields sorted by key.
	hasValue := m.entry.Value != ""
	itemIdx := 0

	// Main value.
	if hasValue {
		cursor := "  "
		if m.viewCursor == itemIdx {
			cursor = AccentStyle.Render("▸ ")
		}
		b.WriteString(cursor + MutedStyle.Render("Value       "))
		masked, _ := m.viewMasks[itemIdx]
		if masked {
			b.WriteString(MaskedValueStyle.Render(maskValue(m.entry.Value)))
		} else {
			b.WriteString(SecretValueStyle.Render(m.entry.Value))
		}
		b.WriteString("\n")
		itemIdx++
	}

	// Fields.
	if len(m.entry.Fields) > 0 {
		if hasValue {
			b.WriteString("\n")
		}
		keys := sortedFieldKeys(m.entry.Fields)
		for _, key := range keys {
			cursor := "  "
			if m.viewCursor == itemIdx {
				cursor = AccentStyle.Render("▸ ")
			}
			label := padRight(key, 12)
			b.WriteString(cursor + FieldKeyStyle.Render(label) + " ")
			masked, _ := m.viewMasks[itemIdx]
			if masked {
				b.WriteString(MaskedValueStyle.Render(maskValue(m.entry.Fields[key])))
			} else {
				b.WriteString(SecretValueStyle.Render(m.entry.Fields[key]))
			}
			b.WriteString("\n")
			itemIdx++
		}
	}

	// Timestamps.
	b.WriteString("\n")
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

	// Toast.
	if m.toast != "" {
		switch m.toastKind {
		case "success":
			b.WriteString("  " + ToastSuccessStyle.Render(" "+m.toast+" "))
		case "error":
			b.WriteString("  " + ToastErrorStyle.Render(" "+m.toast+" "))
		default:
			b.WriteString("  " + ToastInfoStyle.Render(" "+m.toast+" "))
		}
		b.WriteString("\n")
	}

	// Help bar.
	var helpParts []string
	helpParts = append(helpParts,
		HelpKeyStyle.Render("↑/↓")+" "+HelpDescStyle.Render("navigate"),
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

	// Fixed fields.
	fixedFields := []struct {
		label string
		input textinput.Model
		idx   int
	}{
		{"Name        ", m.nameInput, fieldName},
		{"Environment ", m.envInput, fieldEnv},
		{"Value       ", m.valueInput, fieldValue},
		{"Metadata    ", m.metadataInput, fieldMetadata},
	}

	for _, f := range fixedFields {
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

	// Dynamic field pairs.
	if len(m.fieldPairs) > 0 {
		b.WriteString("\n")
		b.WriteString("  " + MutedStyle.Render("── Fields ──────────────────"))
		b.WriteString("\n")
		for i, fp := range m.fieldPairs {
			keyFocusIdx := fixedFieldCount + i*2
			valFocusIdx := fixedFieldCount + i*2 + 1

			// Key input.
			cursor := "  "
			labelStyle := MutedStyle
			if m.focusField == keyFocusIdx {
				cursor = AccentStyle.Render("▸ ")
				labelStyle = AccentStyle
			}
			b.WriteString(cursor + labelStyle.Render(fmt.Sprintf("Key %-2d      ", i+1)) + " " + fp.keyInput.View())
			b.WriteString("\n")

			// Value input.
			cursor = "  "
			labelStyle = MutedStyle
			if m.focusField == valFocusIdx {
				cursor = AccentStyle.Render("▸ ")
				labelStyle = AccentStyle
			}
			extra := ""
			if m.focusField == valFocusIdx && fp.valueInput.EchoMode == textinput.EchoNormal {
				extra = MutedStyle.Render(" (visible)")
			}
			b.WriteString(cursor + labelStyle.Render(fmt.Sprintf("Val %-2d      ", i+1)) + extra + " " + fp.valueInput.View())
			b.WriteString("\n")
		}
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
			HelpKeyStyle.Render("ctrl+a")+" "+HelpDescStyle.Render("add field"),
		)
		if len(m.fieldPairs) > 0 {
			helpParts = append(helpParts,
				HelpKeyStyle.Render("ctrl+d")+" "+HelpDescStyle.Render("del field"),
			)
		}
		helpParts = append(helpParts,
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

// ── Helpers ───────────────────────────────────────────────────

// sortedFieldKeys returns the keys of a string map in sorted order.
func sortedFieldKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
