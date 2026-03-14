// Package clipboard provides system clipboard operations for macOS.
// It copies values to the clipboard and supports auto-clearing after
// a configurable timeout for security.
package clipboard

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// clearMu and clearTimer form a global singleton for clipboard auto-clear
// scheduling. This is intentional: MaestroVault is a single-user tool, so
// a last-copy-wins model is appropriate. If multiple copies happen before the
// timer fires, only the latest timer matters (the previous one is cancelled).
var (
	clearMu    sync.Mutex
	clearTimer *time.Timer
)

// Copy copies the given value to the system clipboard using pbcopy.
func Copy(value string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copying to clipboard: %w", err)
	}
	return nil
}

// CopyWithClear copies a value to the clipboard and schedules automatic
// clearing after the specified duration. If duration is 0, no auto-clear
// is scheduled. Returns a cancel function to prevent the clear.
func CopyWithClear(value string, after time.Duration) (cancel func(), err error) {
	if err := Copy(value); err != nil {
		return nil, err
	}

	if after <= 0 {
		return func() {}, nil
	}

	clearMu.Lock()
	// Cancel any existing clear timer.
	if clearTimer != nil {
		clearTimer.Stop()
	}
	clearTimer = time.AfterFunc(after, func() {
		_ = Clear()
	})
	t := clearTimer
	clearMu.Unlock()

	return func() { t.Stop() }, nil
}

// Clear empties the system clipboard.
func Clear() error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clearing clipboard: %w", err)
	}
	return nil
}
