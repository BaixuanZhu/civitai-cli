//go:build windows

package report

import (
	"os"
	"syscall"
	"unsafe"
)

// consoleScreenBufferInfo mirrors the Win32 CONSOLE_SCREEN_BUFFER_INFO
// layout: COORD Size, COORD CursorPosition, WORD Attributes,
// SMALL_RECT Window, COORD MaximumWindowSize.
type consoleScreenBufferInfo struct {
	Size              [2]int16
	CursorPosition    [2]int16
	Attributes        uint16
	Window            [4]int16 // Left, Top, Right, Bottom
	MaximumWindowSize [2]int16
}

// consoleWidth queries GetConsoleScreenBufferInfo on the stdout handle
// (kernel32 via syscall, zero third-party deps). Returns 0 on any
// failure so ConsoleWidth applies the 80-column fallback.
func consoleWidth() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetConsoleScreenBufferInfo")
	if err := proc.Find(); err != nil {
		return 0
	}
	var info consoleScreenBufferInfo
	ret, _, _ := proc.Call(
		uintptr(os.Stdout.Fd()),
		uintptr(unsafe.Pointer(&info)),
	)
	if ret == 0 {
		return 0
	}
	w := int(info.Window[2]-info.Window[0]) + 1
	if w <= 0 {
		return 0
	}
	return w
}
