package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Large value / textarea state.
	valueArea   textarea.Model
	useTextarea bool // true when value field uses textarea instead of textinput

	// Full view overlay (view mode).
	showFullView    bool
	fullViewContent string
	fullViewTitle   string
	fullViewScroll  int
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
	vi.CharLimit = 0 // unlimited — supports PEM certs, long tokens, etc.
	vi.EchoMode = textinput.EchoPassword

	mi := textinput.New()
	mi.Placeholder = "key=value, key=value (optional)"
	mi.CharLimit = 4096

	m.nameInput = ni
	m.envInput = ei
	m.valueInput = vi
	m.metadataInput = mi
}

// initValueTextarea switches the value field to a multi-line textarea,
// carrying over the given content. Called when a large value is detected.
func (m *SecretModal) initValueTextarea(value string) {
	ta := textarea.New()
	ta.SetValue(value)
	w := m.modalWidth() - 16 // space for label + cursor prefix
	if w < 30 {
		w = 30
	}
	ta.SetWidth(w)
	ta.SetHeight(m.textareaHeight())
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	m.valueArea = ta
	m.useTextarea = true
}

func (m *SecretModal) focusCurrentField() {
	m.nameInput.Blur()
	m.envInput.Blur()
	m.valueInput.Blur()
	m.metadataInput.Blur()
	if m.useTextarea {
		m.valueArea.Blur()
	}
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
		if m.useTextarea {
			m.valueArea.Focus()
		} else {
			m.valueInput.Focus()
		}
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
		// Resize textarea to fit new terminal dimensions.
		if m.useTextarea {
			w := m.modalWidth() - 16
			if w < 30 {
				w = 30
			}
			m.valueArea.SetWidth(w)
			m.valueArea.SetHeight(m.textareaHeight())
		}
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
		if m.useTextarea {
			m.valueArea, cmd = m.valueArea.Update(msg)
		} else {
			m.valueInput, cmd = m.valueInput.Update(msg)
		}
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
	// Full view overlay intercepts all keys.
	if m.showFullView {
		return m.handleFullViewKey(msg)
	}

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
	case "f":
		// Open full view for the focused item if it's revealed and large.
		val, title := m.focusedViewValue()
		if val != "" && isLargeValue(val) {
			m.showFullView = true
			m.fullViewContent = val
			m.fullViewTitle = title
			m.fullViewScroll = 0
		}
		return m, nil
	case "e":
		if m.entry != nil {
			m.mode = modalEdit
			m.origName = m.entry.Name
			m.origEnv = m.entry.Environment
			m.nameInput.SetValue(m.entry.Name)
			m.envInput.SetValue(m.entry.Environment)
			m.metadataInput.SetValue(formatMetadataPlain(m.entry.Metadata))
			// Use textarea for large values, textinput for short ones.
			if isLargeValue(m.entry.Value) {
				m.initValueTextarea(m.entry.Value)
			} else {
				m.valueInput.SetValue(m.entry.Value)
			}
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

	// When textarea is focused, only intercept field-management keys.
	// Everything else (up, down, enter, chars) goes to the textarea.
	if m.useTextarea && m.focusField == fieldValue {
		switch {
		case msg.Type == tea.KeyCtrlR:
			// No EchoPassword for textarea — no-op.
			return m, nil
		case msg.Type == tea.KeyCtrlA:
			m.fieldPairs = append(m.fieldPairs, newFieldPair("", ""))
			m.focusField = fixedFieldCount + (len(m.fieldPairs)-1)*2
			m.focusCurrentField()
			return m, textinput.Blink
		case msg.Type == tea.KeyCtrlD:
			// Focus is on value, not a field pair — no-op.
			return m, nil
		case msg.Type == tea.KeyTab:
			m.focusField = (m.focusField + 1) % total
			m.focusCurrentField()
			return m, textinput.Blink
		case msg.String() == "shift+tab":
			m.focusField = (m.focusField - 1 + total) % total
			m.focusCurrentField()
			return m, textinput.Blink
		default:
			return m.forwardKeyToInput(msg)
		}
	}

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
		if m.useTextarea {
			m.valueArea, cmd = m.valueArea.Update(msg)
		} else {
			m.valueInput, cmd = m.valueInput.Update(msg)
			// In add mode, swap to textarea if value exceeds threshold.
			if m.mode == modalAdd && !m.useTextarea && isLargeValue(m.valueInput.Value()) {
				m.initValueTextarea(m.valueInput.Value())
				m.valueArea.Focus()
			}
		}
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
	if m.useTextarea {
		value = m.valueArea.Value()
	}

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
	m.useTextarea = false
	m.showFullView = false
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
	if m.useTextarea {
		m.valueArea.Blur()
	}
	for i := range m.fieldPairs {
		m.fieldPairs[i].keyInput.Blur()
		m.fieldPairs[i].valueInput.Blur()
	}
}

// ── View ──────────────────────────────────────────────────────

func (m SecretModal) View() string {
	if m.showFullView {
		return m.fullViewView()
	}
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
	w := m.modalWidth()

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
		masked, _ := m.viewMasks[itemIdx]
		if masked {
			b.WriteString(cursor + MutedStyle.Render("Value       "))
			b.WriteString(MaskedValueStyle.Render(maskValue(m.entry.Value)))
			b.WriteString("\n")
		} else if isLargeValue(m.entry.Value) {
			b.WriteString(cursor + MutedStyle.Render("Value"))
			b.WriteString("\n")
			visible, total := wrapAndTruncate(m.entry.Value, w-4, 6)
			for _, line := range strings.Split(visible, "\n") {
				b.WriteString("    " + SecretValueStyle.Render(line) + "\n")
			}
			if total > 6 {
				b.WriteString("    " + MutedStyle.Render(fmt.Sprintf("(%d more lines — f full view · c copy)", total-6)) + "\n")
			}
		} else {
			b.WriteString(cursor + MutedStyle.Render("Value       "))
			b.WriteString(SecretValueStyle.Render(m.entry.Value))
			b.WriteString("\n")
		}
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
			masked, _ := m.viewMasks[itemIdx]
			fieldVal := m.entry.Fields[key]
			if masked {
				b.WriteString(cursor + FieldKeyStyle.Render(label) + " ")
				b.WriteString(MaskedValueStyle.Render(maskValue(fieldVal)))
				b.WriteString("\n")
			} else if isLargeValue(fieldVal) {
				b.WriteString(cursor + FieldKeyStyle.Render(key))
				b.WriteString("\n")
				visible, total := wrapAndTruncate(fieldVal, w-4, 6)
				for _, line := range strings.Split(visible, "\n") {
					b.WriteString("    " + SecretValueStyle.Render(line) + "\n")
				}
				if total > 6 {
					b.WriteString("    " + MutedStyle.Render(fmt.Sprintf("(%d more lines — f full view · c copy)", total-6)) + "\n")
				}
			} else {
				b.WriteString(cursor + FieldKeyStyle.Render(label) + " ")
				b.WriteString(SecretValueStyle.Render(fieldVal))
				b.WriteString("\n")
			}
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
	)
	// Show full view hint if the focused item has a large revealed value.
	if val, _ := m.focusedViewValue(); val != "" && isLargeValue(val) {
		helpParts = append(helpParts,
			HelpKeyStyle.Render("f")+" "+HelpDescStyle.Render("full view"),
		)
	}
	helpParts = append(helpParts,
		HelpKeyStyle.Render("e")+" "+HelpDescStyle.Render("edit"),
		HelpKeyStyle.Render("q")+" "+HelpDescStyle.Render("close"),
	)
	b.WriteString("  " + strings.Join(helpParts, MutedStyle.Render("  ·  ")))

	content := b.String()

	// Constrain to terminal: use viewportRender with auto-scroll to focused item.
	maxVisible := m.modalHeight()
	content, _ = viewportRender(content, maxVisible, 0, m.estimateViewFocusLine())

	modal := ModalStyle.Width(w).MaxHeight(m.height - 2).Render(content)

	if m.standalone {
		return m.centerStandalone(modal)
	}
	return modal
}

func (m SecretModal) editModeView(titleText string) string {
	var b strings.Builder
	w := m.modalWidth()

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
		// Textarea rendering for value field.
		if f.idx == fieldValue && m.useTextarea {
			extra := MutedStyle.Render(" (multi-line)")
			b.WriteString(cursor + labelStyle.Render(f.label) + extra)
			b.WriteString("\n")
			// Indent the textarea view. Trim trailing newline to avoid
			// an off-by-one in line count.
			taView := strings.TrimRight(m.valueArea.View(), "\n")
			for _, line := range strings.Split(taView, "\n") {
				b.WriteString("    " + line + "\n")
			}
			continue
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
		b.WriteString("  " + MutedStyle.Render("── Fields "+strings.Repeat("─", w-14)))
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
			HelpKeyStyle.Render("ctrl+A")+" "+HelpDescStyle.Render("add field"),
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

	// The textarea has built-in scrolling, so if textareaHeight() is
	// correct the form already fits. Apply a lipgloss MaxHeight safety cap
	// to guarantee we never overflow the terminal even if the line math
	// is slightly off.
	modal := ModalStyle.Width(w).MaxHeight(m.height - 2).Render(content)

	if m.standalone {
		return m.centerStandalone(modal)
	}
	return modal
}

// ── Full view overlay ─────────────────────────────────────────

// focusedViewValue returns the decrypted value and label for the currently
// focused item in view mode.  Returns ("","") when the item is masked or
// there is no entry.
func (m SecretModal) focusedViewValue() (string, string) {
	if m.entry == nil {
		return "", ""
	}
	// Check if the item is revealed (not masked).
	masked, _ := m.viewMasks[m.viewCursor]
	if masked {
		return "", ""
	}
	hasValue := m.entry.Value != ""
	if hasValue && m.viewCursor == 0 {
		return m.entry.Value, m.entry.Name
	}
	fieldIdx := m.viewCursor
	if hasValue {
		fieldIdx--
	}
	keys := sortedFieldKeys(m.entry.Fields)
	if fieldIdx >= 0 && fieldIdx < len(keys) {
		key := keys[fieldIdx]
		return m.entry.Fields[key], fmt.Sprintf("%s.%s", m.entry.Name, key)
	}
	return "", ""
}

// handleFullViewKey handles keys when the full view overlay is active.
func (m SecretModal) handleFullViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "f":
		m.showFullView = false
		return m, nil
	case "j", "down":
		m.fullViewScroll++
		return m, nil
	case "k", "up":
		if m.fullViewScroll > 0 {
			m.fullViewScroll--
		}
		return m, nil
	case "c":
		// Copy the full value.
		val := m.fullViewContent
		title := m.fullViewTitle
		return m, func() tea.Msg {
			if _, err := clipboard.CopyWithClear(val, 45*time.Second); err != nil {
				return secretModalResultMsg{toast: fmt.Sprintf("clipboard: %s", err), kind: "error"}
			}
			return toastMsg{text: fmt.Sprintf("Copied %q to clipboard (clears in 45s)", title), kind: "success"}
		}
	}
	return m, nil
}

// fullViewView renders the scrollable full view overlay.
func (m SecretModal) fullViewView() string {
	tw := m.width
	if tw <= 0 {
		tw = 80
	}
	th := m.height
	if th <= 0 {
		th = 24
	}

	// Overlay dimensions: ~90% width, ~80% height.
	ow := tw * 90 / 100
	if ow < 60 {
		ow = 60
	}
	if ow > tw {
		ow = tw
	}
	oh := th * 80 / 100
	if oh < 12 {
		oh = 12
	}
	if oh > th {
		oh = th
	}

	contentWidth := ow - 6 // borders + padding
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Wrap all content (maxLines=0 means no truncation).
	wrapped, _ := wrapAndTruncate(m.fullViewContent, contentWidth, 0)
	allLines := strings.Split(wrapped, "\n")

	// Visible area: subtract title (2 lines), help bar (2 lines), borders (2 lines).
	visibleCount := oh - 6
	if visibleCount < 1 {
		visibleCount = 1
	}

	// Clamp scroll.
	maxScroll := len(allLines) - visibleCount
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.fullViewScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := scroll + visibleCount
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := strings.Join(allLines[scroll:end], "\n")

	var b strings.Builder
	b.WriteString(TitleStyle.Render(" " + m.fullViewTitle))
	b.WriteString("\n\n")
	b.WriteString(SecretValueStyle.Render(visible))
	b.WriteString("\n\n")

	// Scroll indicator + help.
	scrollInfo := fmt.Sprintf("line %d/%d", scroll+1, len(allLines))
	helpParts := []string{
		HelpKeyStyle.Render("↑/↓") + " " + HelpDescStyle.Render("scroll"),
		HelpKeyStyle.Render("c") + " " + HelpDescStyle.Render("copy"),
		HelpKeyStyle.Render("esc") + " " + HelpDescStyle.Render("close"),
		MutedStyle.Render(scrollInfo),
	}
	b.WriteString("  " + strings.Join(helpParts, MutedStyle.Render("  ·  ")))

	content := b.String()
	modal := ModalStyle.Width(contentWidth).Render(content)

	if m.standalone {
		return m.centerStandalone(modal)
	}
	return modal
}

func (m SecretModal) centerStandalone(content string) string {
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

// ── Helpers ───────────────────────────────────────────────────

// modalWidth returns the content width for the modal, scaled to the terminal
// width. The modal border + padding consume ~6 columns, so we subtract that
// and cap between a floor of 56 and a ceiling of 120.
func (m SecretModal) modalWidth() int {
	w := m.width - 6 // border (2) + padding (2*2)
	if w < 56 {
		w = 56
	}
	if w > 120 {
		w = 120
	}
	return w
}

// modalHeight returns the maximum content height for the modal, scaled to the
// terminal height. The modal border + padding consume ~4 rows, so we subtract
// that and leave a small margin.
func (m SecretModal) modalHeight() int {
	h := m.height - 4 // border (2) + padding (1*2)
	if h < 12 {
		h = 12
	}
	return h
}

// textareaHeight returns the ideal textarea height given how many other
// fixed lines the edit/add form needs. This ensures the overall modal
// fits within the terminal.
func (m SecretModal) textareaHeight() int {
	// Fixed overhead in edit mode:
	//   title(1) + blank(1) = 2
	//   name(1) + env(1) + value label(1) + metadata(1) = 4
	//   blank before help(1) + help bar(1) = 2
	// Total fixed = 8
	overhead := 8

	// Dynamic field pairs: blank + header line + 2 lines per pair.
	if len(m.fieldPairs) > 0 {
		overhead += 2 + len(m.fieldPairs)*2
	}

	// Toast adds blank + toast line = 2.
	if m.toast != "" {
		overhead += 2
	}

	avail := m.modalHeight() - overhead
	if avail < 3 {
		avail = 3
	}
	if avail > 20 {
		avail = 20
	}
	return avail
}

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

// estimateViewFocusLine estimates the line number (0-indexed) of the
// currently focused item in view mode, so viewportRender can auto-scroll
// to keep it visible.
func (m SecretModal) estimateViewFocusLine() int {
	if m.entry == nil {
		return 0
	}
	line := 2 // title + blank line
	hasValue := m.entry.Value != ""
	itemIdx := 0

	if hasValue {
		if m.viewCursor == itemIdx {
			return line
		}
		// Account for multi-line value display.
		masked, _ := m.viewMasks[itemIdx]
		if !masked && isLargeValue(m.entry.Value) {
			_, total := wrapAndTruncate(m.entry.Value, m.modalWidth()-4, 6)
			lines := minVal(total, 6)
			line += 1 + lines // label line + content lines
			if total > 6 {
				line++ // "N more lines" hint
			}
		} else {
			line++ // single line value
		}
		itemIdx++
	}

	// Fields.
	if len(m.entry.Fields) > 0 {
		if hasValue {
			line++ // blank separator
		}
		keys := sortedFieldKeys(m.entry.Fields)
		for _, key := range keys {
			if m.viewCursor == itemIdx {
				return line
			}
			masked, _ := m.viewMasks[itemIdx]
			fieldVal := m.entry.Fields[key]
			if !masked && isLargeValue(fieldVal) {
				_, total := wrapAndTruncate(fieldVal, m.modalWidth()-4, 6)
				lines := minVal(total, 6)
				line += 1 + lines
				if total > 6 {
					line++
				}
			} else {
				line++
			}
			itemIdx++
		}
	}

	return line
}
