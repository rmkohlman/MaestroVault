//go:build !darwin

package touchid

import "fmt"

// Available reports whether TouchID is available. Always returns false on
// non-macOS platforms.
func Available() (bool, error) {
	return false, fmt.Errorf("TouchID is only available on macOS")
}

// Authenticate always returns an error on non-macOS platforms.
func Authenticate(reason string) error {
	return fmt.Errorf("TouchID is only available on macOS")
}
