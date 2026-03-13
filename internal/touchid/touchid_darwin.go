//go:build darwin

package touchid

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework LocalAuthentication -framework Foundation

#import <LocalAuthentication/LocalAuthentication.h>

// touchid_available returns 1 if biometric auth is available, 0 otherwise.
// If not available, errOut receives the error description (caller must free).
int touchid_available(char **errOut) {
    LAContext *ctx = [[LAContext alloc] init];
    NSError *error = nil;
    BOOL ok = [ctx canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&error];
    if (!ok && errOut != NULL && error != nil) {
        const char *desc = [[error localizedDescription] UTF8String];
        *errOut = strdup(desc);
    }
    return ok ? 1 : 0;
}

// touchid_authenticate prompts the user for biometric authentication.
// reason is the string shown in the TouchID dialog.
// Returns 1 on success, 0 on failure. errOut receives error description on failure.
int touchid_authenticate(const char *reason, char **errOut) {
    __block int result = 0;
    __block char *blockErr = NULL;

    LAContext *ctx = [[LAContext alloc] init];
    // Disable fallback to device passcode — we want biometrics only.
    ctx.localizedFallbackTitle = @"";

    dispatch_semaphore_t sema = dispatch_semaphore_create(0);

    NSString *nsReason = [NSString stringWithUTF8String:reason];

    [ctx evaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics
        localizedReason:nsReason
                  reply:^(BOOL success, NSError *error) {
        if (success) {
            result = 1;
        } else if (error != nil && errOut != NULL) {
            const char *desc = [[error localizedDescription] UTF8String];
            blockErr = strdup(desc);
        }
        dispatch_semaphore_signal(sema);
    }];

    dispatch_semaphore_wait(sema, DISPATCH_TIME_FOREVER);

    if (blockErr != NULL && errOut != NULL) {
        *errOut = blockErr;
    }

    return result;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Available reports whether TouchID (biometric authentication) is available
// on this device.
func Available() (bool, error) {
	var errStr *C.char
	ok := C.touchid_available(&errStr)
	if ok == 1 {
		return true, nil
	}
	if errStr != nil {
		defer C.free(unsafe.Pointer(errStr))
		return false, fmt.Errorf("%s", C.GoString(errStr))
	}
	return false, fmt.Errorf("TouchID not available")
}

// Authenticate prompts the user for TouchID biometric authentication.
// The reason string is displayed in the system TouchID dialog.
// Returns nil on successful authentication.
func Authenticate(reason string) error {
	cReason := C.CString(reason)
	defer C.free(unsafe.Pointer(cReason))

	var errStr *C.char
	ok := C.touchid_authenticate(cReason, &errStr)
	if ok == 1 {
		return nil
	}
	if errStr != nil {
		defer C.free(unsafe.Pointer(errStr))
		return fmt.Errorf("TouchID authentication failed: %s", C.GoString(errStr))
	}
	return fmt.Errorf("TouchID authentication failed")
}
