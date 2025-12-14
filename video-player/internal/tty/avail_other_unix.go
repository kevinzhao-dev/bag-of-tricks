//go:build !windows && !darwin && !linux

package tty

func bytesAvailable(fd uintptr) (int, error) {
	return 0, nil
}

