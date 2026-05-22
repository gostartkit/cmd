//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package terminal

import (
	"syscall"
	"unsafe"
)

type State struct {
	termios syscall.Termios
}

func IsTerminalFD(fd int) bool {
	_, err := getTermios(fd)
	return err == nil
}

func MakeRawFD(fd int) (*State, error) {
	termios, err := getTermios(fd)
	if err != nil {
		return nil, err
	}

	raw := *termios
	raw.Iflag &^= rawInputFlags
	raw.Oflag &^= rawOutputFlags
	raw.Cflag &^= rawControlMask
	raw.Cflag |= rawControlValue
	raw.Lflag &^= rawLocalFlags
	raw.Cc[rawVMin] = 1
	raw.Cc[rawVTime] = 0

	if err := setTermios(fd, &raw); err != nil {
		return nil, err
	}
	return &State{termios: *termios}, nil
}

func RestoreFD(fd int, state *State) error {
	if state == nil {
		return nil
	}
	return setTermios(fd, &state.termios)
}

func getTermios(fd int) (*syscall.Termios, error) {
	var termios syscall.Termios
	if err := ioctlTermios(fd, ioctlReadTermiosRequest, &termios); err != nil {
		return nil, err
	}
	return &termios, nil
}

func setTermios(fd int, termios *syscall.Termios) error {
	return ioctlTermios(fd, ioctlWriteTermiosRequest, termios)
}

func ioctlTermios(fd int, request uintptr, termios *syscall.Termios) error {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		request,
		uintptr(unsafe.Pointer(termios)),
		0,
		0,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
