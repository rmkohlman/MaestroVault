# TouchID Integration

MaestroVault supports macOS TouchID as an optional biometric authentication gate. When enabled, you must authenticate with your fingerprint (or Apple Watch) each time the vault is opened.

## How It Works

TouchID integration uses the macOS `LocalAuthentication` framework via CGo. When you run any command that accesses the vault, the system TouchID dialog appears before the vault is unlocked.

```
Command invoked
    |
    v
Load config (~/.maestrovault/config.json)
    |
    +-- TouchID enabled?
    |       |
    |       +-- YES: Prompt for biometric auth
    |       |       |
    |       |       +-- Success: Continue to open vault
    |       |       +-- Failure: Abort with error
    |       |
    |       +-- NO: Continue without prompt
    |
    +-- vault.Open()
    |       |
    |       +-- Open database
    |
    v
Ready
```

## Enabling TouchID

```bash
mav touchid enable
```

This command:

1. Checks that TouchID hardware is available
2. Performs a test authentication to verify it works
3. Saves `{"touchid": true}` to `~/.maestrovault/config.json`

## Disabling TouchID

```bash
mav touchid disable
```

Disabling requires TouchID authentication first -- this prevents an unauthorized user from silently turning off biometric protection.

## Checking Status

```bash
mav touchid status
```

Shows whether TouchID is enabled in config and whether the hardware is available:

```
  TouchID:  enabled
  Hardware: available
```

JSON output:

```bash
mav touchid status -o json
```

```json
{
  "enabled": true,
  "available": true
}
```

## Configuration

TouchID settings are stored in `~/.maestrovault/config.json`:

```json
{
  "touchid": true,
  "vim_mode": false,
  "fuzzy_search": false
}
```

The `touchid` field controls biometric authentication. The other fields (`vim_mode`, `fuzzy_search`) control TUI behavior. The file is created with `0600` permissions. If the file is missing, all settings default to disabled.

## Requirements

- macOS with Touch ID sensor (MacBook Pro/Air 2016+, some iMacs) or
- Apple Watch paired for Mac unlock
- The vault must be initialized (`mav init`)

## Security Notes

- Uses `LAPolicyDeviceOwnerAuthenticationWithBiometrics` -- biometrics only, no passcode fallback
- Authentication is session-based: one prompt per vault open (not per operation)
- The TouchID prompt reads: "MaestroVault wants to access your secrets"
- On non-macOS platforms, TouchID functions return graceful "not available" errors

## Troubleshooting

**"TouchID is not available on this device"**

- Verify your Mac has a Touch ID sensor or paired Apple Watch
- Check System Settings > Touch ID & Password
- Ensure at least one fingerprint is enrolled

**"TouchID authentication failed"**

- Try again -- the system allows multiple attempts
- If you cancel the dialog, the command will fail with an auth error
- Check that your fingerprint is registered correctly

**TouchID enabled but hardware not available**

This can happen if you enabled TouchID on a Mac with the sensor and then moved the vault directory to a Mac without one. Run `mav touchid disable` -- since the hardware is not available, the disable command skips the biometric check.
