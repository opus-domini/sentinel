//go:build linux

package cli

import (
	"syscall"
	"unsafe"
)

func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&t)))
	return err == 0
}
