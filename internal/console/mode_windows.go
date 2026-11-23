//go:build windows

package console

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modKernel32        = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle   = modKernel32.NewProc("GetStdHandle")
	procGetConsoleMode = modKernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = modKernel32.NewProc("SetConsoleMode")
)

// Win32 constants hand-declared from the Windows SDK (wincon.h) so the
// package needs no golang.org/x/sys dependency (zero-dep contract, the
// internal/env precedent). The two 0x0004-named flags live on different
// handles: echo on input, VT processing on output.
const (
	stdInputHandle = ^uintptr(9)  // (DWORD)-10, GetStdHandle(STD_INPUT_HANDLE)
	stdErrorHandle = ^uintptr(11) // (DWORD)-12, GetStdHandle(STD_ERROR_HANDLE)

	enableProcessedInput            = 0x0001
	enableLineInput                 = 0x0002
	enableEchoInput                 = 0x0004
	enableVirtualTerminalInput      = 0x0200
	enableVirtualTerminalProcessing = 0x0004
)

// IsTerminal reports whether fd is an interactive console handle: only a
// console accepts GetConsoleMode, pipes and files do not.
func IsTerminal(fd uintptr) bool {
	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode)))
	return ret != 0
}

// stdHandle resolves a standard handle by its GetStdHandle id. GetStdHandle
// reports failure through the return value (NULL or INVALID_HANDLE_VALUE),
// not the last error, so err from Call is unreliable here.
func stdHandle(id uintptr) (uintptr, error) {
	h, _, _ := procGetStdHandle.Call(id)
	if h == 0 || h == ^uintptr(0) {
		return 0, errors.New("console: no standard handle")
	}
	return h, nil
}

// consoleMode reads the current mode of a console handle.
func consoleMode(h uintptr, mode *uint32) bool {
	ret, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(mode)))
	return ret != 0
}

// setConsoleMode writes the mode of a console handle.
func setConsoleMode(h uintptr, mode uint32) bool {
	ret, _, _ := procSetConsoleMode.Call(h, uintptr(mode))
	return ret != 0
}

// notSupported wraps err into the ErrNotSupported fallback signal.
func notSupported(err error) error {
	return fmt.Errorf("%w: %v", ErrNotSupported, err)
}

// enterRawMode switches the process console for menu I/O and returns the
// restore function (design D3): stdin drops line/echo/processed input and
// gains VT input — arrows arrive as ESC sequences and Ctrl+C as 0x03 — and
// stderr gains VT processing so the ANSI repaint also works on classic
// conhost (Windows Terminal has it by default). Any failure returns an
// error wrapping ErrNotSupported with the console left as it was, so the
// caller can fall back to text prompts instead of crashing on exotic
// terminals.
func enterRawMode() (func(), error) {
	in, err := stdHandle(stdInputHandle)
	if err != nil {
		return nil, notSupported(err)
	}
	out, err := stdHandle(stdErrorHandle)
	if err != nil {
		return nil, notSupported(err)
	}

	var inMode, outMode uint32
	if !consoleMode(in, &inMode) || !consoleMode(out, &outMode) {
		return nil, notSupported(errors.New("console: stdin/stderr is not a console"))
	}
	newIn := inMode&^(enableLineInput|enableEchoInput|enableProcessedInput) | enableVirtualTerminalInput
	if !setConsoleMode(in, newIn) {
		return nil, notSupported(errors.New("console: SetConsoleMode failed (VT input unsupported)"))
	}
	if !setConsoleMode(out, outMode|enableVirtualTerminalProcessing) {
		_ = setConsoleMode(in, inMode) // undo the stdin switch before failing
		return nil, notSupported(errors.New("console: SetConsoleMode failed (VT processing unsupported)"))
	}

	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		_ = setConsoleMode(in, inMode)
		_ = setConsoleMode(out, outMode)
	}, nil
}
