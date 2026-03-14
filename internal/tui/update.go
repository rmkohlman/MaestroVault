package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rmkohlman/MaestroVault/internal/clipboard"
)

// ── Update ────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case secretsLoadedMsg:
		m.secrets = msg.secrets
		m.rebuildDisplay()
		m.clampCursor()
		m.adjustScroll()
		return m, nil

	case secretDetailMsg:
		m.secrets = updateEntry(m.secrets, msg.entry)
		m.rebuildDisplay()
		if m.screen == screenDetail {
			// Refresh the detail view with decrypted value.
			for i, s := range m.display {
				if s.Name == msg.entry.Name && s.Environment == msg.entry.Environment {
					m.display[i] = *msg.entry
					break
				}
			}
		}
		return m, nil

	case statusMsg:
		m.status = msg.text
		m.toast = msg.text
		m.toastKind = "success"
		return m, tea.Batch(loadSecrets(m.vault), clearToastAfter(3*time.Second))

	case errMsg:
		m.err = msg.err
		m.toast = msg.err.Error()
		m.toastKind = "error"
		return m, clearToastAfter(5 * time.Second)

	case toastMsg:
		m.toast = msg.text
		m.toastKind = msg.kind
		return m, clearToastAfter(3 * time.Second)

	case toastClearMsg:
		m.toast = ""
		m.toastKind = ""
		return m, nil

	case vaultInfoMsg:
		m.vaultInfo = msg.info
		m.showInfo = true
		return m, nil

	case clipboardMsg:
		m.toast = fmt.Sprintf("Copied %q to clipboard (clears in 45s)", msg.name)
		m.toastKind = "success"
		return m, clearToastAfter(3 * time.Second)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward text input updates when in insert mode or search.
	if m.screen == screenSetName {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
	if m.screen == screenSetEnv {
		var cmd tea.Cmd
		m.envInput, cmd = m.envInput.Update(msg)
		return m, cmd
	}
	if m.screen == screenSetValue {
		var cmd tea.Cmd
		m.valueInput, cmd = m.valueInput.Update(msg)
		return m, cmd
	}
	if m.screen == screenSetMetadata {
		var cmd tea.Cmd
		m.metadataInput, cmd = m.metadataInput.Update(msg)
		return m, cmd
	}
	if m.searchActive {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	if m.showGenerator && m.gen.cursor == genOptName {
		var cmd tea.Cmd
		m.gen.nameInput, cmd = m.gen.nameInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// ── Key dispatch ──────────────────────────────────────────────

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit: ctrl+c always quits.
	if msg.Type == tea.KeyCtrlC {
		m.quitting = true
		return m, tea.Quit
	}

	// Overlay handlers take priority.
	if m.showHelp {
		return m.handleHelpKey(msg)
	}
	if m.showGenerator {
		return m.handleGeneratorKey(msg)
	}
	if m.showInfo {
		return m.handleInfoKey(msg)
	}

	if m.vimEnabled {
		switch m.mode {
		case ModeNormal:
			return m.handleNormal(msg)
		case ModeVisual:
			return m.handleVisual(msg)
		case ModeInsert:
			return m.handleInsert(msg)
		}
		return m, nil
	}

	return m.handleKeySimple(msg)
}

// ── Normal mode ───────────────────────────────────────────────

func (m Model) handleNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Search mode entry/handling.
	if m.searchActive {
		return m.handleSearchKey(msg)
	}

	switch m.screen {
	case screenList:
		return m.handleNormalList(msg, key)
	case screenDetail:
		return m.handleNormalDetail(msg, key)
	case screenConfirmDelete:
		return m.handleConfirmDelete(msg, key)
	}

	return m, nil
}

func (m Model) handleNormalList(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	// Handle pending key sequences (gg, dd).
	if m.pendingKey != "" {
		return m.handlePendingKey(key)
	}

	switch key {
	// Movement.
	case "j", "down":
		m.cursor = minVal(m.cursor+1, maxIdx(m.display))
		m.adjustScroll()
	case "k", "up":
		m.cursor = maxVal(m.cursor-1, 0)
		m.adjustScroll()
	case "G":
		m.cursor = maxIdx(m.display)
		m.adjustScroll()
	case "g":
		m.pendingKey = "g"
		return m, nil

	// Half / full page scroll.
	case "ctrl+d":
		m.cursor = minVal(m.cursor+m.halfPage(), maxIdx(m.display))
		m.adjustScroll()
	case "ctrl+u":
		m.cursor = maxVal(m.cursor-m.halfPage(), 0)
		m.adjustScroll()
	case "ctrl+f":
		m.cursor = minVal(m.cursor+m.fullPage(), maxIdx(m.display))
		m.adjustScroll()
	case "ctrl+b":
		m.cursor = maxVal(m.cursor-m.fullPage(), 0)
		m.adjustScroll()

	// View secret.
	case "enter", "l":
		if s := m.currentSecret(); s != nil {
			m.screen = screenDetail
			m.valueMasked = true
			m.selectedEnv = s.Environment
			return m, getSecret(m.vault, s.Name, s.Environment)
		}

	// Add (insert mode).
	case "i", "a", "o":
		m.mode = ModeInsert
		m.screen = screenSetName
		m.editing = false
		m.editName = ""
		m.editEnv = ""
		m.nameInput.SetValue("")
		m.envInput.SetValue("")
		m.valueInput.SetValue("")
		m.metadataInput.SetValue("")
		m.nameInput.Focus()
		return m, textinput.Blink

	// Edit current.
	case "e":
		if s := m.currentSecret(); s != nil {
			m.mode = ModeInsert
			m.screen = screenSetName
			m.editing = true
			m.editName = s.Name
			m.editEnv = s.Environment
			m.nameInput.SetValue(s.Name)
			m.envInput.SetValue(s.Environment)
			m.valueInput.SetValue("")
			m.metadataInput.SetValue(formatMetadataPlain(s.Metadata))
			m.nameInput.Focus()
			return m, textinput.Blink
		}

	// Visual mode.
	case "v", "V":
		m.mode = ModeVisual
		m.visualAnchor = m.cursor

	// Delete.
	case "d":
		m.pendingKey = "d"
		return m, nil
	case "x":
		if s := m.currentSecret(); s != nil {
			m.selectedEnv = s.Environment
			m.screen = screenConfirmDelete
		}

	// Copy.
	case "c":
		if s := m.currentSecret(); s != nil {
			return m, copyToClipboard(m.vault, s.Name, s.Environment)
		}

	// Search.
	case "/":
		m.searchActive = true
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, textinput.Blink

	// Sort toggle.
	case "s":
		m.sortOrder = m.sortOrder.Next()
		m.rebuildDisplay()
		m.clampCursor()
		m.adjustScroll()

	// Generator.
	case "g ": // fallthrough if pendingKey "g" followed by non-"g" handled elsewhere
		// Not used here, "g" is handled via pendingKey.
	case "G ": // Not used.

	// Open generator overlay.
	case "n":
		m.gen = newGenState()
		m.showGenerator = true

	// Vault info.
	case "I":
		return m, loadVaultInfo(m.vault)

	// Refresh.
	case "r":
		return m, loadSecrets(m.vault)

	// Help.
	case "?":
		m.showHelp = true

	// Quit.
	case "q":
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handlePendingKey(key string) (tea.Model, tea.Cmd) {
	pending := m.pendingKey
	m.pendingKey = ""

	switch pending {
	case "g":
		if key == "g" {
			// gg = go to top
			m.cursor = 0
			m.adjustScroll()
		}
		// Any other key after "g" is ignored.

	case "d":
		if key == "d" {
			// dd = delete current
			if s := m.currentSecret(); s != nil {
				m.selectedEnv = s.Environment
				m.screen = screenConfirmDelete
			}
		}
	}

	return m, nil
}

func (m Model) handleNormalDetail(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "h", "esc":
		m.screen = screenList
		m.valueMasked = true
	case " ":
		m.valueMasked = !m.valueMasked
	case "c":
		if s := m.currentSecret(); s != nil {
			return m, copyToClipboard(m.vault, s.Name, s.Environment)
		}
	case "e":
		if s := m.currentSecret(); s != nil {
			m.mode = ModeInsert
			m.screen = screenSetName
			m.editing = true
			m.editName = s.Name
			m.editEnv = s.Environment
			m.nameInput.SetValue(s.Name)
			m.envInput.SetValue(s.Environment)
			m.valueInput.SetValue("")
			m.metadataInput.SetValue(formatMetadataPlain(s.Metadata))
			m.nameInput.Focus()
			return m, textinput.Blink
		}
	case "d":
		if s := m.currentSecret(); s != nil {
			m.selectedEnv = s.Environment
		}
		m.screen = screenConfirmDelete
	case "q":
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

// ── Visual mode ───────────────────────────────────────────────

func (m Model) handleVisual(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "j", "down":
		m.cursor = minVal(m.cursor+1, maxIdx(m.display))
		m.adjustScroll()
	case "k", "up":
		m.cursor = maxVal(m.cursor-1, 0)
		m.adjustScroll()
	case "G":
		m.cursor = maxIdx(m.display)
		m.adjustScroll()
	case "g":
		m.pendingKey = "g"
	case "d", "x":
		refs := m.selectedRefs()
		if len(refs) > 0 {
			m.screen = screenConfirmDelete
		}
		m.mode = ModeNormal
	case "esc":
		m.mode = ModeNormal
	}

	return m, nil
}

// ── Insert mode ───────────────────────────────────────────────

func (m Model) handleInsert(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.screen {
	case screenSetName:
		switch key {
		case "enter":
			name := m.nameInput.Value()
			if name == "" {
				m.toast = "Name cannot be empty"
				m.toastKind = "error"
				return m, clearToastAfter(3 * time.Second)
			}
			m.screen = screenSetEnv
			m.nameInput.Blur()
			m.envInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.mode = ModeNormal
			m.screen = screenList
			m.nameInput.Blur()
		default:
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(msg)
			return m, cmd
		}

	case screenSetEnv:
		switch key {
		case "enter":
			m.screen = screenSetValue
			m.envInput.Blur()
			m.valueInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.mode = ModeNormal
			m.screen = screenList
			m.envInput.Blur()
		default:
			var cmd tea.Cmd
			m.envInput, cmd = m.envInput.Update(msg)
			return m, cmd
		}

	case screenSetValue:
		switch key {
		case "enter":
			value := m.valueInput.Value()
			if value == "" {
				m.toast = "Value cannot be empty"
				m.toastKind = "error"
				return m, clearToastAfter(3 * time.Second)
			}
			m.screen = screenSetMetadata
			m.valueInput.Blur()
			m.metadataInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.mode = ModeNormal
			m.screen = screenList
			m.valueInput.Blur()
		default:
			var cmd tea.Cmd
			m.valueInput, cmd = m.valueInput.Update(msg)
			return m, cmd
		}

	case screenSetMetadata:
		switch key {
		case "enter":
			name := m.nameInput.Value()
			env := m.envInput.Value()
			value := m.valueInput.Value()
			metadata := parseMetadataInput(m.metadataInput.Value())
			m.mode = ModeNormal
			m.screen = screenList
			m.metadataInput.Blur()
			return m, setSecret(m.vault, name, env, value, metadata)
		case "esc":
			m.mode = ModeNormal
			m.screen = screenList
			m.metadataInput.Blur()
		default:
			var cmd tea.Cmd
			m.metadataInput, cmd = m.metadataInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// ── Confirm delete ────────────────────────────────────────────

func (m Model) handleConfirmDelete(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y", "enter":
		refs := m.selectedRefs()
		m.screen = screenList
		m.mode = ModeNormal
		if len(refs) == 1 {
			return m, deleteSecret(m.vault, refs[0].Name, refs[0].Environment)
		}
		if len(refs) > 1 {
			return m, deleteSecrets(m.vault, refs)
		}
	case "n", "N", "esc", "q":
		m.screen = screenList
		m.mode = ModeNormal
	}
	return m, nil
}

// ── Search key handling ───────────────────────────────────────

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		m.searchActive = false
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.rebuildDisplay()
		m.clampCursor()
		m.adjustScroll()
	case "enter":
		m.searchActive = false
		m.searchInput.Blur()
		// Keep filter active with current query.
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		// Live filter as user types.
		m.rebuildDisplay()
		m.clampCursor()
		m.adjustScroll()
		return m, cmd
	}

	return m, nil
}

// ── Help overlay ──────────────────────────────────────────────

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "q":
		m.showHelp = false
	}
	return m, nil
}

// ── Generator overlay ─────────────────────────────────────────

func (m Model) handleGeneratorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		m.showGenerator = false
	case "j", "down":
		m.gen.cursor = (m.gen.cursor + 1) % genOptCount
		if m.gen.cursor == genOptName {
			m.gen.nameInput.Focus()
			return m, textinput.Blink
		}
		m.gen.nameInput.Blur()
	case "k", "up":
		m.gen.cursor = (m.gen.cursor - 1 + genOptCount) % genOptCount
		if m.gen.cursor == genOptName {
			m.gen.nameInput.Focus()
			return m, textinput.Blink
		}
		m.gen.nameInput.Blur()
	case "h", "left":
		if m.gen.cursor == genOptLength && m.gen.length > 8 {
			m.gen.length--
			m.gen.regenerate()
		}
	case "l", "right":
		if m.gen.cursor == genOptLength && m.gen.length < 128 {
			m.gen.length++
			m.gen.regenerate()
		}
	case " ":
		switch m.gen.cursor {
		case genOptUppercase:
			m.gen.uppercase = !m.gen.uppercase
			m.gen.regenerate()
		case genOptLowercase:
			m.gen.lowercase = !m.gen.lowercase
			m.gen.regenerate()
		case genOptDigits:
			m.gen.digits = !m.gen.digits
			m.gen.regenerate()
		case genOptSymbols:
			m.gen.symbols = !m.gen.symbols
			m.gen.regenerate()
		}
	case "r":
		m.gen.regenerate()
	case "c":
		if m.gen.preview != "" && m.gen.preview != "(error)" {
			preview := m.gen.preview
			return m, func() tea.Msg {
				if err := clipboard.Copy(preview); err != nil {
					return errMsg{fmt.Errorf("clipboard: %w", err)}
				}
				return toastMsg{text: "Password copied to clipboard", kind: "success"}
			}
		}
	case "enter":
		name := m.gen.nameInput.Value()
		if name != "" {
			value := m.gen.preview
			m.showGenerator = false
			return m, setSecret(m.vault, name, "", value, nil)
		}
		// No name: just close.
		m.showGenerator = false
	default:
		// Forward to name input when it's focused.
		if m.gen.cursor == genOptName {
			var cmd tea.Cmd
			m.gen.nameInput, cmd = m.gen.nameInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// ── Info overlay ──────────────────────────────────────────────

func (m Model) handleInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "I":
		m.showInfo = false
	}
	return m, nil
}

// ── Simple (non-vim) key handling ─────────────────────────────

func (m Model) handleKeySimple(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Search mode.
	if m.searchActive {
		return m.handleSearchKey(msg)
	}

	switch m.screen {
	case screenList:
		switch key {
		case "up", "k":
			m.cursor = maxVal(m.cursor-1, 0)
			m.adjustScroll()
		case "down", "j":
			m.cursor = minVal(m.cursor+1, maxIdx(m.display))
			m.adjustScroll()
		case "enter":
			if s := m.currentSecret(); s != nil {
				m.screen = screenDetail
				m.valueMasked = true
				m.selectedEnv = s.Environment
				return m, getSecret(m.vault, s.Name, s.Environment)
			}
		case "a":
			m.screen = screenSetName
			m.editing = false
			m.editName = ""
			m.editEnv = ""
			m.nameInput.SetValue("")
			m.envInput.SetValue("")
			m.valueInput.SetValue("")
			m.metadataInput.SetValue("")
			m.nameInput.Focus()
			return m, textinput.Blink
		case "e":
			if s := m.currentSecret(); s != nil {
				m.screen = screenSetName
				m.editing = true
				m.editName = s.Name
				m.editEnv = s.Environment
				m.nameInput.SetValue(s.Name)
				m.envInput.SetValue(s.Environment)
				m.valueInput.SetValue("")
				m.metadataInput.SetValue(formatMetadataPlain(s.Metadata))
				m.nameInput.Focus()
				return m, textinput.Blink
			}
		case "d":
			if s := m.currentSecret(); s != nil {
				m.selectedEnv = s.Environment
				m.screen = screenConfirmDelete
			}
		case "c":
			if s := m.currentSecret(); s != nil {
				return m, copyToClipboard(m.vault, s.Name, s.Environment)
			}
		case "/":
			m.searchActive = true
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			return m, textinput.Blink
		case "s":
			m.sortOrder = m.sortOrder.Next()
			m.rebuildDisplay()
			m.clampCursor()
			m.adjustScroll()
		case "n":
			m.gen = newGenState()
			m.showGenerator = true
		case "?":
			m.showHelp = true
		case "r":
			return m, loadSecrets(m.vault)
		case "q":
			m.quitting = true
			return m, tea.Quit
		}

	case screenDetail:
		switch key {
		case "esc":
			m.screen = screenList
			m.valueMasked = true
		case " ":
			m.valueMasked = !m.valueMasked
		case "c":
			if s := m.currentSecret(); s != nil {
				return m, copyToClipboard(m.vault, s.Name, s.Environment)
			}
		case "e":
			if s := m.currentSecret(); s != nil {
				m.screen = screenSetName
				m.editing = true
				m.editName = s.Name
				m.editEnv = s.Environment
				m.nameInput.SetValue(s.Name)
				m.envInput.SetValue(s.Environment)
				m.valueInput.SetValue("")
				m.metadataInput.SetValue(formatMetadataPlain(s.Metadata))
				m.nameInput.Focus()
				return m, textinput.Blink
			}
		case "d":
			if s := m.currentSecret(); s != nil {
				m.selectedEnv = s.Environment
			}
			m.screen = screenConfirmDelete
		case "q":
			m.quitting = true
			return m, tea.Quit
		}

	case screenSetName:
		switch key {
		case "enter":
			name := m.nameInput.Value()
			if name == "" {
				m.toast = "Name cannot be empty"
				m.toastKind = "error"
				return m, clearToastAfter(3 * time.Second)
			}
			m.screen = screenSetEnv
			m.nameInput.Blur()
			m.envInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.screen = screenList
			m.nameInput.Blur()
		default:
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(msg)
			return m, cmd
		}

	case screenSetEnv:
		switch key {
		case "enter":
			m.screen = screenSetValue
			m.envInput.Blur()
			m.valueInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.screen = screenList
			m.envInput.Blur()
		default:
			var cmd tea.Cmd
			m.envInput, cmd = m.envInput.Update(msg)
			return m, cmd
		}

	case screenSetValue:
		switch key {
		case "enter":
			value := m.valueInput.Value()
			if value == "" {
				m.toast = "Value cannot be empty"
				m.toastKind = "error"
				return m, clearToastAfter(3 * time.Second)
			}
			m.screen = screenSetMetadata
			m.valueInput.Blur()
			m.metadataInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.screen = screenList
			m.valueInput.Blur()
		default:
			var cmd tea.Cmd
			m.valueInput, cmd = m.valueInput.Update(msg)
			return m, cmd
		}

	case screenSetMetadata:
		switch key {
		case "enter":
			name := m.nameInput.Value()
			env := m.envInput.Value()
			value := m.valueInput.Value()
			metadata := parseMetadataInput(m.metadataInput.Value())
			m.screen = screenList
			m.metadataInput.Blur()
			return m, setSecret(m.vault, name, env, value, metadata)
		case "esc":
			m.screen = screenList
			m.metadataInput.Blur()
		default:
			var cmd tea.Cmd
			m.metadataInput, cmd = m.metadataInput.Update(msg)
			return m, cmd
		}

	case screenConfirmDelete:
		return m.handleConfirmDelete(msg, key)
	}

	return m, nil
}
