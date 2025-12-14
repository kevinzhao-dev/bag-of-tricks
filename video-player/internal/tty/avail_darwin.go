//go:build darwin

package tty

import (
	"syscall"
	"unsafe"
)

// From sys/ioctl.h on macOS.
const ioctlFIONREAD = 0x4004667f

func bytesAvailable(fd uintptr) (int, error) {
	var n int
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlFIONREAD), uintptr(unsafe.Pointer(&n)))
	if errno != 0 {
		return 0, errno
	}
	return n, nil
}

