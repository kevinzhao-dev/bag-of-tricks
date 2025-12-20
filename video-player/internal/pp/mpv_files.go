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

a     seek -%.0f relative
d     seek +%.0f relative
w     seek +%.0f relative
s     seek -%.0f relative

j script-message pp_prev_wrap
k script-message pp_next_wrap
e script-message pp_prev_wrap
r script-message pp_next_wrap
ENTER script-message pp_next_wrap

b script-message pp_browser_toggle
B script-message pp_browser_toggle

BS  script-message pp_trash_current
DEL script-message pp_trash_current

m cycle mute
[ add speed -0.1
] add speed 0.1

1 seek 10 absolute-percent
2 seek 20 absolute-percent
3 seek 30 absolute-percent
4 seek 40 absolute-percent
5 seek 50 absolute-percent
6 seek 60 absolute-percent
7 seek 70 absolute-percent
8 seek 80 absolute-percent
9 seek 90 absolute-percent

f cycle fullscreen

q quit
ESC quit
`)+"\n",
		opts.SeekShortS, opts.SeekShortS, opts.SeekLongS, opts.SeekLongS,
		opts.SeekShortS, opts.SeekShortS, opts.SeekLongS, opts.SeekLongS,
	)

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
local utils = require 'mp.utils'

local active = false
local sel = 0
local win = 13
local pending_trash_until = 0

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

local function is_probably_local_file(p)
  if p == nil or p == "" then return false end
  if string.find(p, "://") ~= nil then return false end
  return true
end

local size_cache = {}

local function resolve_local_path(p)
  if p == nil or p == "" then return "" end
  local path = p
  if string.sub(path, 1, 2) == "~/" then
    local home = os.getenv("HOME")
    if home ~= nil and home ~= "" then
      path = home .. string.sub(path, 2)
    end
  end
  if string.sub(path, 1, 1) ~= "/" then
    local wd = mp.get_property("working-directory")
    if wd ~= nil and wd ~= "" then
      path = utils.join_path(wd, path)
    end
  end
  return path
end

local function size_suffix_gb(p)
  if p == nil or p == "" then return "" end
  if size_cache[p] ~= nil then
    if size_cache[p] == false then return "" end
    return size_cache[p]
  end
  if not is_probably_local_file(p) then
    size_cache[p] = false
    return ""
  end
  local info = utils.file_info(resolve_local_path(p))
  if info == nil or info.size == nil then
    size_cache[p] = false
    return ""
  end
  local gb = info.size / (1024 * 1024 * 1024)
  local s = string.format("  %.2fGB", gb)
  size_cache[p] = s
  return s
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
  table.insert(lines, string.format("Playlist (%d/%d)  ↑↓ move  Enter play  Esc close  (GB)", sel + 1, n))
  for i = start, finish do
    local p = pl[i + 1]
    local mark = "  "
    if i == sel then mark = "→ " end
    local now = "  "
    if i == pos then now = "• " end
    local title = p.title
    if title == nil or title == "" then title = basename(p.filename) end
    local size = size_suffix_gb(p.filename)
    table.insert(lines, string.format("%s%s%3d  %s%s", mark, now, i + 1, title, size))
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

local function next_wrap()
  local count = mp.get_property_number('playlist-count', 0)
  if count <= 0 then return end
  local pos = mp.get_property_number('playlist-pos', 0)
  local next = pos + 1
  if next >= count then next = 0 end
  mp.commandv("playlist-play-index", next)
end

local function prev_wrap()
  local count = mp.get_property_number('playlist-count', 0)
  if count <= 0 then return end
  local pos = mp.get_property_number('playlist-pos', 0)
  local prev = pos - 1
  if prev < 0 then prev = count - 1 end
  mp.commandv("playlist-play-index", prev)
end

mp.register_script_message("pp_next_wrap", next_wrap)
mp.register_script_message("pp_prev_wrap", prev_wrap)

local function applescript_escape(s)
  s = string.gsub(s, "\\", "\\\\")
  s = string.gsub(s, "\"", "\\\"")
  return s
end

local function trash_current()
  local now = mp.get_time()
  if now < pending_trash_until then
    pending_trash_until = 0
  else
    pending_trash_until = now + 2.0
    mp.osd_message("Trash file? Press Backspace/Delete again to confirm", 2.0)
    return
  end

  local p = mp.get_property("path")
  if not is_probably_local_file(p) then
    mp.osd_message("Trash: not a local file", 1.5)
    return
  end

  local pos = mp.get_property_number("playlist-pos", 0)
  local count = mp.get_property_number("playlist-count", 0)

  local script = 'tell application "Finder" to delete POSIX file "' .. applescript_escape(p) .. '"'
  local res = utils.subprocess({ args = { "osascript", "-e", script } })
  if res.error ~= nil then
    mp.osd_message("Trash failed: " .. tostring(res.error), 2.0)
    return
  end
  if res.status ~= 0 then
    mp.osd_message("Trash failed", 2.0)
    return
  end

  mp.commandv("playlist-remove", pos)
  mp.osd_message("Moved to Trash: " .. basename(p), 1.5)

  local newCount = count - 1
  if newCount <= 0 then
    mp.commandv("quit")
    return
  end
  if pos >= newCount then pos = newCount - 1 end
  if pos < 0 then pos = 0 end
  mp.commandv("playlist-play-index", pos)
end

mp.register_script_message("pp_trash_current", trash_current)
`)
	script += "\n"

	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}
