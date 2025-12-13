# pp (Go) - keyboard-first video player CLI

This repo contains `pp.py` (OpenCV-based) and a Go reimplementation focused on keyboard control while delegating playback to `mpv` via JSON IPC.

## Requirements

- `mpv` in `$PATH` (or pass `--mpv /path/to/mpv`)

## Build & run

```bash
go build -o pp ./cmd/pp
./pp .              # play videos in current directory
./pp path/to/a.mp4  # start at a specific file (playlist is its directory)
```

If your environment restricts Go's default cache location, run with a workspace cache:

```bash
env GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" go build -o pp ./cmd/pp
```

## Keyboard controls

- `Space`: play/pause
- `←/→`: seek `±--seek-short` seconds (default 10)
- `↑/↓`: seek `±--seek-long` seconds (default 60)
- `j` / `k` (or `Enter`): previous / next video
- `s` / `e`: start / end(-5s)
- `m`: mute
- `[` / `]`: speed `- / +` 0.1x (clamped to 0.1x–3.0x)
- `f`: fullscreen
- `b`: browse playlist (OSD)
- `Backspace/Delete`: move current file to Trash (press twice to confirm)
- `:`: command mode
- `q` / `Esc`: quit

## Command mode

Press `:` then type:

- `ls` / `list`: print playlist in terminal
- `open 3`: open playlist item (1-based)
- `open substring`: open first filename match
- `seek +30` / `seek -10`: relative seek
- `jump 50%`: jump to percent
- `jump 120`: jump to absolute seconds
- `next` / `prev` / `quit`

## Resume timestamps

By default, `pp` stores per-file playback positions in `~/.pp_timestamps_go.json`.

- Disable with `--no-resume`
