package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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
	valueArea      textarea.Model
	useTextarea    bool // true when value field is currently showing textarea
	textareaInited bool // true once initValueTextarea has been called (textarea retains edits)

	// Viewport for view-mode scrollable content and edit-mode form scrolling.
	// In view mode, all body content is rendered into this viewport so the
	// modal never exceeds terminal bounds and scrolling is handled natively
	// by the Bubble Tea viewport component.
	viewVP viewport.Model

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
	m.initViewport()
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
	m.initViewport()
	m.nameInput.SetValue(entry.Name)
	m.envInput.SetValue(entry.Environment)
	// Value always starts masked (EchoPassword on the textinput).
	// Textarea is initialised lazily on Ctrl+R reveal for large values.
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
	m.initViewport()
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

// initViewport creates the viewport for view-mode scrolling with all
// built-in keybindings disabled (we handle navigation ourselves).
func (m *SecretModal) initViewport() {
	vp := viewport.New(m.modalWidth(), m.viewportHeight())
	// Disable all built-in key bindings — view mode uses j/k for item
	// navigation and we auto-scroll the viewport to the focused item.
	// Edit mode doesn't use the viewport for key input at all.
	vp.KeyMap.Up.Unbind()
	vp.KeyMap.Down.Unbind()
	vp.KeyMap.PageUp.Unbind()
	vp.KeyMap.PageDown.Unbind()
	vp.KeyMap.HalfPageUp.Unbind()
	vp.KeyMap.HalfPageDown.Unbind()
	vp.KeyMap.Left.Unbind()
	vp.KeyMap.Right.Unbind()
	m.viewVP = vp
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
	m.textareaInited = true
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

// Resize updates the modal's dimensions and resizes internal components
// (viewport, textarea) to match. Call this after constructing a modal
// when the parent already knows the terminal size, since constructors
// run with width=0/height=0 and produce minimum-sized components.
func (m *SecretModal) Resize(width, height int) {
	m.width = width
	m.height = height
	// Resize viewport.
	m.viewVP.Width = m.modalWidth()
	m.viewVP.Height = m.viewportHeight()
	// Resize textarea if active.
	if m.useTextarea {
		w := m.modalWidth() - 16
		if w < 30 {
			w = 30
		}
		m.valueArea.SetWidth(w)
		m.recalcTextareaHeight()
	}
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
		// Resize viewport for view mode.
		m.viewVP.Width = m.modalWidth()
		m.viewVP.Height = m.viewportHeight()
		// Resize textarea to fit new terminal dimensions.
		if m.useTextarea {
			w := m.modalWidth() - 16
			if w < 30 {
				w = 30
			}
			m.valueArea.SetWidth(w)
			m.recalcTextareaHeight()
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
		m.recalcTextareaHeight() // toast changes overhead
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
			// Value always starts masked (EchoPassword on the textinput).
			// Textarea is initialised lazily on Ctrl+R reveal for large values.
			m.valueInput.SetValue(m.entry.Value)
			m.useTextarea = false
			m.textareaInited = false // Reset for fresh edit cycle.
			m.valueRevealed = false
			m.valueInput.EchoMode = textinput.EchoPassword
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
			// Hide: switch back to masked textinput display.
			// Do NOT sync textarea→textinput (textinput strips newlines).
			// The textarea retains its value for saving and future reveals.
			m.valueRevealed = false
			m.useTextarea = false
			m.valueArea.Blur()
			m.valueInput.EchoMode = textinput.EchoPassword
			m.valueInput.Focus()
			return m, nil
		case msg.Type == tea.KeyCtrlA:
			m.fieldPairs = append(m.fieldPairs, newFieldPair("", ""))
			m.recalcTextareaHeight() // field pair changes overhead
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
				// Revealing: determine the source value for display.
				// Use the textarea if already initialized (preserves edits),
				// otherwise use the entry's original value (textinput strips
				// newlines so it can't hold multi-line content faithfully).
				var val string
				if m.textareaInited {
					val = m.valueArea.Value()
				} else if m.entry != nil {
					val = m.entry.Value
				} else {
					val = m.valueInput.Value()
				}
				if isLargeValue(val) {
					if m.textareaInited {
						// Re-show existing textarea (preserves user edits).
						m.useTextarea = true
						m.valueArea.Focus()
					} else {
						m.initValueTextarea(val)
						m.valueArea.Focus()
					}
				} else {
					m.valueInput.EchoMode = textinput.EchoNormal
				}
			} else {
				// Hiding: switch back to masked textinput display.
				// Do NOT sync textarea→textinput (textinput strips newlines).
				// The textarea retains its value for saving and future reveals.
				if m.useTextarea {
					m.useTextarea = false
					m.valueArea.Blur()
					m.valueInput.Focus()
				}
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
		m.recalcTextareaHeight() // field pair changes overhead
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
				m.recalcTextareaHeight() // field pair changes overhead
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
		m.recalcTextareaHeight() // toast changes overhead
		return m, nil
	}
	// Determine the authoritative value. When the textarea has been
	// initialized (via Ctrl+R reveal) it holds the canonical content —
	// even when currently hidden (useTextarea==false) — because
	// textinput.SetValue() replaces newlines with spaces, corrupting
	// multi-line values like PEM certs.
	//
	// In edit mode, if the user never revealed (Ctrl+R) the value,
	// preserve the original entry value exactly to avoid silently
	// re-saving a newline-corrupted copy.
	value := m.valueInput.Value()
	if m.textareaInited {
		value = m.valueArea.Value()
	} else if m.mode == modalEdit && m.entry != nil && !m.valueRevealed {
		value = m.entry.Value
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
		m.recalcTextareaHeight() // toast changes overhead
		return m, nil
	}

	env := strings.TrimSpace(m.envInput.Value())
	metadata := parseMetadataInput(m.metadataInput.Value())

	m.saving = true
	m.toast = ""
	m.recalcTextareaHeight() // toast cleared changes overhead

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
	// Block rendering until dimensions are known. In overlay mode the
	// parent TUI sets width/height directly; in standalone mode
	// tea.WindowSizeMsg sets them. Either way, 0 means "not yet ready".
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var result string
	if m.showFullView {
		result = m.fullViewView()
	} else {
		switch m.mode {
		case modalView:
			result = m.viewModeView()
		case modalEdit:
			result = m.editModeView("Editing")
		case modalAdd:
			result = m.editModeView("Add Secret")
		default:
			return ""
		}
	}

	// Safety net: truncate output to terminal height. This mirrors what the
	// Bubble Tea renderer does internally, but we do it proactively so the
	// renderer doesn't silently drop our header/top-border.
	lines := strings.Split(result, "\n")
	if m.height > 0 && len(lines) > m.height {
		result = strings.Join(lines[:m.height], "\n")
	}

	return result
}

func (m *SecretModal) viewModeView() string {
	w := m.modalWidth()

	// ── Build scrollable body content ──
	var body strings.Builder

	// Title.
	title := " " + m.entry.Name
	if m.entry.Environment != "" {
		title += " " + EnvBadgeStyle.Render("["+m.entry.Environment+"]")
	}
	body.WriteString(TitleStyle.Render(title))
	body.WriteString("\n\n")

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
			body.WriteString(cursor + MutedStyle.Render("Value       "))
			body.WriteString(MaskedValueStyle.Render(maskValue(m.entry.Value)))
			body.WriteString("\n")
		} else if isLargeValue(m.entry.Value) {
			body.WriteString(cursor + MutedStyle.Render("Value"))
			body.WriteString("\n")
			visible, total := wrapAndTruncate(m.entry.Value, w-4, 6)
			for _, line := range strings.Split(visible, "\n") {
				body.WriteString("    " + SecretValueStyle.Render(line) + "\n")
			}
			if total > 6 {
				body.WriteString("    " + MutedStyle.Render(fmt.Sprintf("(%d more lines — f full view · c copy)", total-6)) + "\n")
			}
		} else {
			body.WriteString(cursor + MutedStyle.Render("Value       "))
			body.WriteString(SecretValueStyle.Render(m.entry.Value))
			body.WriteString("\n")
		}
		itemIdx++
	}

	// Fields.
	if len(m.entry.Fields) > 0 {
		if hasValue {
			body.WriteString("\n")
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
				body.WriteString(cursor + FieldKeyStyle.Render(label) + " ")
				body.WriteString(MaskedValueStyle.Render(maskValue(fieldVal)))
				body.WriteString("\n")
			} else if isLargeValue(fieldVal) {
				body.WriteString(cursor + FieldKeyStyle.Render(key))
				body.WriteString("\n")
				visible, total := wrapAndTruncate(fieldVal, w-4, 6)
				for _, line := range strings.Split(visible, "\n") {
					body.WriteString("    " + SecretValueStyle.Render(line) + "\n")
				}
				if total > 6 {
					body.WriteString("    " + MutedStyle.Render(fmt.Sprintf("(%d more lines — f full view · c copy)", total-6)) + "\n")
				}
			} else {
				body.WriteString(cursor + FieldKeyStyle.Render(label) + " ")
				body.WriteString(SecretValueStyle.Render(fieldVal))
				body.WriteString("\n")
			}
			itemIdx++
		}
	}

	// Timestamps.
	body.WriteString("\n")
	if m.entry.CreatedAt != "" {
		created := m.entry.CreatedAt
		if len(created) > 16 {
			created = created[:16]
		}
		body.WriteString("  " + MutedStyle.Render("Created     ") + created)
		body.WriteString("\n")
	}
	if m.entry.UpdatedAt != "" {
		updated := m.entry.UpdatedAt
		if len(updated) > 16 {
			updated = updated[:16]
		}
		body.WriteString("  " + MutedStyle.Render("Updated     ") + updated)
		body.WriteString("\n")
	}

	// Metadata.
	if len(m.entry.Metadata) > 0 {
		body.WriteString("  " + MutedStyle.Render("Metadata    ") + formatMetadataInline(m.entry.Metadata))
		body.WriteString("\n")
	}

	// Toast.
	if m.toast != "" {
		body.WriteString("\n")
		switch m.toastKind {
		case "success":
			body.WriteString("  " + ToastSuccessStyle.Render(" "+m.toast+" "))
		case "error":
			body.WriteString("  " + ToastErrorStyle.Render(" "+m.toast+" "))
		default:
			body.WriteString("  " + ToastInfoStyle.Render(" "+m.toast+" "))
		}
		body.WriteString("\n")
	}

	// ── Set viewport content and auto-scroll to focused item ──
	bodyContent := body.String()
	m.viewVP.Width = w
	m.viewVP.Height = m.viewportHeight()
	m.viewVP.SetContent(bodyContent)

	// Auto-scroll to keep the focused item visible.
	focusLine := m.estimateViewFocusLine()
	if focusLine < m.viewVP.YOffset {
		m.viewVP.SetYOffset(focusLine)
	} else if focusLine >= m.viewVP.YOffset+m.viewVP.Height {
		m.viewVP.SetYOffset(focusLine - m.viewVP.Height + 1)
	}

	// ── Assemble final modal: viewport + help bar ──
	var final strings.Builder
	final.WriteString(m.viewVP.View())
	final.WriteString("\n")

	// Help bar (always visible, outside the viewport).
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
	final.WriteString("  " + strings.Join(helpParts, MutedStyle.Render("  ·  ")))

	content := final.String()

	// The modal frame adds border(2) + padding(2) = 4 rows.
	// viewport.Height + help_bar(1) + separator_newline(1) = viewport.Height + 2
	// Total rendered = viewport.Height + 2 + 4 = viewport.Height + 6
	// Since viewportHeight = m.height - 6, total = m.height. Exactly fits.
	modal := ModalStyle.Width(w).Render(content)

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
			// Render textarea as a single block with left margin.
			// DO NOT split by \n — that corrupts ANSI escape sequences
			// spanning multiple lines in the textarea's rendered output.
			taView := strings.TrimRight(m.valueArea.View(), "\n")
			indented := lipgloss.NewStyle().MarginLeft(4).Render(taView)
			b.WriteString(indented)
			b.WriteString("\n")
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
		)
		// Show "ctrl+r hide" when value is revealed, "ctrl+r peek" when masked.
		if m.valueRevealed {
			helpParts = append(helpParts,
				HelpKeyStyle.Render("ctrl+r")+" "+HelpDescStyle.Render("hide"),
			)
		} else {
			helpParts = append(helpParts,
				HelpKeyStyle.Render("ctrl+r")+" "+HelpDescStyle.Render("peek"),
			)
		}
		helpParts = append(helpParts,
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

	// The textarea has built-in scrolling and textareaHeight() is
	// calculated to fit within the terminal. Apply a lipgloss MaxHeight
	// safety cap to guarantee we never overflow even if the line math
	// is slightly off. Account for modal frame: border (2) + padding (2) = 4.
	maxH := m.height - 4
	if maxH < 16 {
		maxH = 16
	}

	// For forms WITHOUT textarea, constrain content via viewportRender
	// to handle cases where many field pairs overflow the terminal.
	if !m.useTextarea {
		maxContent := maxH
		contentLines := strings.Count(content, "\n") + 1
		if contentLines > maxContent {
			content, _ = viewportRender(content, maxContent, 0, m.estimateEditFocusLine())
		}
	}

	modal := ModalStyle.Width(w).MaxHeight(maxH).Render(content)

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
	// Safety cap: ensure the rendered modal never exceeds terminal height.
	// oh accounts for ~80% terminal but includes the frame; MaxHeight caps the
	// entire rendered output at the terminal height.
	maxH := th
	if maxH < 12 {
		maxH = 12
	}
	modal := ModalStyle.Width(contentWidth).MaxHeight(maxH).Render(content)

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

// viewportHeight returns the height available for the viewport in view mode.
// This accounts for the modal frame (border + padding = 4 rows) and the
// content that sits OUTSIDE the viewport (help bar = 1 row, blank before
// help = 1 row).
func (m SecretModal) viewportHeight() int {
	// Modal frame: border(2) + padding(2) = 4
	// Help bar at bottom: 1
	// Newline separator: 1
	// Total non-viewport overhead: 6
	h := m.height - 6
	if h < 3 {
		h = 3
	}
	return h
}

// textareaHeight returns the ideal textarea height given how many other
// fixed lines the edit/add form needs. This ensures the overall modal
// (including borders/padding) fits within the terminal height.
//
// The textarea has built-in scrolling, so clamping its height does NOT
// hide content — the user simply scrolls within the component.
//
// Height budget breakdown:
//
//	Terminal height (m.height)
//	- Modal frame: border(2) + padding(2)         = 4  (ModalStyle)
//	- MaxHeight safety: editModeView uses MaxHeight(m.height-4)
//	  so the content area is at most m.height - 4 - 4 = m.height - 8
//
// Fixed content lines in editModeView (NOT the textarea):
//
//	title(1) + blank(1)                            = 2
//	name(1) + env(1) + value label(1) + meta(1)    = 4
//	blank before help(1) + help bar(1)              = 2
//	Total fixed                                     = 8
//
// Therefore: textarea height = (m.height - 8) - 8 = m.height - 16
func (m SecretModal) textareaHeight() int {
	// Fixed lines in the content area that are NOT the textarea.
	overhead := 8

	// Dynamic field pairs: blank + header line + 2 lines per pair.
	if len(m.fieldPairs) > 0 {
		overhead += 2 + len(m.fieldPairs)*2
	}

	// Toast adds blank + toast line = 2.
	if m.toast != "" {
		overhead += 2
	}

	// The modal frame consumes border(2) + padding(2) = 4 rows.
	// editModeView() uses MaxHeight(m.height - 4), so the rendered modal
	// (including frame) is capped at m.height - 4 rows. The content area
	// inside that is (m.height - 4) - 4 = m.height - 8.
	maxContent := m.height - 8
	if maxContent < 12 {
		maxContent = 12
	}

	avail := maxContent - overhead
	if avail < 3 {
		avail = 3
	}
	return avail
}

// recalcTextareaHeight updates the textarea component height to match the
// current terminal dimensions and form state. Call this after any change
// that affects the available space (window resize, field add/remove, toast).
func (m *SecretModal) recalcTextareaHeight() {
	if !m.useTextarea {
		return
	}
	m.valueArea.SetHeight(m.textareaHeight())
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

// estimateEditFocusLine estimates the line number (0-indexed) of the
// currently focused field in edit mode, so viewportRender can auto-scroll
// to keep it visible when the form overflows the terminal.
func (m SecretModal) estimateEditFocusLine() int {
	// title(1) + blank(1) = 2
	line := 2

	// Fixed fields: name(1), env(1), value(1), metadata(1)
	switch {
	case m.focusField <= fieldName:
		return line
	case m.focusField <= fieldEnv:
		return line + 1
	case m.focusField <= fieldValue:
		return line + 2
	case m.focusField <= fieldMetadata:
		// Value field with textarea takes more lines.
		if m.useTextarea {
			return line + 3 + m.textareaHeight()
		}
		return line + 3
	}

	// Dynamic field pairs start after fixed fields.
	// Offset: 4 fixed field lines + textarea if present + blank + header
	offset := 4
	if m.useTextarea {
		offset += m.textareaHeight()
	}
	if len(m.fieldPairs) > 0 {
		offset += 2 // blank + header
	}

	idx := m.focusField - fixedFieldCount
	pairIdx := idx / 2
	isValue := idx%2 == 1

	pairLine := offset + pairIdx*2
	if isValue {
		pairLine++
	}
	return line + pairLine
}
