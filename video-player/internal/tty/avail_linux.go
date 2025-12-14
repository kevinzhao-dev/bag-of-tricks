//go:build linux

package tty

import (
	"syscall"
	"unsafe"
)

// From asm-generic/ioctls.h on Linux.
const ioctlFIONREAD = 0x541b

func bytesAvailable(fd uintptr) (int, error) {
	var n int
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlFIONREAD), uintptr(unsafe.Pointer(&n)))
	if errno != 0 {
		return 0, errno
	}
	return n, nil
}

