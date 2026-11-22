// Package env persists civ's own directory into the user PATH, written to the
// registry at
//
//	HKCU\Environment
//
// It does not shell out to cmd.exe or setx (setx truncates long PATH values,
// a classic pitfall). After writing it broadcasts WM_SETTINGCHANGE so newly
// started processes see the change. This lets users run the bare command name
// "civ" instead of an absolute path.
//
// Windows-only: the file name suffix gates compilation to GOOS=windows. All
// registry/broadcast calls are hand-written syscall (no golang.org/x/sys), so
// the package keeps the project's zero-third-party-dependency contract.
package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	modAdvapi32 = syscall.NewLazyDLL("advapi32.dll")
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegOpenKeyEx           = modAdvapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueEx        = modAdvapi32.NewProc("RegQueryValueExW")
	procRegSetValueEx          = modAdvapi32.NewProc("RegSetValueExW")
	procRegCloseKey            = modAdvapi32.NewProc("RegCloseKey")
	procSendMessageW           = modUser32.NewProc("SendMessageTimeoutW")
	procSetEnvironmentVariable = modKernel32.NewProc("SetEnvironmentVariableW")
)

// Win32 constants hand-declared from the Windows SDK (winreg.h / winuser.h)
// so the package needs no golang.org/x/sys dependency (zero-dep contract).
const (
	// HKEY_CURRENT_USER is the predefined handle to the per-user HKCU hive.
	HKEY_CURRENT_USER = 0x80000001
	// KEY_READ grants read access (query values, enumerate subkeys, notify).
	KEY_READ = 0x20019
	// KEY_ALL_ACCESS grants full access (read, write, create, delete).
	KEY_ALL_ACCESS = 0xF003F
	// REG_EXPAND_SZ is a null-terminated string with %VAR% expansion (the PATH type).
	REG_EXPAND_SZ = 2

	// HWND_BROADCAST targets every top-level window (SendMessageTimeoutW recipient).
	HWND_BROADCAST = 0xFFFF
	// WM_SETTINGCHANGE notifies apps that a system setting (Environment) changed.
	WM_SETTINGCHANGE = 0x001A
	// SMTO_ABORTIFHUNG returns immediately when a receiving window is hung.
	SMTO_ABORTIFHUNG = 0x0002
)

// Persist writes a value to HKCU\Environment (REG_EXPAND_SZ, supports variable
// expansion). It also sets the current process's env var and broadcasts
// WM_SETTINGCHANGE to notify other processes.
func Persist(name, value string) error {
	var hKey syscall.Handle
	envKey, _ := syscall.UTF16PtrFromString("Environment")
	r1, _, e := procRegOpenKeyEx.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(envKey)),
		0,
		uintptr(KEY_ALL_ACCESS),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r1 != 0 {
		return fmt.Errorf("failed to open registry key: %w", os.NewSyscallError("RegOpenKeyEx", e))
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	// UTF16FromString appends the trailing \0; size in UTF-16 units (not UTF-8 bytes)
	valueUTF16, err := syscall.UTF16FromString(value)
	if err != nil {
		return fmt.Errorf("value contains invalid characters: %w", err)
	}
	nameUTF16, _ := syscall.UTF16PtrFromString(name)

	r1, _, e = procRegSetValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(nameUTF16)),
		0,
		uintptr(REG_EXPAND_SZ),
		uintptr(unsafe.Pointer(&valueUTF16[0])),
		uintptr(len(valueUTF16)*2), // byte size including the trailing \0
	)
	if r1 != 0 {
		return fmt.Errorf("failed to write registry value: %w", os.NewSyscallError("RegSetValueEx", e))
	}

	setCurrentProcessEnv(name, value)
	broadcastSettingChange()
	return nil
}

// setCurrentProcessEnv sets the current process's environment variable (UTF-16 interface).
func setCurrentProcessEnv(name, value string) {
	nameUTF16, _ := syscall.UTF16PtrFromString(name)
	valueUTF16, _ := syscall.UTF16PtrFromString(value)
	procSetEnvironmentVariable.Call(
		uintptr(unsafe.Pointer(nameUTF16)),
		uintptr(unsafe.Pointer(valueUTF16)),
	)
}

// broadcastSettingChange broadcasts WM_SETTINGCHANGE so Explorer etc. refresh the environment.
func broadcastSettingChange() {
	envUTF16, _ := syscall.UTF16PtrFromString("Environment")
	procSendMessageW.Call(
		uintptr(HWND_BROADCAST),
		uintptr(WM_SETTINGCHANGE),
		0,
		uintptr(unsafe.Pointer(envUTF16)),
		SMTO_ABORTIFHUNG,
		5000,
		0,
	)
}

// readUserEnv reads a value from HKCU\Environment.
func readUserEnv(name string) (string, error) {
	var hKey syscall.Handle
	envKey, _ := syscall.UTF16PtrFromString("Environment")
	r1, _, e := procRegOpenKeyEx.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(envKey)),
		0,
		uintptr(KEY_READ),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r1 != 0 {
		return "", os.NewSyscallError("RegOpenKeyEx", e)
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	nameUTF16, _ := syscall.UTF16PtrFromString(name)
	buf := make([]uint16, 32768)
	// RegQueryValueExW's lpcbData is a BYTE count, not a UTF16 element count.
	// Passing len(buf) would under-report capacity by half and make long PATH
	// values fail with ERROR_MORE_DATA. 32768 uint16 = 65536 bytes covers the
	// Windows maximum PATH length (32767 characters).
	var bufLen uint32 = uint32(len(buf) * 2)
	var valueType uint32
	r1, _, e = procRegQueryValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(nameUTF16)),
		0,
		uintptr(unsafe.Pointer(&valueType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufLen)),
	)
	if r1 != 0 {
		return "", os.NewSyscallError("RegQueryValueEx", e)
	}
	return syscall.UTF16ToString(buf), nil
}

// splitPathEntries splits a PATH string, ignoring empty entries.
// Pure function, for table-driven tests.
func splitPathEntries(p string) []string {
	parts := strings.Split(p, ";")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// containsEntry reports whether the PATH string already contains dir
// (case-insensitive, matched after path normalization, tolerating trailing
// separator differences). Pure function, for table-driven tests.
func containsEntry(userPath, dir string) bool {
	want := strings.ToLower(filepath.Clean(dir))
	for _, e := range splitPathEntries(userPath) {
		if strings.ToLower(filepath.Clean(e)) == want {
			return true
		}
	}
	return false
}

// removePathEntry removes every entry equal to dir (case-insensitive, matched
// after path normalization, tolerating trailing separator differences) from a
// PATH string, preserving the order of the remaining entries. Empty entries are
// dropped by splitPathEntries. Returns the rebuilt PATH and whether any entry
// was removed. Pure function, for table-driven tests.
func removePathEntry(userPath, dir string) (string, bool) {
	want := strings.ToLower(filepath.Clean(dir))
	entries := splitPathEntries(userPath)
	kept := make([]string, 0, len(entries))
	removed := false
	for _, e := range entries {
		if strings.ToLower(filepath.Clean(e)) == want {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	return strings.Join(kept, ";"), removed
}

// EnsureUserPath ensures the directory containing civ.exe is in the user PATH.
// Called on every startup (gated by app.BootstrapEnabled): resolves the exe
// directory via os.Executable(), and appends + persists it if missing. Injected
// automatically on first run; adapts if the user moves the exe later.
//
// Returns an error on failure so the caller can log a [WARN] and continue —
// bootstrap must never be fatal to the main flow. An already-present directory
// is a no-op returning nil (idempotent).
func EnsureUserPath() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate the current exe: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// A genuinely absent PATH value (ERROR_FILE_NOT_FOUND) is safe to create
	// from empty. Any other read failure must abort bootstrap rather than
	// risk overwriting the user's PATH with a partial/empty value.
	userPath, err := readUserEnv("PATH")
	if err != nil {
		if !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("failed to read user PATH: %w", err)
		}
		userPath = ""
	}
	if containsEntry(userPath, exeDir) {
		return nil // already in PATH
	}

	// Append at the end (the tool itself stays low-priority, never displaces existing entries)
	newPath := exeDir
	if strings.TrimSpace(userPath) != "" {
		newPath = userPath + ";" + exeDir
	}
	return Persist("PATH", newPath)
}

// RemoveUserPath removes the directory containing civ.exe from the user PATH.
// It is the inverse of EnsureUserPath, invoked by the uninstaller (through the
// internal "unbootstrap" command) before the exe is deleted so uninstall leaves
// no dead PATH entry. Idempotent: an absent entry is a no-op. Like EnsureUserPath
// it returns an error rather than exiting, so the caller can warn and continue.
func RemoveUserPath() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate the current exe: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	userPath, err := readUserEnv("PATH")
	if err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return nil // no PATH value at all; nothing to remove
		}
		return fmt.Errorf("failed to read user PATH: %w", err)
	}
	newPath, changed := removePathEntry(userPath, exeDir)
	if !changed {
		return nil // already absent
	}
	return Persist("PATH", newPath)
}
