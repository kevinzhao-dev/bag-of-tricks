package mpv

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type Process struct {
	cmd *exec.Cmd
}

type StartOptions struct {
	SocketPath    string
	PlaylistPath  string
	PlaylistStart int
	InputConfPath string
	KeepOpen      bool
}

func Start(mpvPath string, opts StartOptions) (*Process, error) {
	args := []string{
		"--no-terminal",
		"--input-ipc-server=" + opts.SocketPath,
		"--input-default-bindings=no",
		"--input-terminal=no",
	}

	if opts.KeepOpen {
		args = append(args, "--keep-open=yes")
	} else {
		args = append(args, "--keep-open=no")
	}

	if opts.InputConfPath != "" {
		args = append(args, "--input-conf="+opts.InputConfPath)
	}

	if opts.PlaylistPath != "" {
		args = append(args, "--playlist="+opts.PlaylistPath, "--playlist-start="+strconv.Itoa(opts.PlaylistStart))
	}

	cmd := exec.Command(mpvPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Process{cmd: cmd}, nil
}

func (p *Process) Quit(ctx context.Context) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	// Best-effort: send SIGTERM, allow mpv to exit.
	_ = p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(150 * time.Millisecond):
		return nil
	}
}

func (p *Process) Wait() error {
	if p == nil || p.cmd == nil {
		return nil
	}
	err := p.cmd.Wait()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}
