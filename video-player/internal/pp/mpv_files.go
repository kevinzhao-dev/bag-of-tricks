package pp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func WriteTempPlaylist(files []string, displayNames []string) (path string, cleanup func(), err error) {
	dir := os.TempDir()
	name := "pp-playlist-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".m3u"
	path = filepath.Join(dir, name)

	var b strings.Builder
	if len(displayNames) == len(files) {
		b.WriteString("#EXTM3U\n")
		for i, f := range files {
			fmt.Fprintf(&b, "#EXTINF:0,%s\n%s\n", displayNames[i], f)
		}
	} else {
		b.WriteString(strings.Join(files, "\n") + "\n")
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}

type KeybindOptions struct {
	SeekShortS float64
	SeekFineS  float64
	SeekLongS  float64
}

func WriteTempInputConf(opts KeybindOptions) (path string, cleanup func(), err error) {
	dir := os.TempDir()
	name := "pp-input-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".conf"
	path = filepath.Join(dir, name)
	// Keep bindings simple and mpv-native so they work when the mpv window is focused.
	conf := fmt.Sprintf(strings.TrimSpace(`
SPACE cycle pause
LEFT  seek -%s relative
RIGHT seek +%s relative
UP    seek +%s relative
DOWN  seek -%s relative

z     seek -%s relative
c     seek +%s relative

a     seek -%s relative
d     seek +%s relative
w     seek +%s relative
s     seek -%s relative
k     seek +%s relative
j     seek -%s relative

q script-message pp_prev_wrap
e script-message pp_next_wrap
h script-message pp_prev_wrap
l script-message pp_next_wrap
ENTER script-message pp_next_wrap

b script-message pp_browser_toggle
B script-message pp_browser_toggle

x script-message pp_screenshot
X script-message pp_screenshot
g script-message pp_clip_toggle
G script-message pp_clip_toggle
t script-message pp_trim_toggle
T script-message pp_trim_toggle

BS  script-message pp_trash_current
DEL script-message pp_trash_current

m cycle mute
[ add speed -0.1
] add speed 0.1

+ add window-scale 0.1
= add window-scale 0.1
- add window-scale -0.1
_ add window-scale -0.1

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

ESC quit
`)+"\n",
		formatSeekSeconds(opts.SeekFineS),
		formatSeekSeconds(opts.SeekFineS),
		formatSeekSeconds(opts.SeekLongS),
		formatSeekSeconds(opts.SeekLongS),
		formatSeekSeconds(opts.SeekFineS),
		formatSeekSeconds(opts.SeekFineS),
		formatSeekSeconds(opts.SeekShortS),
		formatSeekSeconds(opts.SeekShortS),
		formatSeekSeconds(opts.SeekLongS),
		formatSeekSeconds(opts.SeekLongS),
		formatSeekSeconds(opts.SeekLongS),
		formatSeekSeconds(opts.SeekLongS),
	)

	if err := os.WriteFile(path, []byte(conf), 0o644); err != nil {
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}

func formatSeekSeconds(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
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
local win = 18
local pending_trash_until = 0
local overlay = mp.create_osd_overlay("ass-events")

local function basename(p)
  if p == nil then return "" end
  return string.gsub(p, "^.*[/\\\\]", "")
end

local function split_dir(title)
  local d = string.match(title, "^(.*)/[^/]+$")
  local f = string.match(title, "([^/]+)$") or title
  return d, f
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

local function ass_escape(s)
  s = string.gsub(s, "\\", "\\\\")
  s = string.gsub(s, "{", "\\{")
  s = string.gsub(s, "}", "\\}")
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

  -- ASS header: small font, semi-transparent dark background, top-left
  local fs = 16
  local lines = {}
  table.insert(lines, string.format(
    "{\\an7\\fs%d\\fnmonospace\\bord1\\shad0\\1c&HFFFFFF&\\3c&H000000&\\4c&H000000&\\4a&H80&}" ..
    "{\\b1}Playlist (%d/%d){\\b0}   ↑↓ move  Enter play  Esc close",
    fs, sel + 1, n))

  local last_dir = false
  for i = start, finish do
    local p = pl[i + 1]
    local title = p.title
    if title == nil or title == "" then title = basename(p.filename) end
    local d, fname = split_dir(title)

    if d ~= last_dir then
      if d ~= nil then
        table.insert(lines, string.format("{\\1c&H88CCFF&}  ▸ %s/{\\1c&HFFFFFF&}", ass_escape(d)))
      end
      last_dir = d
    end

    local size = size_suffix_gb(p.filename)
    local indent = d ~= nil and "    " or "  "
    local prefix = ""

    if i == sel and i == pos then
      prefix = string.format("{\\1c&H00FFFF&}→ • %3d%s", i + 1, indent)
    elseif i == sel then
      prefix = string.format("{\\1c&H00FF00&}→   %3d%s", i + 1, indent)
    elseif i == pos then
      prefix = string.format("{\\1c&H88DDFF&}  • %3d%s", i + 1, indent)
    else
      prefix = string.format("    %3d%s", i + 1, indent)
    end

    local line = prefix .. ass_escape(fname)
    if size ~= "" then
      line = line .. "{\\1c&HAAAAAA&}" .. ass_escape(size) .. "{\\1c&HFFFFFF&}"
    end
    table.insert(lines, line)
  end

  overlay.data = table.concat(lines, "\\N")
  overlay:update()
end

local function close()
  active = false
  overlay:remove()
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

local function mkdir_p(path)
  if path == nil or path == "" then return end
  if utils.file_info(path) ~= nil then return end
  local sep = package.config:sub(1,1)
  if sep == "\\" then
    utils.subprocess({ args = { "cmd", "/c", "mkdir", path } })
  else
    utils.subprocess({ args = { "mkdir", "-p", path } })
  end
end

local function screenshot()
  local wd = mp.get_property("working-directory")
  if wd == nil or wd == "" then
    mp.osd_message("Snapshot failed (no wd)", 1.5)
    return
  end

  local dir = utils.join_path(wd, "snapshots")
  mkdir_p(dir)

  local p = mp.get_property("path") or ""
  local base = basename(p)
  base = string.gsub(base, "%.%w+$", "")
  if base == "" then base = "snapshot" end

  local pos = mp.get_property_number("time-pos", 0) or 0
  if pos < 0 then pos = 0 end
  local ms = math.floor(pos * 1000 + 0.5)
  local h = math.floor(ms / 3600000)
  local m = math.floor(ms / 60000) % 60
  local s = math.floor(ms / 1000) % 60
  local mmm = ms % 1000
  local name = string.format("%s_%02dh%02dm%02ds%03dms.jpg", base, h, m, s, mmm)
  local out = utils.join_path(dir, name)

  mp.commandv("screenshot-to-file", out, "video")
  mp.osd_message("Saved: " .. name, 1.5)
end

mp.register_script_message("pp_screenshot", screenshot)

local clip_active = false
local clip_start_pos = 0
local clip_start_path = ""

local function format_ts(sec)
  if sec < 0 then sec = 0 end
  local ms = math.floor(sec * 1000 + 0.5)
  local h = math.floor(ms / 3600000)
  local m = math.floor(ms / 60000) % 60
  local s = math.floor(ms / 1000) % 60
  local mmm = ms % 1000
  return string.format("%02dh%02dm%02ds%03dms", h, m, s, mmm)
end

local function unique_clip_path(dir, base, start_tag, end_tag, ext)
  local name = string.format("%s_%s-%s%s", base, start_tag, end_tag, ext)
  local out = utils.join_path(dir, name)
  if utils.file_info(out) == nil then return out, name end
  for i = 2, 999 do
    name = string.format("%s_%s-%s-%d%s", base, start_tag, end_tag, i, ext)
    out = utils.join_path(dir, name)
    if utils.file_info(out) == nil then return out, name end
  end
  return out, name
end

local function clip_toggle()
  local p = mp.get_property("path") or ""
  if p == "" then
    mp.osd_message("Clip failed (no file)", 1.5)
    return
  end
  if not is_probably_local_file(p) then
    mp.osd_message("Clip failed (not local)", 1.5)
    return
  end
  local pos = mp.get_property_number("time-pos", 0) or 0
  if pos < 0 then pos = 0 end

  if not clip_active then
    clip_active = true
    clip_start_pos = pos
    clip_start_path = p
    mp.osd_message("Clip start", 1.5)
    return
  end

  clip_active = false
  if p ~= clip_start_path then
    mp.osd_message("Clip canceled (file changed)", 1.5)
    return
  end
  if pos <= clip_start_pos + 0.05 then
    mp.osd_message("Clip too short", 1.5)
    return
  end

  local wd = mp.get_property("working-directory")
  if wd == nil or wd == "" then
    mp.osd_message("Clip failed (no wd)", 1.5)
    return
  end
  local dir = utils.join_path(wd, "clips")
  mkdir_p(dir)

  local base = basename(p)
  local ext = string.match(base, "%.[^%.]+$") or ".mp4"
  base = string.gsub(base, "%.[^%.]+$", "")
  if base == "" then base = "clip" end

  local start_tag = format_ts(clip_start_pos)
  local end_tag = format_ts(pos)
  local out, name = unique_clip_path(dir, base, start_tag, end_tag, ext)
  local input = resolve_local_path(p)

  local args = {
    "ffmpeg",
    "-hide_banner",
    "-loglevel", "error",
    "-ss", tostring(clip_start_pos),
    "-t", tostring(pos - clip_start_pos),
    "-i", input,
    "-c", "copy",
    "-map", "0",
    "-avoid_negative_ts", "make_zero",
    out,
  }

  mp.osd_message("Saving clip...", 1.5)
  mp.command_native_async({ name = "subprocess", args = args }, function(success, res, err)
    if not success or (res and res.status and res.status ~= 0) or err ~= nil then
      mp.osd_message("Clip failed", 2.0)
      return
    end
    mp.osd_message("Saved: " .. name, 1.5)
  end)
end

mp.register_script_message("pp_clip_toggle", clip_toggle)

local trim_active = false
local trim_start_pos = 0
local trim_start_path = ""

local function trim_toggle()
  local p = mp.get_property("path") or ""
  if p == "" then
    mp.osd_message("Trim failed (no file)", 1.5)
    return
  end
  if not is_probably_local_file(p) then
    mp.osd_message("Trim failed (not local)", 1.5)
    return
  end
  local pos = mp.get_property_number("time-pos", 0) or 0
  if pos < 0 then pos = 0 end

  if not trim_active then
    trim_active = true
    trim_start_pos = pos
    trim_start_path = p
    mp.osd_message("Trim start", 1.5)
    return
  end

  if p ~= trim_start_path then
    trim_active = false
    mp.osd_message("Trim canceled (file changed)", 1.5)
    return
  end
  if pos < trim_start_pos then
    mp.osd_message("Trim invalid (end before start)", 1.5)
    return
  end
  if pos <= trim_start_pos + 0.05 then
    mp.osd_message("Trim too short", 1.5)
    return
  end

  trim_active = false

  local wd = mp.get_property("working-directory")
  if wd == nil or wd == "" then
    mp.osd_message("Trim failed (no wd)", 1.5)
    return
  end
  local dir = utils.join_path(wd, "clips")
  mkdir_p(dir)

  local base = basename(p)
  local ext = string.match(base, "%.[^%.]+$") or ".mp4"
  base = string.gsub(base, "%.[^%.]+$", "")
  if base == "" then base = "trim" end

  local start_tag = format_ts(trim_start_pos)
  local end_tag = format_ts(pos)
  local out, name = unique_clip_path(dir, base, start_tag, end_tag, ext)
  local input = resolve_local_path(p)

  local args = {
    "ffmpeg",
    "-hide_banner",
    "-loglevel", "error",
    "-ss", tostring(trim_start_pos),
    "-t", tostring(pos - trim_start_pos),
    "-i", input,
    "-c", "copy",
    "-map", "0",
    "-avoid_negative_ts", "make_zero",
    out,
  }

  mp.osd_message("Saving trim...", 1.5)
  mp.command_native_async({ name = "subprocess", args = args }, function(success, res, err)
    if not success or (res and res.status and res.status ~= 0) or err ~= nil then
      mp.osd_message("Trim failed", 2.0)
      return
    end
    mp.osd_message("Saved: " .. name, 1.5)
  end)
end

mp.register_script_message("pp_trim_toggle", trim_toggle)

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
