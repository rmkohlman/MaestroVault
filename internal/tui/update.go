package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rmkohlman/MaestroVault/internal/clipboard"
	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// ── Update ────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.showSecretModal {
			var updated tea.Model
			var cmd tea.Cmd
			updated, cmd = m.secretModal.Update(msg)
			m.secretModal = updated.(SecretModal)
			return m, cmd
		}
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

	case editDetailMsg:
		// Received decrypted secret for editing — open modal in edit mode.
		e := msg.entry
		m.secretModal = NewSecretModalEdit(e, m.vault)
		m.secretModal.width = m.width
		m.secretModal.height = m.height
		m.showSecretModal = true
		return m, textinput.Blink

	case secretModalResultMsg:
		m.secretModal.saving = false
		if msg.saved {
			m.secretModal.savedMsg = msg.toast
			m.secretModal.savedKind = msg.kind
			toast := msg.toast
			kind := msg.kind
			return m, tea.Tick(700*time.Millisecond, func(t time.Time) tea.Msg {
				return secretModalDoneMsg{toast: toast, kind: kind, saved: true}
			})
		}
		// Error: show in modal, keep open.
		m.secretModal.toast = msg.toast
		m.secretModal.toastKind = msg.kind
		m.secretModal.recalcTextareaHeight() // toast changes overhead
		return m, nil

	case secretModalDoneMsg:
		m.showSecretModal = false
		if msg.saved {
			m.toast = msg.toast
			m.toastKind = msg.kind
			return m, tea.Batch(loadSecrets(m.vault), clearToastAfter(3*time.Second))
		}
		return m, nil

	case secretModalCloseMsg:
		m.showSecretModal = false
		return m, nil

	case configSavedMsg:
		m.toast = "Settings saved"
		m.toastKind = "success"
		return m, clearToastAfter(3 * time.Second)

	case genSaveResultMsg:
		m.gen.saving = false
		if msg.saved {
			m.gen.savedMsg = msg.toast
			toast := msg.toast
			kind := msg.kind
			return m, tea.Tick(700*time.Millisecond, func(t time.Time) tea.Msg {
				return genDoneMsg{toast: toast, kind: kind}
			})
		}
		m.gen.toast = msg.toast
		m.gen.toastKind = msg.kind
		return m, nil

	case genDoneMsg:
		m.showGenerator = false
		m.toast = msg.toast
		m.toastKind = msg.kind
		return m, tea.Batch(loadSecrets(m.vault), clearToastAfter(3*time.Second))

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward text input updates to active overlay inputs.
	if m.showSecretModal {
		var updated tea.Model
		var cmd tea.Cmd
		updated, cmd = m.secretModal.Update(msg)
		m.secretModal = updated.(SecretModal)
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
	if m.showGenerator && m.gen.cursor == genOptEnv {
		var cmd tea.Cmd
		m.gen.envInput, cmd = m.gen.envInput.Update(msg)
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

	// Secret modal takes priority over all other overlays.
	if m.showSecretModal {
		var updated tea.Model
		var cmd tea.Cmd
		updated, cmd = m.secretModal.Update(msg)
		m.secretModal = updated.(SecretModal)
		return m, cmd
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
	if m.showSettings {
		return m.handleSettingsKey(msg)
	}

	if m.vimEnabled {
		switch m.mode {
		case ModeNormal:
			return m.handleNormal(msg)
		case ModeVisual:
			return m.handleVisual(msg)
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

	// Add (open modal in add mode).
	case "i", "a", "o":
		m.secretModal = NewSecretModalAdd(m.vault)
		m.secretModal.width = m.width
		m.secretModal.height = m.height
		m.showSecretModal = true
		return m, textinput.Blink

	// Edit current.
	case "e":
		if s := m.currentSecret(); s != nil {
			return m, fetchForEdit(m.vault, s.Name, s.Environment)
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

	// Open generator overlay.
	case "n":
		m.gen = newGenState()
		m.showGenerator = true

	// Vault info.
	case "I":
		return m, loadVaultInfo(m.vault)

	// Settings.
	case "S":
		cfg, err := vault.LoadConfig()
		if err != nil {
			m.toast = err.Error()
			m.toastKind = "error"
			return m, clearToastAfter(5 * time.Second)
		}
		m.settingsConfig = cfg
		m.settingsCursor = 0
		m.showSettings = true

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
			return m, fetchForEdit(m.vault, s.Name, s.Environment)
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

	switch {
	case key == "esc":
		m.searchActive = false
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.rebuildDisplay()
		m.clampCursor()
		m.adjustScroll()
	case key == "enter":
		m.searchActive = false
		m.searchInput.Blur()
		// Keep filter active with current query.
	case msg.Type == tea.KeyTab:
		// Toggle between fuzzy and exact search.
		m.fuzzySearch = !m.fuzzySearch
		m.rebuildDisplay()
		m.clampCursor()
		m.adjustScroll()
		return m, nil
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
		m.helpScroll = 0
	case "j", "down":
		m.helpScroll++
	case "k", "up":
		if m.helpScroll > 0 {
			m.helpScroll--
		}
	}
	return m, nil
}

// ── Generator overlay ─────────────────────────────────────────

func (m Model) handleGeneratorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block input during save or saved flash.
	if m.gen.saving || m.gen.savedMsg != "" {
		return m, nil
	}

	key := msg.String()

	switch key {
	case "esc":
		m.showGenerator = false
	case "j", "down":
		m.gen.cursor = (m.gen.cursor + 1) % genOptCount
		if m.gen.cursor == genOptName {
			m.gen.nameInput.Focus()
			m.gen.envInput.Blur()
			return m, textinput.Blink
		}
		if m.gen.cursor == genOptEnv {
			m.gen.envInput.Focus()
			m.gen.nameInput.Blur()
			return m, textinput.Blink
		}
		m.gen.nameInput.Blur()
		m.gen.envInput.Blur()
	case "k", "up":
		m.gen.cursor = (m.gen.cursor - 1 + genOptCount) % genOptCount
		if m.gen.cursor == genOptName {
			m.gen.nameInput.Focus()
			m.gen.envInput.Blur()
			return m, textinput.Blink
		}
		if m.gen.cursor == genOptEnv {
			m.gen.envInput.Focus()
			m.gen.nameInput.Blur()
			return m, textinput.Blink
		}
		m.gen.nameInput.Blur()
		m.gen.envInput.Blur()
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
			env := m.gen.envInput.Value()
			m.gen.saving = true
			m.gen.toast = ""
			return m, genSaveSecret(m.vault, name, env, value)
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
		// Forward to env input when it's focused.
		if m.gen.cursor == genOptEnv {
			var cmd tea.Cmd
			m.gen.envInput, cmd = m.gen.envInput.Update(msg)
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

// ── Settings overlay ──────────────────────────────────────────

func (m Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "j", "down":
		m.settingsCursor = (m.settingsCursor + 1) % settingCount
	case "k", "up":
		m.settingsCursor = (m.settingsCursor - 1 + settingCount) % settingCount
	case " ", "enter":
		switch m.settingsCursor {
		case settingVimMode:
			m.settingsConfig.VimMode = !m.settingsConfig.VimMode
			// Apply immediately so the user sees the effect.
			m.vimEnabled = m.settingsConfig.VimMode
			if !m.vimEnabled {
				m.mode = ModeNormal
			}
		case settingTouchID:
			m.settingsConfig.TouchID = !m.settingsConfig.TouchID
		case settingFuzzySearch:
			m.settingsConfig.FuzzySearch = !m.settingsConfig.FuzzySearch
			m.fuzzySearch = m.settingsConfig.FuzzySearch
		}
	case "esc", "S":
		m.showSettings = false
		// Save config to disk on close.
		return m, saveConfig(m.settingsConfig)
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
			m.secretModal = NewSecretModalAdd(m.vault)
			m.secretModal.width = m.width
			m.secretModal.height = m.height
			m.showSecretModal = true
			return m, textinput.Blink
		case "e":
			if s := m.currentSecret(); s != nil {
				return m, fetchForEdit(m.vault, s.Name, s.Environment)
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
		case "S":
			cfg, err := vault.LoadConfig()
			if err != nil {
				m.toast = err.Error()
				m.toastKind = "error"
				return m, clearToastAfter(5 * time.Second)
			}
			m.settingsConfig = cfg
			m.settingsCursor = 0
			m.showSettings = true
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
				return m, fetchForEdit(m.vault, s.Name, s.Environment)
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

	case screenConfirmDelete:
		return m.handleConfirmDelete(msg, key)
	}

	return m, nil
}
