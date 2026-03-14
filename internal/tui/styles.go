// Package tui provides the terminal styling primitives for MaestroVault.
// All colors reference the ANSI palette so the TUI automatically inherits
// whatever theme the user has active in their terminal (wezterm, kitty, etc.).
package tui

import "github.com/charmbracelet/lipgloss"

// ANSI color references — these map to the terminal's configured palette,
// so the TUI adapts to any theme without configuration.
var (
	ColorBlack   = lipgloss.Color("0")
	ColorRed     = lipgloss.Color("1")
	ColorGreen   = lipgloss.Color("2")
	ColorYellow  = lipgloss.Color("3")
	ColorBlue    = lipgloss.Color("4")
	ColorMagenta = lipgloss.Color("5")
	ColorCyan    = lipgloss.Color("6")
	ColorWhite   = lipgloss.Color("7")

	ColorBrightBlack   = lipgloss.Color("8")
	ColorBrightRed     = lipgloss.Color("9")
	ColorBrightGreen   = lipgloss.Color("10")
	ColorBrightYellow  = lipgloss.Color("11")
	ColorBrightBlue    = lipgloss.Color("12")
	ColorBrightMagenta = lipgloss.Color("13")
	ColorBrightCyan    = lipgloss.Color("14")
	ColorBrightWhite   = lipgloss.Color("15")
)

// Semantic color aliases — map intent to ANSI palette slots.
var (
	ColorPrimary   = ColorBlue
	ColorSecondary = ColorMagenta
	ColorAccent    = ColorCyan
	ColorSuccess   = ColorGreen
	ColorWarning   = ColorYellow
	ColorError     = ColorRed
	ColorMuted     = ColorBrightBlack
	ColorSubtle    = ColorBrightBlack
)

// Reusable style fragments.
var (
	// Title is bold + primary color, used for panel headers.
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	// Subtitle is dimmer, used beneath titles.
	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Accent is used for interactive highlights (selected items, cursor).
	AccentStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	// Success / Warning / Error for status messages.
	SuccessStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	WarningStyle = lipgloss.NewStyle().Foreground(ColorWarning)
	ErrorStyle   = lipgloss.NewStyle().Foreground(ColorError)

	// SecretValue renders secret values in bold green.
	SecretValueStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorGreen)

	// MaskedValueStyle renders masked secret values.
	MaskedValueStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	// Metadata renders key=value metadata (replaces old Label styles).
	MetadataKeyStyle = lipgloss.NewStyle().
				Foreground(ColorMagenta)
	MetadataValueStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	// Environment badge shown next to secret names in list view.
	EnvBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	// Muted text for timestamps, hints, etc.
	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Border box used around panels and modals.
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)

	// Modal style for overlays (help, generator, info).
	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(1, 2)

	// StatusBar at the bottom of the TUI.
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Search bar input.
	SearchStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	SearchLabelStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	// Help text in the status bar.
	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)
	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Toast notification styles.
	ToastSuccessStyle = lipgloss.NewStyle().
				Foreground(ColorBlack).
				Background(ColorGreen).
				Padding(0, 1)
	ToastErrorStyle = lipgloss.NewStyle().
			Foreground(ColorBlack).
			Background(ColorRed).
			Padding(0, 1)
	ToastInfoStyle = lipgloss.NewStyle().
			Foreground(ColorBlack).
			Background(ColorCyan).
			Padding(0, 1)

	// Column header in the list.
	ColumnHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Bold(true)

	// Divider line.
	DividerStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Generator option styles.
	GenLabelStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)
	GenValueStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)
	GenActiveStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)
	GenInactiveStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)
	GenCheckOnStyle = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Bold(true)
	GenCheckOffStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)
)
