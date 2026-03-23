package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// debugTUIModel is a minimal Bubble Tea model for diagnosing terminal size
// reporting. It displays environment info, a numbered test pattern that should
// exactly fill the reported terminal dimensions, and checks whether Bubble Tea's
// WindowSizeMsg matches a direct term.GetSize() call.
type debugTUIModel struct {
	width        int
	height       int
	sizeReceived bool

	// Direct term.GetSize() result (called once at startup for comparison).
	directWidth  int
	directHeight int
	directErr    error
}

func newDebugTUIModel() debugTUIModel {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	return debugTUIModel{
		directWidth:  w,
		directHeight: h,
		directErr:    err,
	}
}

func (m debugTUIModel) Init() tea.Cmd {
	return nil
}

func (m debugTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.sizeReceived = true
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m debugTUIModel) View() string {
	if !m.sizeReceived {
		return "Waiting for WindowSizeMsg..."
	}

	var b strings.Builder

	// ── Header: diagnostic info ──────────────────────────────
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "(unset)"
	}
	termProgram := os.Getenv("TERM_PROGRAM")
	if termProgram == "" {
		termProgram = "(unset)"
	}

	directSizeStr := fmt.Sprintf("%dx%d", m.directWidth, m.directHeight)
	if m.directErr != nil {
		directSizeStr = fmt.Sprintf("error: %v", m.directErr)
	}

	match := "YES"
	if m.directWidth != m.width || m.directHeight != m.height {
		match = "NO  <<<< MISMATCH"
	}

	header := []string{
		fmt.Sprintf("mav debug-tui | version: %s", version),
		fmt.Sprintf("WindowSizeMsg received: %v | Reported: %dx%d", m.sizeReceived, m.width, m.height),
		fmt.Sprintf("term.GetSize(stdout):   %s | Match: %s", directSizeStr, match),
		fmt.Sprintf("TERM=%s  TERM_PROGRAM=%s", termEnv, termProgram),
		"", // blank separator line
	}

	headerLines := len(header)
	footerLines := 1 // the "Press q to quit" line

	// Number of lines available for the test pattern.
	patternLines := m.height - headerLines - footerLines
	if patternLines < 1 {
		patternLines = 1
	}

	// ── Write header ─────────────────────────────────────────
	for _, line := range header {
		// Pad or truncate each header line to exactly m.width.
		b.WriteString(padToWidth(line, m.width))
		b.WriteByte('\n')
	}

	// ── Write test pattern ───────────────────────────────────
	for i := 1; i <= patternLines; i++ {
		prefix := fmt.Sprintf("Line %3d of %3d | Width: %3d | ", i, patternLines, m.width)
		// Fill the rest of the line with '=' characters to reach exactly m.width.
		remaining := m.width - len(prefix)
		var line string
		if remaining > 0 {
			line = prefix + strings.Repeat("=", remaining)
		} else {
			// Prefix alone exceeds width; truncate.
			line = prefix[:m.width]
		}
		b.WriteString(line)
		if i < patternLines {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')

	// ── Footer ───────────────────────────────────────────────
	footer := "Press q to quit. Can you see this line? What is the last visible line number?"
	b.WriteString(padToWidth(footer, m.width))

	return b.String()
}

// padToWidth pads s with spaces to exactly width characters, or truncates if longer.
func padToWidth(s string, width int) string {
	if len(s) >= width {
		if width <= 0 {
			return ""
		}
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func newDebugTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "debug-tui",
		Short: "Run terminal size diagnostic (for debugging TUI overflow issues)",
		Long: `Launches a minimal Bubble Tea program in alt-screen mode to diagnose
terminal size reporting. It displays:

  - The mav version and environment variables (TERM, TERM_PROGRAM)
  - Whether WindowSizeMsg was received and the reported dimensions
  - A direct term.GetSize() call for comparison
  - A numbered test pattern filling the entire terminal

If lines wrap or are cut off, the terminal size is being reported incorrectly.
If the pattern renders perfectly, the bug is in our rendering logic.`,
		Run: func(cmd *cobra.Command, args []string) {
			p := tea.NewProgram(newDebugTUIModel(), tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "debug-tui error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}
