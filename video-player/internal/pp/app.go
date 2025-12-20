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
	"sync"
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
	AutoPlay   bool

	Timestamps  *TimestampStore
	ResumeState bool

	helpShown bool

	pauseAfterLoad bool

	lastMu         sync.Mutex
	lastSamplePath string
	lastSamplePos  float64
	lastSavedAt    time.Time
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
		select {
		case <-a.MPV.Done():
			return nil
		default:
		}

		if !tty.InputReady(in) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

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
			quit, err := a.handleRune(key.Rune, in)
			if err != nil {
				return err
			}
			if quit {
				return nil
			}
		}
	}
}

func (a *App) handleRune(r rune, in *bufio.Reader) (quit bool, err error) {
	switch r {
	case 'q':
		_ = a.persistPosition()
		_ = a.MPV.Command(context.Background(), "quit")
		return true, nil
	case 'j', 'e':
		return false, a.Prev(context.Background())
	case 'k', 'r', '\r', '\n':
		return false, a.Next(context.Background())
	case 'a':
		_ = a.MPV.Command(context.Background(), "seek", -a.SeekShortS, "relative")
		a.osd(fmt.Sprintf("◀ %.0fs", a.SeekShortS))
		return false, nil
	case 'd':
		_ = a.MPV.Command(context.Background(), "seek", a.SeekShortS, "relative")
		a.osd(fmt.Sprintf("▶ %.0fs", a.SeekShortS))
		return false, nil
	case 'w':
		_ = a.MPV.Command(context.Background(), "seek", a.SeekLongS, "relative")
		a.osd(fmt.Sprintf("▶ %.0fs", a.SeekLongS))
		return false, nil
	case 's':
		_ = a.MPV.Command(context.Background(), "seek", -a.SeekLongS, "relative")
		a.osd(fmt.Sprintf("◀ %.0fs", a.SeekLongS))
		return false, nil
	case 'm':
		_ = a.MPV.Command(context.Background(), "cycle", "mute")
		a.osd("Toggle mute")
		return false, nil
	case '[':
		return false, a.bumpSpeed(-0.1)
	case ']':
		return false, a.bumpSpeed(0.1)
	case 'h', '?':
		a.ShowHelpOnce()
		return false, nil
	case ':':
		return a.commandMode(in)
	default:
		if r >= '1' && r <= '9' {
			pct := int(r-'0') * 10
			_ = a.MPV.Command(context.Background(), "seek", pct, "absolute-percent")
			a.osd(fmt.Sprintf("Jump %d%%", pct))
			return false, nil
		}
		return false, nil
	}
}

func (a *App) ShowHelpOnce() {
	if a.helpShown {
		a.osd("Keys: space pause, arrows/WASD seek, j/k prev/next, : commands, q quit")
		return
	}
	a.helpShown = true
	fmt.Fprintln(os.Stdout, "\npp (Go) controls:")
	fmt.Fprintln(os.Stdout, "  Space  play/pause")
	fmt.Fprintln(os.Stdout, "  ←/→    seek ±short")
	fmt.Fprintln(os.Stdout, "  ↑/↓    seek ±long")
	fmt.Fprintln(os.Stdout, "  WASD   seek (same as arrows)")
	fmt.Fprintln(os.Stdout, "  1-9    jump 10%-90%")
	fmt.Fprintln(os.Stdout, "  j/k    prev/next video")
	fmt.Fprintln(os.Stdout, "  e/r    prev/next video")
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
	a.syncIndex()
	if len(a.Playlist) > 0 && a.Index >= len(a.Playlist)-1 {
		_ = a.MPV.Command(ctx, "playlist-play-index", 0)
		a.Index = 0
		a.osd("Loop → start")
		if !a.Continuous {
			a.pauseAfterLoad = false
		}
		return nil
	}

	_ = a.MPV.Command(ctx, "playlist-next", "weak")
	a.syncIndex()
	a.osd("Next")
	if !a.Continuous {
		a.pauseAfterLoad = false
	}
	return nil
}

func (a *App) Prev(ctx context.Context) error {
	_ = a.persistPosition()
	a.syncIndex()
	if len(a.Playlist) > 0 && a.Index <= 0 {
		last := len(a.Playlist) - 1
		_ = a.MPV.Command(ctx, "playlist-play-index", last)
		a.Index = last
		a.osd("Loop → end")
		if !a.Continuous {
			a.pauseAfterLoad = false
		}
		return nil
	}

	_ = a.MPV.Command(ctx, "playlist-prev", "weak")
	a.syncIndex()
	a.osd("Prev")
	if !a.Continuous {
		a.pauseAfterLoad = false
	}
	return nil
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
	return nil
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
				// Switching can happen from mpv window keybindings; flush last sampled position
				// so toggling back/forth resumes instead of starting from 0.
				_ = a.flushLastSample()
				var n int
				_ = json.Unmarshal(ev.Raw["data"], &n)
				if n >= 0 {
					a.Index = n
				}
			}
		case "end-file":
			_ = a.persistPosition()
			if !a.Continuous && !a.AutoPlay {
				// mpv will move to the next file in the playlist; pause once it loads.
				a.pauseAfterLoad = true
			}
		case "file-loaded":
			a.syncIndex()
			_ = a.RestorePosition(context.Background())
			if a.AutoPlay {
				_ = a.MPV.Command(context.Background(), "set_property", "pause", false)
			}
			if a.pauseAfterLoad && !a.AutoPlay {
				_ = a.MPV.Command(context.Background(), "set_property", "pause", true)
				a.osd("Paused (space to play)")
				a.pauseAfterLoad = false
			}
		}
	}
}

func (a *App) commandMode(in *bufio.Reader) (quit bool, err error) {
	line, ok, err := tty.ReadLine(in, ":")
	if err != nil {
		return false, err
	}
	if !ok {
		a.osd("Canceled")
		return false, nil
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return false, nil
	}

	fields := splitCmd(line)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "h", "help", "?":
		a.ShowHelpOnce()
		a.osd(":ls, :open, :seek, :jump, :n, :p, :q")
		return false, nil
	case "q", "quit", "exit":
		_ = a.persistPosition()
		_ = a.MPV.Command(context.Background(), "quit")
		return true, nil
	case "n", "next":
		return false, a.Next(context.Background())
	case "p", "prev":
		return false, a.Prev(context.Background())
	case "ls", "list":
		a.printPlaylist()
		a.osd(fmt.Sprintf("%d files", len(a.Playlist)))
		return false, nil
	case "open", "o":
		if len(args) == 0 {
			a.osd("open: need index or substring")
			return false, nil
		}
		target := strings.Join(args, " ")
		if i, err := strconv.Atoi(target); err == nil {
			return false, a.Load(context.Background(), i-1)
		}
		i := a.findBySubstring(target)
		if i < 0 {
			a.osd("not found")
			return false, nil
		}
		return false, a.Load(context.Background(), i)
	case "seek":
		if len(args) != 1 {
			a.osd("seek: usage seek +10 | -10")
			return false, nil
		}
		sec, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			a.osd("seek: invalid seconds")
			return false, nil
		}
		_ = a.MPV.Command(context.Background(), "seek", sec, "relative")
		a.osd(fmt.Sprintf("Seek %.0fs", sec))
		return false, nil
	case "jump":
		if len(args) != 1 {
			a.osd("jump: usage jump 50% | 120")
			return false, nil
		}
		if strings.HasSuffix(args[0], "%") {
			pctStr := strings.TrimSuffix(args[0], "%")
			pct, err := strconv.ParseFloat(pctStr, 64)
			if err != nil {
				a.osd("jump: invalid percent")
				return false, nil
			}
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			_ = a.MPV.Command(context.Background(), "seek", pct, "absolute-percent")
			a.osd(fmt.Sprintf("Jump %.0f%%", pct))
			return false, nil
		}
		sec, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			a.osd("jump: invalid seconds")
			return false, nil
		}
		_ = a.MPV.Command(context.Background(), "seek", sec, "absolute")
		a.osd(fmt.Sprintf("Jump %.0fs", sec))
		return false, nil
	default:
		a.osd("unknown command (try :help)")
		return false, nil
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
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for range t.C {
		a.sampleAndMaybeSave()
	}
}

func (a *App) sampleAndMaybeSave() {
	path, err := a.MPV.GetString(withTimeout(200*time.Millisecond), "path")
	if err != nil || path == "" {
		return
	}
	pos, err := a.MPV.GetFloat(withTimeout(200*time.Millisecond), "time-pos")
	if err != nil || pos < 0 {
		return
	}

	a.lastMu.Lock()
	a.lastSamplePath = path
	a.lastSamplePos = pos
	shouldSave := time.Since(a.lastSavedAt) >= 3*time.Second
	if shouldSave {
		a.lastSavedAt = time.Now()
	}
	a.lastMu.Unlock()

	// Keep in-memory store fresh; persist to disk every few seconds.
	a.Timestamps.Set(path, pos)
	if shouldSave {
		_ = a.Timestamps.Save()
	}
}

func (a *App) flushLastSample() error {
	if !a.ResumeState || a.Timestamps == nil {
		return nil
	}
	a.lastMu.Lock()
	path := a.lastSamplePath
	pos := a.lastSamplePos
	a.lastMu.Unlock()
	if path == "" || pos < 0 {
		return nil
	}
	a.Timestamps.Set(path, pos)
	return a.Timestamps.Save()
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
