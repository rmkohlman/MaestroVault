package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// DebugLogger writes diagnostic data to /tmp/mav-debug.log.
// It is safe for concurrent use but in practice only called from the
// single-threaded Bubble Tea event loop (View/Update).
type DebugLogger struct {
	mu   sync.Mutex
	file *os.File
	seq  int // monotonic call counter
}

// NewDebugLogger creates a new debug logger that writes to /tmp/mav-debug.log.
// If the file cannot be opened, logging silently becomes a no-op.
func NewDebugLogger() *DebugLogger {
	f, err := os.OpenFile("/tmp/mav-debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return &DebugLogger{} // no-op logger
	}
	fmt.Fprintf(f, "=== MaestroVault TUI Debug Log — %s ===\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "PID: %d\n\n", os.Getpid())
	return &DebugLogger{file: f}
}

// Log writes a formatted log line with a timestamp and sequence number.
func (d *DebugLogger) Log(format string, args ...any) {
	if d == nil || d.file == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(d.file, "[%s #%04d] %s\n", ts, d.seq, msg)
}

// LogView records a View() call with key diagnostic metrics.
func (d *DebugLogger) LogView(screen string, width, height int, sizeReceived bool, outputLines int, extraKV ...string) {
	if d == nil || d.file == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	ts := time.Now().Format("15:04:05.000")

	var extra string
	if len(extraKV) >= 2 {
		parts := make([]string, 0, len(extraKV)/2)
		for i := 0; i+1 < len(extraKV); i += 2 {
			parts = append(parts, extraKV[i]+"="+extraKV[i+1])
		}
		extra = " " + strings.Join(parts, " ")
	}

	fmt.Fprintf(d.file, "[%s #%04d] View() screen=%s w=%d h=%d sizeRecv=%v outputLines=%d%s\n",
		ts, d.seq, screen, width, height, sizeReceived, outputLines, extra)
}

// LogWindowSize records when a WindowSizeMsg is received.
func (d *DebugLogger) LogWindowSize(width, height int, firstTime bool) {
	if d == nil || d.file == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(d.file, "[%s #%04d] WindowSizeMsg w=%d h=%d first=%v\n",
		ts, d.seq, width, height, firstTime)
}

// Close closes the log file.
func (d *DebugLogger) Close() {
	if d == nil || d.file == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.file.Close()
}

// ── Debug status bar for View() overlay ──────────────────────

// debugStatusBar returns a one-line debug status string showing current
// dimensions, sizeReceived flag, screen name, and output line count.
func (m Model) debugStatusBar(outputLines int) string {
	screenName := "list"
	switch m.screen {
	case screenDetail:
		screenName = "detail"
	case screenConfirmDelete:
		screenName = "confirm"
	}
	if m.showSecretModal {
		screenName = "modal"
	} else if m.showHelp {
		screenName = "help"
	} else if m.showGenerator {
		screenName = "generator"
	} else if m.showInfo {
		screenName = "info"
	} else if m.showSettings {
		screenName = "settings"
	}

	sizeFlag := "NO"
	if m.sizeReceived {
		sizeFlag = "YES"
	}

	return DebugBarStyle.Render(fmt.Sprintf(
		" DEBUG  w=%d h=%d sizeRecv=%s screen=%s lines=%d ",
		m.width, m.height, sizeFlag, screenName, outputLines,
	))
}

// DebugBarStyle is the lipgloss style for the debug status bar.
var DebugBarStyle = ModalStyle.
	Border(noBorder).
	Padding(0, 0).
	Background(ColorYellow).
	Foreground(ColorBlack).
	Bold(true)

// noBorder is a no-op border for the debug bar.
var noBorder = lipgloss.Border{}
