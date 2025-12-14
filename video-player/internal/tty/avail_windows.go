//go:build windows

package tty

func bytesAvailable(fd uintptr) (int, error) {
	return 0, nil
}

