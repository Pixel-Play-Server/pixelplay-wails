//go:build !windows

package targeted

// repairSSHSupportKey is Windows-only; the manifest scopes this action with
// os:"windows", so non-Windows clients never reach here.
func repairSSHSupportKey(emit func(string)) error { return nil }
