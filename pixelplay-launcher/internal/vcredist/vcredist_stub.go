//go:build !windows

package vcredist

import "fmt"

type Progress func(string)

var ErrNeedSetup = fmt.Errorf("NEED_VCREDIST_SETUP")

const PopupMessage = ""
const AlreadyInstalledHint = ""
const RedistURL = "https://aka.ms/vs/17/release/vc_redist.x64.exe"

func Present() bool { return true }

func Acknowledged() bool { return true }

func MarkAcknowledged() {}

func IsDLLMissingExit(code int) bool { return false }

func CachedPath() string { return "" }

func Download(emit Progress) (string, error) { return "", nil }

func LaunchSetup(emit Progress) (string, error) { return "", nil }

func Ensure(nativesDir string, emit Progress) error { return nil }

func RepairAfterCrash(nativesDir string, emit Progress) error { return nil }

func StageInto(nativesDir string) error { return nil }

func ExplainExit(code int) string { return "" }
