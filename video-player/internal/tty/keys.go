package tty

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type KeyKind int

const (
	KeyUnknown KeyKind = iota
	KeyRune
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeySpace
	KeyQuit
)

type Key struct {
	Kind KeyKind
	Rune rune
}

func MakeRaw() (restore func(), err error) {
	// Avoid extra dependencies: use stty to toggle raw mode.
	cmdGet := exec.Command("stty", "-g")
	cmdGet.Stdin = os.Stdin
	out, err := cmdGet.Output()
	if err != nil {
		return nil, fmt.Errorf("stty -g: %w", err)
	}
	prev := strings.TrimSpace(string(out))

	// "stty raw" disables output post-processing (-opost), which makes '\n' not return
	// carriage and leads to "diagonal" output. We only need non-canonical, no-echo input.
	cmdRaw := exec.Command("stty", "-echo", "-icanon", "min", "1", "time", "0")
	cmdRaw.Stdin = os.Stdin
	if err := cmdRaw.Run(); err != nil {
		return nil, fmt.Errorf("stty -echo -icanon: %w", err)
	}

	return func() {
		cmd := exec.Command("stty", prev)
		cmd.Stdin = os.Stdin
		_ = cmd.Run()
	}, nil
}

func ReadKey(r *bufio.Reader) (Key, error) {
	b, err := r.ReadByte()
	if err != nil {
		return Key{}, err
	}

	switch b {
	case 0x1b: // ESC or escape sequence
		// Distinguish bare ESC from escape sequences (e.g. arrow keys).
		// bufio.Reader blocks on ReadByte, so only read more when input is available.
		if !hasMoreInput(r) {
			return Key{Kind: KeyQuit}, nil
		}
		next, err := r.ReadByte()
		if err != nil {
			return Key{Kind: KeyQuit}, nil
		}
		if next == '[' {
			third, err := r.ReadByte()
			if err != nil {
				return Key{Kind: KeyUnknown}, nil
			}
			switch third {
			case 'A':
				return Key{Kind: KeyUp}, nil
			case 'B':
				return Key{Kind: KeyDown}, nil
			case 'C':
				return Key{Kind: KeyRight}, nil
			case 'D':
				return Key{Kind: KeyLeft}, nil
			default:
				return Key{Kind: KeyUnknown}, nil
			}
		}
		if next == 'O' {
			third, err := r.ReadByte()
			if err != nil {
				return Key{Kind: KeyUnknown}, nil
			}
			switch third {
			case 'A':
				return Key{Kind: KeyUp}, nil
			case 'B':
				return Key{Kind: KeyDown}, nil
			case 'C':
				return Key{Kind: KeyRight}, nil
			case 'D':
				return Key{Kind: KeyLeft}, nil
			default:
				return Key{Kind: KeyUnknown}, nil
			}
		}
		return Key{Kind: KeyQuit}, nil
	case ' ':
		return Key{Kind: KeySpace}, nil
	default:
		if b == 0x03 { // Ctrl-C
			return Key{Kind: KeyQuit}, nil
		}
		if b == '\r' || b == '\n' {
			return Key{Kind: KeyRune, Rune: '\n'}, nil
		}
		if b < 0x20 {
			return Key{Kind: KeyUnknown}, nil
		}
		return Key{Kind: KeyRune, Rune: rune(b)}, nil
	}
}

func hasMoreInput(r *bufio.Reader) bool {
	if r.Buffered() > 0 {
		return true
	}

	// We only use ReadKey with stdin; check whether there are bytes ready so we don't
	// block forever after a lone ESC press.
	for i := 0; i < 3; i++ {
		n, err := bytesAvailable(os.Stdin.Fd())
		if err == nil && n > 0 {
			return true
		}
		time.Sleep(4 * time.Millisecond)
		if r.Buffered() > 0 {
			return true
		}
	}
	return false
}

func InputReady(r *bufio.Reader) bool {
	if r.Buffered() > 0 {
		return true
	}
	n, err := bytesAvailable(os.Stdin.Fd())
	return err == nil && n > 0
}

func ReadLine(r *bufio.Reader, prompt string) (line string, ok bool, err error) {
	_, _ = os.Stdout.WriteString(prompt)
	buf := make([]byte, 0, 64)
	for {
		k, err := ReadKey(r)
		if err != nil {
			return "", false, err
		}
		if k.Kind == KeyQuit {
			_, _ = os.Stdout.WriteString("\n")
			return "", false, nil
		}
		if k.Kind == KeyRune && (k.Rune == '\n' || k.Rune == '\r') {
			_, _ = os.Stdout.WriteString("\n")
			return string(buf), true, nil
		}
		if k.Kind != KeyRune {
			continue
		}
		switch k.Rune {
		case 0x7f, 0x08: // backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				_, _ = os.Stdout.WriteString("\b \b")
			}
		default:
			if k.Rune < 0x20 {
				continue
			}
			if len(buf) > 4096 {
				return "", false, errors.New("command too long")
			}
			buf = append(buf, byte(k.Rune))
			_, _ = os.Stdout.WriteString(string(k.Rune))
		}
	}
}
