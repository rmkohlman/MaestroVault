// Package touchid provides macOS TouchID (biometric) authentication using
// the LocalAuthentication framework via CGo.
//
// On non-macOS platforms, all functions return graceful "not available" errors.
//
// The Authenticator interface is the contract for biometric authentication.
// Two implementations are provided:
//
//   - DarwinAuthenticator (touchid_darwin.go) — real TouchID via CGo on macOS
//   - StubAuthenticator   (touchid_other.go)  — graceful stub on all other platforms
//
// Use New() to obtain the correct Authenticator for the current platform.
//
// Package-level Available() and Authenticate() functions are preserved
// for backward compatibility and convenience.
package touchid
