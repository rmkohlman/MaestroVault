// Package touchid provides macOS TouchID (biometric) authentication using
// the LocalAuthentication framework via CGo.
//
// On non-macOS platforms, all functions return graceful "not available" errors.
//
// The Authenticator interface is the contract for biometric authentication.
// DarwinAuthenticator (touchid_darwin.go) and StubAuthenticator
// (touchid_other.go) are the platform-specific implementations.
//
// Package-level Available() and Authenticate() functions are preserved
// for backward compatibility and convenience.
package touchid

// ── Interface ────────────────────────────────────────────────

// Authenticator defines the contract for biometric authentication.
// Callers should invoke Authenticate() before opening the vault
// to gate access behind a biometric check.
type Authenticator interface {
	// Available reports whether biometric authentication is available
	// on this device.
	Available() (bool, error)

	// Authenticate prompts the user for biometric authentication.
	// The reason string is displayed in the system dialog.
	// Returns nil on successful authentication.
	Authenticate(reason string) error
}
