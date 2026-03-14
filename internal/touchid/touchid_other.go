//go:build !darwin

package touchid

import "fmt"

// StubAuthenticator implements Authenticator on non-macOS platforms.
// All methods return graceful "not available" errors.
type StubAuthenticator struct{}

// New returns an Authenticator for the current platform (non-macOS: stub).
func New() Authenticator { return &StubAuthenticator{} }

// Compile-time assertion.
var _ Authenticator = (*StubAuthenticator)(nil)

func (s *StubAuthenticator) Available() (bool, error) {
	return false, fmt.Errorf("TouchID is only available on macOS")
}

func (s *StubAuthenticator) Authenticate(reason string) error {
	return fmt.Errorf("TouchID is only available on macOS")
}

// ── Package-level functions (backward compatibility) ─────────

// Available reports whether TouchID is available. Always returns false on
// non-macOS platforms.
func Available() (bool, error) {
	return false, fmt.Errorf("TouchID is only available on macOS")
}

// Authenticate always returns an error on non-macOS platforms.
func Authenticate(reason string) error {
	return fmt.Errorf("TouchID is only available on macOS")
}
