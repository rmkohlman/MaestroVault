package tui

import (
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
	screenSetName
	screenSetValue
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
	cursor    int // 0=length, 1=upper, 2=lower, 3=digits, 4=symbols, 5=name
	nameInput textinput.Model
}

const (
	genOptLength    = 0
	genOptUppercase = 1
	genOptLowercase = 2
	genOptDigits    = 3
	genOptSymbols   = 4
	genOptName      = 5
	genOptCount     = 6
)

func newGenState() genState {
	ni := textinput.New()
	ni.Placeholder = "secret name (optional, to save)"
	ni.CharLimit = 128

	g := genState{
		length:    32,
		uppercase: true,
		lowercase: true,
		digits:    true,
		symbols:   true,
		cursor:    0,
		nameInput: ni,
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

// ── Model ─────────────────────────────────────────────────────

// Model is the top-level Bubbletea model for MaestroVault.
type Model struct {
	vault   *vault.Vault
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

	// Text inputs for the set/edit flow.
	nameInput  textinput.Model
	valueInput textinput.Model
	editing    bool   // true when editing existing secret
	editName   string // original name of secret being edited

	// Search.
	searchActive bool
	searchInput  textinput.Model

	// Detail view.
	valueMasked bool // value masked by default in detail view

	// Sort.
	sortOrder SortOrder

	// Overlays.
	showHelp      bool
	showGenerator bool
	showInfo      bool
	gen           genState
	vaultInfo     *vault.VaultInfo

	// Toast notification.
	toast     string
	toastKind string // "success", "error", "info"

	// Ephemeral status message (legacy compat).
	status string
}

// Opts configures optional TUI behavior.
type Opts struct {
	VimMode bool
}

// New creates a new TUI model backed by an open vault.
func New(v *vault.Vault, opts Opts) Model {
	ni := textinput.New()
	ni.Placeholder = "secret-name"
	ni.CharLimit = 128

	vi := textinput.New()
	vi.Placeholder = "secret value"
	vi.CharLimit = 4096
	vi.EchoMode = textinput.EchoPassword

	si := textinput.New()
	si.Placeholder = "search..."
	si.CharLimit = 128

	return Model{
		vault:       v,
		screen:      screenList,
		vimEnabled:  opts.VimMode,
		mode:        ModeNormal,
		nameInput:   ni,
		valueInput:  vi,
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

// ── Commands ──────────────────────────────────────────────────

func loadSecrets(v *vault.Vault) tea.Cmd {
	return func() tea.Msg {
		secrets, err := v.List()
		if err != nil {
			return errMsg{err}
		}
		return secretsLoadedMsg{secrets}
	}
}

func getSecret(v *vault.Vault, name string) tea.Cmd {
	return func() tea.Msg {
		entry, err := v.Get(name)
		if err != nil {
			return errMsg{err}
		}
		return secretDetailMsg{entry}
	}
}

func setSecret(v *vault.Vault, name, value string, labels map[string]string) tea.Cmd {
	return func() tea.Msg {
		if err := v.Set(name, value, labels); err != nil {
			return errMsg{err}
		}
		return statusMsg{fmt.Sprintf("Secret %q stored.", name)}
	}
}

func deleteSecret(v *vault.Vault, name string) tea.Cmd {
	return func() tea.Msg {
		if err := v.Delete(name); err != nil {
			return errMsg{err}
		}
		return statusMsg{fmt.Sprintf("Secret %q deleted.", name)}
	}
}

func deleteSecrets(v *vault.Vault, names []string) tea.Cmd {
	return func() tea.Msg {
		for _, name := range names {
			if err := v.Delete(name); err != nil {
				return errMsg{err}
			}
		}
		return statusMsg{fmt.Sprintf("%d secret(s) deleted.", len(names))}
	}
}

func copyToClipboard(v *vault.Vault, name string) tea.Cmd {
	return func() tea.Msg {
		entry, err := v.Get(name)
		if err != nil {
			return errMsg{err}
		}
		if _, err := clipboard.CopyWithClear(entry.Value, 45*time.Second); err != nil {
			return errMsg{fmt.Errorf("clipboard: %w", err)}
		}
		return clipboardMsg{name}
	}
}

func loadVaultInfo(v *vault.Vault) tea.Cmd {
	return func() tea.Msg {
		info, err := v.Info()
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
			if matchesQuery(s, query) {
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
			return secrets[i].Name < secrets[j].Name
		})
	case SortNameDesc:
		sort.Slice(secrets, func(i, j int) bool {
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
