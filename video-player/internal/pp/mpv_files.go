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

b script-message pp_browser_toggle

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

func WriteTempBrowserScript() (path string, cleanup func(), err error) {
	dir := os.TempDir()
	name := "pp-browser-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".lua"
	path = filepath.Join(dir, name)

	// Minimal OSD playlist browser: toggle with `b`, navigate with arrows, Enter to play.
	script := strings.TrimSpace(`
local mp = require 'mp'

local active = false
local sel = 0
local win = 13

local function basename(p)
  if p == nil then return "" end
  return string.gsub(p, "^.*[/\\\\]", "")
end

local function clamp(v, lo, hi)
  if v < lo then return lo end
  if v > hi then return hi end
  return v
end

local function playlist()
  local pl = mp.get_property_native('playlist')
  if pl == nil then return {} end
  return pl
end

local function redraw()
  local pl = playlist()
  local n = #pl
  if n == 0 then
    mp.osd_message("No playlist", 1.5)
    return
  end
  sel = clamp(sel, 0, n - 1)

  local half = math.floor(win / 2)
  local start = clamp(sel - half, 0, math.max(0, n - win))
  local finish = math.min(n - 1, start + win - 1)

  local pos = mp.get_property_number('playlist-pos', 0)
  local lines = {}
  table.insert(lines, string.format("Playlist (%d/%d)  ↑↓ move  Enter play  Esc close", sel + 1, n))
  for i = start, finish do
    local p = pl[i + 1]
    local mark = "  "
    if i == sel then mark = "→ " end
    local now = "  "
    if i == pos then now = "• " end
    local title = p.title
    if title == nil or title == "" then title = basename(p.filename) end
    table.insert(lines, string.format("%s%s%3d  %s", mark, now, i + 1, title))
  end

  mp.osd_message(table.concat(lines, "\n"), 3600)
end

local function close()
  active = false
  mp.osd_message("", 0)
  mp.remove_key_binding("pp_browser_up")
  mp.remove_key_binding("pp_browser_down")
  mp.remove_key_binding("pp_browser_pgup")
  mp.remove_key_binding("pp_browser_pgdn")
  mp.remove_key_binding("pp_browser_enter")
  mp.remove_key_binding("pp_browser_close")
end

local function open()
  local pos = mp.get_property_number('playlist-pos', 0)
  sel = pos
  active = true
  mp.add_forced_key_binding("UP", "pp_browser_up", function() sel = sel - 1; redraw() end, {repeatable=true})
  mp.add_forced_key_binding("DOWN", "pp_browser_down", function() sel = sel + 1; redraw() end, {repeatable=true})
  mp.add_forced_key_binding("PGUP", "pp_browser_pgup", function() sel = sel - win; redraw() end, {repeatable=true})
  mp.add_forced_key_binding("PGDWN", "pp_browser_pgdn", function() sel = sel + win; redraw() end, {repeatable=true})
  mp.add_forced_key_binding("ENTER", "pp_browser_enter", function()
    mp.commandv("playlist-play-index", sel)
    close()
  end)
  mp.add_forced_key_binding("ESC", "pp_browser_close", function() close() end)
  redraw()
end

local function toggle()
  if active then close() else open() end
end

mp.register_script_message("pp_browser_toggle", toggle)
`)
	script += "\n"

	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}
