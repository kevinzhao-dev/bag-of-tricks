package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"video-player/internal/mpv"
	"video-player/internal/pp"
	"video-player/internal/tty"
)

func main() {
	var (
		seekShort   = flag.Int("seek-short", 10, "short seek seconds")
		seekLong    = flag.Int("seek-long", 60, "long seek seconds")
		continuous  = flag.Bool("continuous", false, "auto-advance to next video on end")
		autoplay    = flag.Bool("autoplay", false, "auto-play on start (forces pause=false after load)")
		noResume    = flag.Bool("no-resume", false, "disable resume (even within this session)")
		persist     = flag.Bool("persist-resume", false, "persist resume timestamps across runs (writes to ~/.pp_timestamps_go.json)")
		mpvPathFlag = flag.String("mpv", "mpv", "mpv executable path")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pp (Go) - keyboard-first video player controller (mpv)\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags] [path]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Path may be a video file or a directory (default: .).\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nKeys:\n")
		fmt.Fprintf(os.Stderr, "  Space  play/pause\n  ←/→    seek ±short\n  ↑/↓    seek ±long\n  WASD   seek (same as arrows)\n  1-9    jump 10%%-90%%\n  j/k    prev/next video\n  q/e    prev/next video\n  m      mute\n  [/ ]   speed -/+ 0.1x\n  :      command mode\n  Esc    quit\n")
		fmt.Fprintf(os.Stderr, "\nCommand mode examples:\n")
		fmt.Fprintf(os.Stderr, "  :ls\n  :open 3\n  :open substring\n  :seek +30\n  :jump 50%%\n")
	}
	flag.Parse()

	path := "."
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	mpvPath, err := exec.LookPath(*mpvPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mpv not found (%s). Install mpv first.\n", *mpvPathFlag)
		os.Exit(1)
	}

	playlist, startIndex, err := pp.BuildPlaylist(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if len(playlist) == 0 {
		fmt.Fprintln(os.Stderr, "no video files found")
		os.Exit(1)
	}

	var ts *pp.TimestampStore
	if *persist {
		ts = pp.NewTimestampStore(pp.DefaultTimestampPath())
		if !*noResume {
			_ = ts.Load()
		}
	} else {
		ts = pp.NewTimestampStore("")
	}

	restoreTTY, err := tty.MakeRaw()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to set raw terminal mode: %v\n", err)
		os.Exit(1)
	}
	defer restoreTTY()

	socketPath, cleanupSock, err := mpv.TempSocketPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create socket path: %v\n", err)
		os.Exit(1)
	}
	defer cleanupSock()

	playlistPath, cleanupPlaylist, err := pp.WriteTempPlaylist(playlist)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write playlist: %v\n", err)
		os.Exit(1)
	}
	defer cleanupPlaylist()

	inputConfPath, cleanupInputConf, err := pp.WriteTempInputConf(pp.KeybindOptions{
		SeekShortS: float64(*seekShort),
		SeekLongS:  float64(*seekLong),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write input.conf: %v\n", err)
		os.Exit(1)
	}
	defer cleanupInputConf()

	browserScriptPath, cleanupBrowserScript, err := pp.WriteTempBrowserScript()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write browser script: %v\n", err)
		os.Exit(1)
	}
	defer cleanupBrowserScript()

	player, err := mpv.Start(mpvPath, mpv.StartOptions{
		SocketPath:    socketPath,
		PlaylistPath:  playlistPath,
		PlaylistStart: startIndex,
		InputConfPath: inputConfPath,
		ScriptPaths:   []string{browserScriptPath},
		KeepOpen:      true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start mpv: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = player.Quit(context.Background())
		_ = player.Wait()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := mpv.Dial(ctx, socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to mpv ipc: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	app := &pp.App{
		MPV:         client,
		Proc:        player,
		Playlist:    playlist,
		Index:       startIndex,
		SeekShortS:  float64(*seekShort),
		SeekLongS:   float64(*seekLong),
		Continuous:  *continuous,
		AutoPlay:    *autoplay,
		Timestamps:  ts,
		ResumeState: !*noResume,
	}

	_ = app.RestorePosition(context.Background())
	if *autoplay {
		_ = client.Command(context.Background(), "set_property", "pause", false)
	}
	app.ShowHelpOnce()

	if err := app.Run(); err != nil {
		enc := json.NewEncoder(os.Stderr)
		_ = enc.Encode(map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}
