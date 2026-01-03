# pp (Go) - keyboard-first video player CLI

This repo contains `pp.py` (OpenCV-based) and a Go reimplementation focused on keyboard control while delegating playback to `mpv` via JSON IPC.

## Requirements

- `mpv` in `$PATH` (or pass `--mpv /path/to/mpv`)

## Build & run (Go)

```bash
make build
./bin/pp .              # play videos in current directory
./bin/pp path/to/a.mp4  # start at a specific file (playlist is its directory)
```

Autoplay is enabled by default. Disable it with:

```bash
./bin/pp --no-autoplay .
```

Start muted:

```bash
./bin/pp --mute .
```

If your environment restricts Go's default cache location, either use the `Makefile` (it defaults to a workspace cache) or run:

```bash
env GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" go build -o pp ./cmd/pp
```

## Install as a CLI (macOS)

Install to `~/.local/bin` (no sudo):

```bash
make install
```

Make sure `~/.local/bin` is on your `PATH` (zsh):

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Now run:

```bash
pp .
```

Install system-wide to `/usr/local/bin` (may require sudo):

```bash
sudo make install PREFIX=/usr/local
```

Uninstall:

```bash
make uninstall
```

## Keyboard controls

- `Space`: play/pause
- `←/→`: seek `±--seek-fine` seconds (default 1)
- `Z/C`: seek `±--seek-fine` seconds (same as arrows)
- `↑/↓`: seek `±--seek-long` seconds (default 60)
- `W/A/S/D`: seek (`A/D` = `--seek-short`, `W/S` = `--seek-long`)
- `J/K`: seek (`--seek-long`, same as arrows)
- `1`–`9`: jump to `10%`–`90%`
- `q` / `e`: previous / next video
- `h` / `l`: previous / next video
- `Enter`: next video
- `x`: save snapshot to `./snapshots`
- `g`: clip toggle to `./clips` (requires `ffmpeg`)
- `t`: trim toggle to `./clips` (requires `ffmpeg`)
- `+` / `-`: enlarge / shrink window
- `m`: mute
- `[` / `]`: speed `- / +` 0.1x (clamped to 0.1x–3.0x)
- `f`: fullscreen
- `b`: browse playlist (OSD)
- `Backspace/Delete`: move current file to Trash (press twice to confirm)
- `:`: command mode
- `Esc`: quit

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

By default, resume positions are kept only for this session (switching back/forth resumes correctly, but restarting `pp` starts fresh).

- Disable entirely with `--no-resume`
- Persist across runs with `--persist-resume` (writes `~/.pp_timestamps_go.json`)
