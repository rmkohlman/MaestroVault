package tui

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rmkohlman/MaestroVault/internal/clipboard"
	"github.com/rmkohlman/MaestroVault/internal/crypto"
	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// ── Screen the TUI is displaying ──────────────────────────────

type screen int

const (
	screenList screen = iota
	screenDetail
	screenConfirmDelete
)

// ── Vim mode ──────────────────────────────────────────────────

type vimMode int

const (
	ModeNormal vimMode = iota
	ModeInsert
	ModeVisual
)

func (m vimMode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeVisual:
		return "VISUAL"
	default:
		return ""
	}
}

// ── Sort order ────────────────────────────────────────────────

type SortOrder int

const (
	SortNameAsc SortOrder = iota
	SortNameDesc
	SortNewest
	SortOldest
)

func (s SortOrder) String() string {
	switch s {
	case SortNameAsc:
		return "name A-Z"
	case SortNameDesc:
		return "name Z-A"
	case SortNewest:
		return "newest"
	case SortOldest:
		return "oldest"
	default:
		return ""
	}
}

func (s SortOrder) Next() SortOrder {
	return (s + 1) % 4
}

// ── Generator state ───────────────────────────────────────────

type genState struct {
	length    int
	uppercase bool
	lowercase bool
	digits    bool
	symbols   bool
	preview   string
	cursor    int // 0=length, 1=upper, 2=lower, 3=digits, 4=symbols, 5=name, 6=env
	nameInput textinput.Model
	envInput  textinput.Model

	// Save feedback state.
	saving    bool
	savedMsg  string
	toast     string
	toastKind string
}

const (
	genOptLength    = 0
	genOptUppercase = 1
	genOptLowercase = 2
	genOptDigits    = 3
	genOptSymbols   = 4
	genOptName      = 5
	genOptEnv       = 6
	genOptCount     = 7
)

func newGenState() genState {
	ni := textinput.New()
	ni.Placeholder = "secret name (optional, to save)"
	ni.CharLimit = 128

	ei := textinput.New()
	ei.Placeholder = "environment (optional)"
	ei.CharLimit = 128

	g := genState{
		length:    32,
		uppercase: true,
		lowercase: true,
		digits:    true,
		symbols:   true,
		cursor:    0,
		nameInput: ni,
		envInput:  ei,
	}
	g.regenerate()
	return g
}

func (g *genState) regenerate() {
	opts := crypto.GenerateOpts{
		Length:    g.length,
		Uppercase: g.uppercase,
		Lowercase: g.lowercase,
		Digits:    g.digits,
		Symbols:   g.symbols,
	}
	pw, err := crypto.GeneratePassword(opts)
	if err != nil {
		g.preview = "(error)"
		return
	}
	g.preview = pw
}

func (g *genState) opts() crypto.GenerateOpts {
	return crypto.GenerateOpts{
		Length:    g.length,
		Uppercase: g.uppercase,
		Lowercase: g.lowercase,
		Digits:    g.digits,
		Symbols:   g.symbols,
	}
}

// ── Settings overlay ──────────────────────────────────────────

const (
	settingVimMode     = 0
	settingTouchID     = 1
	settingFuzzySearch = 2
	settingCount       = 3
)

// ── secretRef identifies a secret by name + environment ───────

type secretRef struct {
	Name        string
	Environment string
}

// ── Model ─────────────────────────────────────────────────────

// Model is the top-level Bubbletea model for MaestroVault.
type Model struct {
	vault   vault.Vault         // interface (not pointer)
	secrets []vault.SecretEntry // full list from vault
	display []vault.SecretEntry // sorted + filtered view
	err     error

	screen       screen
	cursor       int
	scrollOffset int
	width        int
	height       int
	quitting     bool

	// Vim mode support.
	vimEnabled   bool
	mode         vimMode
	pendingKey   string
	visualAnchor int

	// Search.
	searchActive bool
	searchInput  textinput.Model
	fuzzySearch  bool // true = fuzzy matching, false = substring

	// Detail view.
	valueMasked bool // value masked by default in detail view

	// Secret modal (unified add/edit/view overlay).
	showSecretModal bool
	secretModal     SecretModal

	// Selected secret tracking (name + environment).
	selectedEnv string // environment of the currently selected secret

	// Sort.
	sortOrder SortOrder

	// Overlays.
	showHelp      bool
	showGenerator bool
	showInfo      bool
	showSettings  bool
	gen           genState
	vaultInfo     *vault.VaultInfo

	// Settings overlay state.
	settingsCursor int
	settingsConfig vault.Config

	// Help overlay scroll offset.
	helpScroll int

	// Toast notification.
	toast     string
	toastKind string // "success", "error", "info"

	// Ephemeral status message (legacy compat).
	status string
}

// Opts configures optional TUI behavior.
type Opts struct {
	VimMode     bool
	FuzzySearch bool
}

// New creates a new TUI model backed by an open vault.
func New(v vault.Vault, opts Opts) Model {
	si := textinput.New()
	si.Placeholder = "search..."
	si.CharLimit = 128

	return Model{
		vault:       v,
		screen:      screenList,
		vimEnabled:  opts.VimMode,
		fuzzySearch: opts.FuzzySearch,
		mode:        ModeNormal,
		searchInput: si,
		valueMasked: true,
		sortOrder:   SortNameAsc,
		gen:         newGenState(),
	}
}

// ── Messages ──────────────────────────────────────────────────

type secretsLoadedMsg struct{ secrets []vault.SecretEntry }
type secretDetailMsg struct{ entry *vault.SecretEntry }
type errMsg struct{ err error }
type statusMsg struct{ text string }
type toastMsg struct {
	text string
	kind string // "success", "error", "info"
}
type toastClearMsg struct{}
type vaultInfoMsg struct{ info *vault.VaultInfo }
type clipboardMsg struct{ name string }
type editDetailMsg struct{ entry *vault.SecretEntry }
type configSavedMsg struct{}

type genSaveResultMsg struct {
	saved bool
	toast string
	kind  string
}

type genDoneMsg struct {
	toast string
	kind  string
}

// ── Commands ──────────────────────────────────────────────────

func loadSecrets(v vault.Vault) tea.Cmd {
	return func() tea.Msg {
		secrets, err := v.List(context.Background(), "") // all environments
		if err != nil {
			return errMsg{err}
		}
		return secretsLoadedMsg{secrets}
	}
}

func getSecret(v vault.Vault, name, env string) tea.Cmd {
	return func() tea.Msg {
		entry, err := v.Get(context.Background(), name, env)
		if err != nil {
			return errMsg{err}
		}
		return secretDetailMsg{entry}
	}
}

func fetchForEdit(v vault.Vault, name, env string) tea.Cmd {
	return func() tea.Msg {
		entry, err := v.Get(context.Background(), name, env)
		if err != nil {
			return errMsg{err}
		}
		return editDetailMsg{entry}
	}
}

func setSecret(v vault.Vault, name, env, value string, metadata map[string]any) tea.Cmd {
	return func() tea.Msg {
		if err := v.Set(context.Background(), name, env, value, metadata); err != nil {
			return errMsg{err}
		}
		return statusMsg{fmt.Sprintf("Secret %q stored.", name)}
	}
}

func deleteSecret(v vault.Vault, name, env string) tea.Cmd {
	return func() tea.Msg {
		if err := v.Delete(context.Background(), name, env); err != nil {
			return errMsg{err}
		}
		return statusMsg{fmt.Sprintf("Secret %q deleted.", name)}
	}
}

func deleteSecrets(v vault.Vault, refs []secretRef) tea.Cmd {
	return func() tea.Msg {
		for _, ref := range refs {
			if err := v.Delete(context.Background(), ref.Name, ref.Environment); err != nil {
				return errMsg{err}
			}
		}
		return statusMsg{fmt.Sprintf("%d secret(s) deleted.", len(refs))}
	}
}

func copyToClipboard(v vault.Vault, name, env string) tea.Cmd {
	return func() tea.Msg {
		entry, err := v.Get(context.Background(), name, env)
		if err != nil {
			return errMsg{err}
		}
		if _, err := clipboard.CopyWithClear(entry.Value, 45*time.Second); err != nil {
			return errMsg{fmt.Errorf("clipboard: %w", err)}
		}
		return clipboardMsg{name}
	}
}

func loadVaultInfo(v vault.Vault) tea.Cmd {
	return func() tea.Msg {
		info, err := v.Info(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return vaultInfoMsg{info}
	}
}

func clearToastAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return toastClearMsg{}
	})
}

func saveConfig(cfg vault.Config) tea.Cmd {
	return func() tea.Msg {
		if err := vault.SaveConfig(cfg); err != nil {
			return errMsg{err}
		}
		return configSavedMsg{}
	}
}

func genSaveSecret(v vault.Vault, name, env, value string) tea.Cmd {
	return func() tea.Msg {
		if err := v.Set(context.Background(), name, env, value, nil); err != nil {
			return genSaveResultMsg{toast: err.Error(), kind: "error"}
		}
		return genSaveResultMsg{saved: true, toast: fmt.Sprintf("Secret %q stored.", name), kind: "success"}
	}
}

// ── Init ──────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return loadSecrets(m.vault)
}

// ── Display list management ───────────────────────────────────

func (m *Model) rebuildDisplay() {
	query := m.searchInput.Value()
	var list []vault.SecretEntry

	if query == "" {
		list = make([]vault.SecretEntry, len(m.secrets))
		copy(list, m.secrets)
	} else {
		for _, s := range m.secrets {
			if matchesQuery(s, query, m.fuzzySearch) {
				list = append(list, s)
			}
		}
	}

	sortSecrets(list, m.sortOrder)
	m.display = list
}

func sortSecrets(secrets []vault.SecretEntry, order SortOrder) {
	switch order {
	case SortNameAsc:
		sort.Slice(secrets, func(i, j int) bool {
			if secrets[i].Name == secrets[j].Name {
				return secrets[i].Environment < secrets[j].Environment
			}
			return secrets[i].Name < secrets[j].Name
		})
	case SortNameDesc:
		sort.Slice(secrets, func(i, j int) bool {
			if secrets[i].Name == secrets[j].Name {
				return secrets[i].Environment > secrets[j].Environment
			}
			return secrets[i].Name > secrets[j].Name
		})
	case SortNewest:
		sort.Slice(secrets, func(i, j int) bool {
			return secrets[i].UpdatedAt > secrets[j].UpdatedAt
		})
	case SortOldest:
		sort.Slice(secrets, func(i, j int) bool {
			return secrets[i].UpdatedAt < secrets[j].UpdatedAt
		})
	}
}
