//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package terminal

import "errors"

var errUnsupported = errors.New("terminal raw mode unsupported on this platform")

type State struct{}

func IsTerminalFD(fd int) bool {
	return false
}

func MakeRawFD(fd int) (*State, error) {
	return nil, errUnsupported
}

func RestoreFD(fd int, state *State) error {
	return nil
}
