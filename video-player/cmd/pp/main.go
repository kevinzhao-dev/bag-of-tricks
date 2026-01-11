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
		seekFine    = flag.Int("seek-fine", 1, "fine seek seconds (left/right)")
		seekShort   = flag.Int("seek-short", 10, "short seek seconds (A/D)")
		seekLong    = flag.Int("seek-long", 60, "long seek seconds (up/down, W/S)")
		continuous  = flag.Bool("continuous", false, "auto-advance to next video on end")
		autoplay    = flag.Bool("autoplay", true, "auto-play on start (default true; forces pause=false after load)")
		noAutoplay  = flag.Bool("no-autoplay", false, "disable autoplay on start")
		startMuted  = flag.Bool("mute", false, "start muted")
		noResume    = flag.Bool("no-resume", false, "disable resume (even within this session)")
		persist     = flag.Bool("persist-resume", false, "persist resume timestamps across runs (writes to ~/.pp_timestamps_go.json)")
		mpvPathFlag = flag.String("mpv", "mpv", "mpv executable path")
		latest      = flag.Bool("latest", false, "order video list by date added (most recent first)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pp (Go) - keyboard-first video player controller (mpv)\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags] [path]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Path may be a video file or a directory (default: .).\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nKeys:\n")
		fmt.Fprintf(os.Stderr, "  Space  play/pause\n  ←/→    seek ±fine\n  Z/C    seek ±fine (same as arrows)\n  ↑/↓    seek ±long\n  WASD   seek (A/D=short, W/S=long)\n  J/K    seek (long, same as ↑/↓)\n  1-9    jump 10%%-90%%\n  q/e    prev/next video\n  h/l    prev/next video\n  x      snapshot (./snapshots)\n  g      clip toggle (./clips)\n  t      trim toggle (./clips)\n  +/-    window scale\n  m      mute\n  [/ ]   speed -/+ 0.1x\n  :      command mode\n  Esc    quit\n")
		fmt.Fprintf(os.Stderr, "\nCommand mode examples:\n")
		fmt.Fprintf(os.Stderr, "  :ls\n  :open 3\n  :open substring\n  :seek +30\n  :jump 50%%\n")
	}
	flag.Parse()

	autoPlayEffective := *autoplay && !*noAutoplay

	path := "."
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	mpvPath, err := exec.LookPath(*mpvPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mpv not found (%s). Install mpv first.\n", *mpvPathFlag)
		os.Exit(1)
	}

	playlist, startIndex, err := pp.BuildPlaylist(path, *latest)
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
		SeekFineS:  float64(*seekFine),
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

	_ = client.Command(context.Background(), "set_property", "mute", *startMuted)

	app := &pp.App{
		MPV:         client,
		Proc:        player,
		Playlist:    playlist,
		Index:       startIndex,
		SeekShortS:  float64(*seekShort),
		SeekFineS:   float64(*seekFine),
		SeekLongS:   float64(*seekLong),
		Continuous:  *continuous,
		AutoPlay:    autoPlayEffective,
		Timestamps:  ts,
		ResumeState: !*noResume,
	}

	_ = app.RestorePosition(context.Background())
	if autoPlayEffective {
		_ = client.Command(context.Background(), "set_property", "pause", false)
	}
	app.ShowHelpOnce()

	if err := app.Run(); err != nil {
		enc := json.NewEncoder(os.Stderr)
		_ = enc.Encode(map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}
