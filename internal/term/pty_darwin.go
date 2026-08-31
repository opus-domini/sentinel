//go:build darwin

package term

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/opus-domini/sentinel/internal/userswitch"
)

const defaultUserSwitchMethod = userswitch.MethodSudo

func openPTY() (master *os.File, slave *os.File, outErr error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}
	defer func() {
		if outErr != nil {
			_ = master.Close()
		}
	}()

	if err := ptyGrant(master); err != nil {
		return nil, nil, err
	}
	if err := ptyUnlock(master); err != nil {
		return nil, nil, err
	}

	slaveName, err := ptyName(master)
	if err != nil {
		return nil, nil, err
	}

	slave, err = os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open slave pty %s: %w", slaveName, err)
	}
	return master, slave, nil
}

func ptyName(master *os.File) (string, error) {
	// Parameter length is encoded in the low 13 bits of the top word.
	const iocParmMask = 0x1FFF
	paramLen := (syscall.TIOCPTYGNAME >> 16) & iocParmMask
	if paramLen <= 0 {
		paramLen = 128
	}
	out := make([]byte, paramLen)

	if err := ioctl(master.Fd(), "TIOCPTYGNAME", uintptr(syscall.TIOCPTYGNAME), uintptr(unsafe.Pointer(&out[0]))); err != nil {
		return "", err
	}

	end := bytes.IndexByte(out, 0x00)
	if end < 0 {
		end = len(out)
	}
	name := string(out[:end])
	if strings.TrimSpace(name) == "" {
		return "", errors.New("empty pty slave path")
	}
	return name, nil
}

func ptyGrant(master *os.File) error {
	return ioctl(master.Fd(), "TIOCPTYGRANT", uintptr(syscall.TIOCPTYGRANT), 0)
}

func ptyUnlock(master *os.File) error {
	return ioctl(master.Fd(), "TIOCPTYUNLK", uintptr(syscall.TIOCPTYUNLK), 0)
}

func setWinsize(fd uintptr, cols, rows int) error {
	ws := struct {
		Rows uint16
		Cols uint16
		X    uint16
		Y    uint16
	}{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}
	return ioctl(fd, "TIOCSWINSZ", uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
}

func ioctl(fd uintptr, name string, cmd uintptr, ptr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, ptr)
	if errno != 0 {
		return fmt.Errorf("%s ioctl failed: %w", name, errno)
	}
	return nil
}
