package pp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"video-player/internal/mpv"
	"video-player/internal/tty"
)

type App struct {
	MPV  *mpv.Client
	Proc *mpv.Process

	Playlist []string
	Index    int

	SeekShortS float64
	SeekLongS  float64
	Continuous bool

	Timestamps  *TimestampStore
	ResumeState bool

	helpShown bool

	pauseAfterLoad bool
}

func (a *App) Run() error {
	if a.MPV == nil {
		return errors.New("mpv client is nil")
	}

	_ = a.MPV.Command(context.Background(), "observe_property", 1, "playlist-pos")

	go a.eventLoop()
	go a.periodicSaveLoop()
	in := bufio.NewReader(os.Stdin)

	for {
		key, err := tty.ReadKey(in)
		if err != nil {
			return err
		}

		switch key.Kind {
		case tty.KeyQuit:
			_ = a.persistPosition()
			_ = a.MPV.Command(context.Background(), "quit")
			return nil
		case tty.KeySpace:
			_ = a.MPV.Command(context.Background(), "cycle", "pause")
			a.osd("Toggle pause")
		case tty.KeyLeft:
			_ = a.MPV.Command(context.Background(), "seek", -a.SeekShortS, "relative")
			a.osd(fmt.Sprintf("◀ %.0fs", a.SeekShortS))
		case tty.KeyRight:
			_ = a.MPV.Command(context.Background(), "seek", a.SeekShortS, "relative")
			a.osd(fmt.Sprintf("▶ %.0fs", a.SeekShortS))
		case tty.KeyUp:
			_ = a.MPV.Command(context.Background(), "seek", a.SeekLongS, "relative")
			a.osd(fmt.Sprintf("▶ %.0fs", a.SeekLongS))
		case tty.KeyDown:
			_ = a.MPV.Command(context.Background(), "seek", -a.SeekLongS, "relative")
			a.osd(fmt.Sprintf("◀ %.0fs", a.SeekLongS))
		case tty.KeyRune:
			if err := a.handleRune(key.Rune, in); err != nil {
				return err
			}
		}
	}
}

func (a *App) handleRune(r rune, in *bufio.Reader) error {
	switch r {
	case 'q':
		_ = a.persistPosition()
		_ = a.MPV.Command(context.Background(), "quit")
		return nil
	case 'j':
		return a.Prev(context.Background())
	case 'k', '\r', '\n':
		return a.Next(context.Background())
	case 's':
		_ = a.MPV.Command(context.Background(), "seek", 0, "absolute")
		a.osd("Start")
		return nil
	case 'e':
		dur, err := a.MPV.GetFloat(withTimeout(400*time.Millisecond), "duration")
		if err == nil && dur > 0 {
			target := dur - 5
			if target < 0 {
				target = 0
			}
			_ = a.MPV.Command(context.Background(), "seek", target, "absolute")
			a.osd("End (-5s)")
		}
		return nil
	case 'm':
		_ = a.MPV.Command(context.Background(), "cycle", "mute")
		a.osd("Toggle mute")
		return nil
	case '[':
		return a.bumpSpeed(-0.1)
	case ']':
		return a.bumpSpeed(0.1)
	case 'h', '?':
		a.ShowHelpOnce()
		return nil
	case ':':
		return a.commandMode(in)
	default:
		return nil
	}
}

func (a *App) ShowHelpOnce() {
	if a.helpShown {
		a.osd("Keys: space pause, arrows seek, j/k prev/next, : commands, q quit")
		return
	}
	a.helpShown = true
	fmt.Fprintln(os.Stdout, "\npp (Go) controls:")
	fmt.Fprintln(os.Stdout, "  Space  play/pause")
	fmt.Fprintln(os.Stdout, "  ←/→    seek ±short")
	fmt.Fprintln(os.Stdout, "  ↑/↓    seek ±long")
	fmt.Fprintln(os.Stdout, "  j/k    prev/next video")
	fmt.Fprintln(os.Stdout, "  s/e    start / end(-5s)")
	fmt.Fprintln(os.Stdout, "  m      mute")
	fmt.Fprintln(os.Stdout, "  [/ ]   speed -/+ 0.1x")
	fmt.Fprintln(os.Stdout, "  :      command mode (ls/open/seek/jump)")
	fmt.Fprintln(os.Stdout, "  q/Esc  quit")
	fmt.Fprintln(os.Stdout)
	a.osd("Ready. Press : for commands, h for help.")
}

func (a *App) osd(msg string) {
	ctx := withTimeout(200 * time.Millisecond)
	_ = a.MPV.Command(ctx, "show-text", msg, 1500)
}

func withTimeout(d time.Duration) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), d)
	return ctx
}

func (a *App) bumpSpeed(delta float64) error {
	cur, err := a.MPV.GetFloat(withTimeout(250*time.Millisecond), "speed")
	if err != nil || cur <= 0 {
		cur = 1.0
	}
	next := cur + delta
	if next < 0.1 {
		next = 0.1
	}
	if next > 3.0 {
		next = 3.0
	}
	_ = a.MPV.Command(context.Background(), "set_property", "speed", next)
	a.osd(fmt.Sprintf("Speed %.1fx", next))
	return nil
}

func (a *App) Next(ctx context.Context) error {
	_ = a.persistPosition()
	_ = a.MPV.Command(ctx, "playlist-next", "weak")
	a.syncIndex()
	a.osd("Next")
	if !a.Continuous {
		a.pauseAfterLoad = false
	}
	return a.RestorePosition(context.Background())
}

func (a *App) Prev(ctx context.Context) error {
	_ = a.persistPosition()
	_ = a.MPV.Command(ctx, "playlist-prev", "weak")
	a.syncIndex()
	a.osd("Prev")
	if !a.Continuous {
		a.pauseAfterLoad = false
	}
	return a.RestorePosition(context.Background())
}

func (a *App) Load(ctx context.Context, index int) error {
	if index < 0 || index >= len(a.Playlist) {
		return fmt.Errorf("index out of range: %d", index)
	}
	a.Index = index
	_ = a.persistPosition()
	if err := a.MPV.Command(ctx, "playlist-play-index", a.Index); err != nil {
		return err
	}
	a.osd(fmt.Sprintf("Open %s (%d/%d)", filepath.Base(a.Playlist[a.Index]), a.Index+1, len(a.Playlist)))
	if !a.Continuous {
		a.pauseAfterLoad = false
	}
	return a.RestorePosition(context.Background())
}

func (a *App) RestorePosition(ctx context.Context) error {
	if !a.ResumeState || a.Timestamps == nil {
		return nil
	}
	path, err := a.MPV.GetString(withTimeout(300*time.Millisecond), "path")
	if err != nil || path == "" {
		path = a.Playlist[a.Index]
	}
	sec, ok := a.Timestamps.Get(path)
	if !ok || sec <= 0.5 {
		return nil
	}
	_ = a.MPV.Command(ctx, "seek", sec, "absolute")
	a.osd(fmt.Sprintf("Resume %.0fs", sec))
	return nil
}

func (a *App) persistPosition() error {
	if !a.ResumeState || a.Timestamps == nil {
		return nil
	}
	pos, err := a.MPV.GetFloat(withTimeout(300*time.Millisecond), "time-pos")
	if err != nil {
		return nil
	}
	path, err := a.MPV.GetString(withTimeout(300*time.Millisecond), "path")
	if err != nil || path == "" {
		path = a.Playlist[a.Index]
	}
	a.Timestamps.Set(path, pos)
	return a.Timestamps.Save()
}

func (a *App) eventLoop() {
	for ev := range a.MPV.Events() {
		switch ev.Name {
		case "property-change":
			var name string
			_ = json.Unmarshal(ev.Raw["name"], &name)
			if name == "playlist-pos" {
				var n int
				_ = json.Unmarshal(ev.Raw["data"], &n)
				if n >= 0 {
					a.Index = n
				}
			}
		case "end-file":
			_ = a.persistPosition()
			if !a.Continuous {
				// mpv will move to the next file in the playlist; pause once it loads.
				a.pauseAfterLoad = true
			}
		case "file-loaded":
			a.syncIndex()
			if a.pauseAfterLoad {
				_ = a.MPV.Command(context.Background(), "set_property", "pause", true)
				a.osd("Paused (space to play)")
				a.pauseAfterLoad = false
			}
		}
	}
}

func (a *App) commandMode(in *bufio.Reader) error {
	line, ok, err := tty.ReadLine(in, ":")
	if err != nil {
		return err
	}
	if !ok {
		a.osd("Canceled")
		return nil
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	fields := splitCmd(line)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "h", "help", "?":
		a.ShowHelpOnce()
		a.osd(":ls, :open, :seek, :jump, :n, :p, :q")
		return nil
	case "q", "quit", "exit":
		_ = a.persistPosition()
		_ = a.MPV.Command(context.Background(), "quit")
		return nil
	case "n", "next":
		return a.Next(context.Background())
	case "p", "prev":
		return a.Prev(context.Background())
	case "ls", "list":
		a.printPlaylist()
		a.osd(fmt.Sprintf("%d files", len(a.Playlist)))
		return nil
	case "open", "o":
		if len(args) == 0 {
			a.osd("open: need index or substring")
			return nil
		}
		target := strings.Join(args, " ")
		if i, err := strconv.Atoi(target); err == nil {
			return a.Load(context.Background(), i-1)
		}
		i := a.findBySubstring(target)
		if i < 0 {
			a.osd("not found")
			return nil
		}
		return a.Load(context.Background(), i)
	case "seek":
		if len(args) != 1 {
			a.osd("seek: usage seek +10 | -10")
			return nil
		}
		sec, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			a.osd("seek: invalid seconds")
			return nil
		}
		_ = a.MPV.Command(context.Background(), "seek", sec, "relative")
		a.osd(fmt.Sprintf("Seek %.0fs", sec))
		return nil
	case "jump":
		if len(args) != 1 {
			a.osd("jump: usage jump 50% | 120")
			return nil
		}
		if strings.HasSuffix(args[0], "%") {
			pctStr := strings.TrimSuffix(args[0], "%")
			pct, err := strconv.ParseFloat(pctStr, 64)
			if err != nil {
				a.osd("jump: invalid percent")
				return nil
			}
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			_ = a.MPV.Command(context.Background(), "seek", pct, "absolute-percent")
			a.osd(fmt.Sprintf("Jump %.0f%%", pct))
			return nil
		}
		sec, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			a.osd("jump: invalid seconds")
			return nil
		}
		_ = a.MPV.Command(context.Background(), "seek", sec, "absolute")
		a.osd(fmt.Sprintf("Jump %.0fs", sec))
		return nil
	default:
		a.osd("unknown command (try :help)")
		return nil
	}
}

func (a *App) printPlaylist() {
	fmt.Fprintln(os.Stdout, "\nPlaylist:")
	for i, p := range a.Playlist {
		prefix := "  "
		if i == a.Index {
			prefix = "→ "
		}
		fmt.Fprintf(os.Stdout, "%s%3d  %s\n", prefix, i+1, filepath.Base(p))
	}
	fmt.Fprintln(os.Stdout)
}

func (a *App) findBySubstring(q string) int {
	q = strings.ToLower(q)
	for i, p := range a.Playlist {
		if strings.Contains(strings.ToLower(filepath.Base(p)), q) {
			return i
		}
	}
	return -1
}

func (a *App) syncIndex() {
	n, err := a.MPV.GetInt(withTimeout(250*time.Millisecond), "playlist-pos")
	if err == nil && n >= 0 {
		a.Index = n
	}
}

func (a *App) periodicSaveLoop() {
	if !a.ResumeState || a.Timestamps == nil {
		return
	}
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for range t.C {
		_ = a.persistPosition()
	}
}

func splitCmd(s string) []string {
	out := []string{}
	cur := strings.Builder{}
	inQuote := false
	var quote rune
	for _, r := range s {
		if inQuote {
			if r == quote {
				inQuote = false
				continue
			}
			cur.WriteRune(r)
			continue
		}
		if r == '"' || r == '\'' {
			inQuote = true
			quote = r
			continue
		}
		if r == ' ' || r == '\t' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}
