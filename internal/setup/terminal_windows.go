//go:build windows

package setup

import (
	"syscall"
	"unsafe"
)

func enableColor() {
	const enableVirtualTerminalProcessing = 0x0004
	kernel32 := syscall.MustLoadDLL("kernel32.dll")
	getConsoleMode := kernel32.MustFindProc("GetConsoleMode")
	setConsoleMode := kernel32.MustFindProc("SetConsoleMode")
	stdout := uintptr(syscall.Stdout)
	var mode uint32
	getConsoleMode.Call(stdout, uintptr(unsafe.Pointer(&mode)))
	setConsoleMode.Call(stdout, uintptr(mode|enableVirtualTerminalProcessing))
}
