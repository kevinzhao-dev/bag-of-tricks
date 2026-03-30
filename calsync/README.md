# calsync

One-way sync from Apple Calendar to Google Calendar on macOS.

Google Calendar acts as the single view that aggregates events from multiple sources. calsync handles the Apple Calendar layer — it reads events locally via EventKit, pushes them to a designated Google Calendar, and never touches events it didn't create.

## How it works

- Reads Apple Calendar events using [maccal](https://github.com/appenz/maccal) (PyObjC + EventKit)
- Writes to Google Calendar using [gws](https://github.com/googleworkspace/cli) CLI (subprocess)
- Tracks what's been synced in `~/.calsync/sync_state.json`
- Tags every created Google event with `extendedProperties.private.calsync_source=apple` so it only ever modifies its own events
- Detects changes via a SHA-256 fingerprint of event content (title, time, location, notes)
- Runs as a launchd daemon for automatic background sync

## Prerequisites

1. **macOS** — EventKit is macOS-only
2. **Python 3.11+**
3. **gws CLI** — `brew install googleworkspace-cli`, then `gws auth setup` and `gws auth login`
4. **Calendar permission** — System Settings > Privacy & Security > Calendar > Terminal (or your terminal app)

## Install

```bash
cd calsync
pip install -e .
```

> **Note:** If you have the `eventkit` PyPI package installed (e.g. via `ib-insync`), it will shadow Apple's EventKit framework. Fix with:
> ```bash
> pip install --force-reinstall --no-deps pyobjc-framework-EventKit
> ```

## Configuration

Create `~/.calsync/config.toml`:

```toml
[default]
sync_days = 30
timezone = "Asia/Taipei"
interval_minutes = 10

[[mappings]]
apple_calendar = "My Calendar"
google_calendar = "primary"
```

- `apple_calendar` — exact name as shown in Apple Calendar
- `google_calendar` — Google Calendar ID (`"primary"` for your main calendar, or a full ID like `abc123@group.calendar.google.com`)
- Multiple `[[mappings]]` sections are supported

If no config file exists, calsync defaults to syncing `Calendar` to `primary`.

## Usage

```bash
# List your Apple Calendars
calsync list-calendars

# Preview events from a specific calendar
calsync list-events --calendar "My Calendar" --days 14

# Dry run — see what would be synced without making changes
calsync sync --dry-run

# Run the sync
calsync sync

# Sync a specific date range
calsync sync --start 2026-04-01 --end 2026-04-30

# Check sync status
calsync status
```

### Background daemon

```bash
# Install launchd daemon (syncs every 10 minutes)
calsync install --interval 10

# Check if daemon is running
calsync status --daemon

# Stop and remove daemon
calsync uninstall
```

### Recovery and cleanup

```bash
# Rebuild sync state from Google Calendar (if sync_state.json is lost)
calsync rebuild

# Delete all calsync-created events from Google Calendar
calsync purge --target "your-calendar-id"
```

## Safety guarantees

- **Source isolation** — calsync only reads, updates, or deletes Google Calendar events that have its `calsync_source=apple` tag. Your manually created events and other calendar subscriptions are never touched.
- **Idempotent** — running sync multiple times produces the same result. Events already synced are skipped.
- **Crash-safe** — sync state is written after each successful operation using atomic file replacement, so a crash mid-sync won't corrupt state.
- **Recoverable** — if `sync_state.json` is lost, `calsync rebuild` reconstructs it from the tags on Google Calendar events.

## Project structure

```
src/calsync/
  cli.py       — Click CLI entry point and launchd management
  apple.py     — Apple Calendar reader (maccal wrapper)
  google.py    — Google Calendar writer (gws subprocess wrapper)
  sync.py      — Sync engine (diff, dedup, execute)
  models.py    — Event dataclass, fingerprint, sync key
  config.py    — Config file loading (~/.calsync/config.toml)
  state.py     — Sync state persistence (~/.calsync/sync_state.json)
```

## Logs

Logs are written to `~/.calsync/calsync.log` (auto-rotated, 5 MB, 3 backups). Use `--verbose` for debug output or `--quiet` to suppress console output.
