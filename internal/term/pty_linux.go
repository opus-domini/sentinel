//go:build linux

package term

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/opus-domini/sentinel/internal/userswitch"
)

const defaultUserSwitchMethod = userswitch.MethodSystemdRun

func openPTY() (master *os.File, slave *os.File, outErr error) {
	masterFD, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	closeMaster := true
	defer func() {
		if closeMaster {
			_ = syscall.Close(masterFD)
		}
	}()

	var unlock int32
	if err := ioctl(masterFD, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		return nil, nil, fmt.Errorf("unlock ptmx: %w", err)
	}

	var ptyNum uint32
	if err := ioctl(masterFD, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyNum))); err != nil {
		return nil, nil, fmt.Errorf("read pty number: %w", err)
	}

	slaveName := fmt.Sprintf("/dev/pts/%d", ptyNum)
	slaveFD, err := syscall.Open(slaveName, syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open slave pty %s: %w", slaveName, err)
	}

	closeMaster = false
	master = os.NewFile(uintptr(masterFD), "/dev/ptmx")
	slave = os.NewFile(uintptr(slaveFD), slaveName)
	return master, slave, nil
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
	return ioctl(int(fd), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
}

func ioctl(fd int, cmd uintptr, ptr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), cmd, ptr)
	if errno != 0 {
		return errno
	}
	return nil
}
