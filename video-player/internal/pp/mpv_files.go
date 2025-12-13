package pp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func WriteTempPlaylist(files []string) (path string, cleanup func(), err error) {
	dir := os.TempDir()
	name := "pp-playlist-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".m3u"
	path = filepath.Join(dir, name)
	content := strings.Join(files, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}

type KeybindOptions struct {
	SeekShortS float64
	SeekLongS  float64
}

func WriteTempInputConf(opts KeybindOptions) (path string, cleanup func(), err error) {
	dir := os.TempDir()
	name := "pp-input-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".conf"
	path = filepath.Join(dir, name)
	// Keep bindings simple and mpv-native so they work when the mpv window is focused.
	conf := fmt.Sprintf(strings.TrimSpace(`
SPACE cycle pause
LEFT  seek -%.0f relative
RIGHT seek +%.0f relative
UP    seek +%.0f relative
DOWN  seek -%.0f relative

j playlist-prev
k playlist-next
ENTER playlist-next

s seek 0 absolute
e seek 100 absolute-percent; seek -5 relative

m cycle mute
[ add speed -0.1
] add speed 0.1

q quit
ESC quit
`)+"\n", opts.SeekShortS, opts.SeekShortS, opts.SeekLongS, opts.SeekLongS)

	if err := os.WriteFile(path, []byte(conf), 0o644); err != nil {
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}
