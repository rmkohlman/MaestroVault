// Package touchid provides macOS TouchID (biometric) authentication using
// the LocalAuthentication framework via CGo.
//
// On non-macOS platforms, all functions return graceful "not available" errors.
package touchid
