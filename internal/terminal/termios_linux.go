//go:build linux

package terminal

import "syscall"

const (
	ioctlReadTermiosRequest  = syscall.TCGETS
	ioctlWriteTermiosRequest = syscall.TCSETS

	rawInputFlags   = syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	rawOutputFlags  = syscall.OPOST
	rawControlMask  = syscall.CSIZE | syscall.PARENB
	rawControlValue = syscall.CS8
	rawLocalFlags   = syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	rawVMin         = syscall.VMIN
	rawVTime        = syscall.VTIME
)
