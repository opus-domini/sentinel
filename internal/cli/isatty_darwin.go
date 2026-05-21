//go:build darwin

package cli

import (
	"syscall"
	"unsafe"
)

func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGETA, uintptr(unsafe.Pointer(&t))) //nolint:gosec // ioctl requires unsafe.Pointer
	return err == 0
}
