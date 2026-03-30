from __future__ import annotations

import json
import logging
import shutil
import subprocess
from datetime import datetime, timedelta
from logging.handlers import RotatingFileHandler
from pathlib import Path
from textwrap import dedent

import click

from .config import CALSYNC_DIR, LOG_PATH, ensure_dirs, load_config
from .models import SyncAction, fingerprint, sync_key

logger = logging.getLogger("calsync")

PLIST_LABEL = "com.calsync.daemon"
PLIST_PATH = Path.home() / "Library" / "LaunchAgents" / f"{PLIST_LABEL}.plist"


def setup_logging(verbose: bool = False, quiet: bool = False) -> None:
    ensure_dirs()
    root = logging.getLogger("calsync")
    root.setLevel(logging.DEBUG)

    file_handler = RotatingFileHandler(
        LOG_PATH, maxBytes=5 * 1024 * 1024, backupCount=3,
    )
    file_handler.setLevel(logging.DEBUG)
    file_handler.setFormatter(logging.Formatter(
        "%(asctime)s [%(levelname)s] %(name)s: %(message)s"
    ))
    root.addHandler(file_handler)

    if not quiet:
        console = logging.StreamHandler()
        console.setLevel(logging.DEBUG if verbose else logging.INFO)
        console.setFormatter(logging.Formatter("%(message)s"))
        root.addHandler(console)


@click.group()
@click.option("--verbose", "-v", is_flag=True, help="Show debug output")
@click.option("--quiet", "-q", is_flag=True, help="Only show errors")
def cli(verbose: bool, quiet: bool) -> None:
    """calsync — Apple Calendar → Google Calendar sync."""
    setup_logging(verbose=verbose, quiet=quiet)


@cli.command("list-calendars")
@click.option("--json-output", "--json", "use_json", is_flag=True, help="Output as JSON")
def list_calendars(use_json: bool) -> None:
    """List all Apple Calendars."""
    from .apple import get_apple_calendars

    calendars = get_apple_calendars()

    if use_json:
        click.echo(json.dumps(calendars, indent=2, ensure_ascii=False))
        return

    if not calendars:
        click.echo("No calendars found.")
        return

    click.echo(f"{'Name':<30} {'Type':<15} {'Source':<20}")
    click.echo("-" * 65)
    for cal in calendars:
        click.echo(f"{cal['title']:<30} {cal['type']:<15} {cal['source']:<20}")


@cli.command("list-events")
@click.option("--calendar", "-c", required=True, help="Apple Calendar name")
@click.option("--days", "-d", default=7, help="Number of days to look ahead")
@click.option("--start", "start_date", type=click.DateTime(), default=None)
@click.option("--end", "end_date", type=click.DateTime(), default=None)
@click.option("--json-output", "--json", "use_json", is_flag=True)
def list_events(
    calendar: str,
    days: int,
    start_date: datetime | None,
    end_date: datetime | None,
    use_json: bool,
) -> None:
    """List events from an Apple Calendar."""
    from .apple import get_apple_events

    start = start_date or datetime.now()
    end = end_date or (start + timedelta(days=days))

    events = get_apple_events(calendar, start, end)

    if use_json:
        data = [
            {
                "title": e.title,
                "start": e.start.isoformat(),
                "end": e.end.isoformat(),
                "location": e.location,
                "all_day": e.is_all_day,
            }
            for e in events
        ]
        click.echo(json.dumps(data, indent=2, ensure_ascii=False))
        return

    if not events:
        click.echo("No events found.")
        return

    click.echo(f"{'Start':<20} {'End':<20} {'Title':<30} {'Location':<20}")
    click.echo("-" * 90)
    for e in events:
        start_str = e.start.strftime("%Y-%m-%d %H:%M") if not e.is_all_day else e.start.strftime("%Y-%m-%d (all day)")
        end_str = e.end.strftime("%H:%M") if not e.is_all_day else ""
        loc = (e.location or "")[:20]
        click.echo(f"{start_str:<20} {end_str:<20} {e.title:<30} {loc:<20}")


@cli.command()
@click.option("--dry-run", is_flag=True, help="Preview without making changes")
@click.option("--start", "start_date", type=click.DateTime(), default=None)
@click.option("--end", "end_date", type=click.DateTime(), default=None)
@click.option("--quiet", "-q", is_flag=True)
def sync(
    dry_run: bool,
    start_date: datetime | None,
    end_date: datetime | None,
    quiet: bool,
) -> None:
    """Sync Apple Calendar events to Google Calendar."""
    from .apple import get_apple_events
    from .google import check_gws_available
    from .state import SyncState
    from .sync import compute_sync_plan, execute_sync_plan

    check_gws_available()
    config = load_config()
    state = SyncState.load()

    start = start_date or datetime.now()
    end = end_date or (start + timedelta(days=config.sync_days))

    for mapping in config.mappings:
        if not quiet:
            click.echo(f"\nSyncing: {mapping.apple_calendar} → {mapping.google_calendar}")

        events = get_apple_events(mapping.apple_calendar, start, end)
        plan = compute_sync_plan(events, state)

        if dry_run:
            inserts = sum(1 for a, _, _ in plan if a == SyncAction.INSERT)
            patches = sum(1 for a, _, _ in plan if a == SyncAction.PATCH)
            skips = sum(1 for a, _, _ in plan if a == SyncAction.SKIP)
            orphans = sum(1 for a, _, _ in plan if a == SyncAction.ORPHAN)
            click.echo(f"  [DRY-RUN] Insert: {inserts}, Patch: {patches}, Skip: {skips}, Orphan: {orphans}")
            for action, event, meta in plan:
                if action in (SyncAction.INSERT, SyncAction.PATCH):
                    click.echo(f"    {action.value.upper()}: {event.title} ({event.start.strftime('%Y-%m-%d %H:%M')})")
            continue

        result = execute_sync_plan(plan, mapping.google_calendar, state, dry_run=False)

        if not quiet:
            click.echo(
                f"  Done — Inserted: {result.inserted}, Patched: {result.patched}, "
                f"Skipped: {result.skipped}, Orphans: {result.orphaned}, Errors: {result.errors}"
            )


@cli.command()
@click.option("--daemon", is_flag=True, help="Also show launchd daemon status")
def status(daemon: bool) -> None:
    """Show sync status."""
    from .state import SyncState

    state = SyncState.load()
    click.echo(f"Synced events: {state.count}")
    click.echo(f"Last sync: {state.last_sync_time or 'never'}")

    config = load_config()
    click.echo(f"Mappings: {len(config.mappings)}")
    for m in config.mappings:
        click.echo(f"  {m.apple_calendar} → {m.google_calendar}")

    if daemon:
        try:
            result = subprocess.run(
                ["launchctl", "list", PLIST_LABEL],
                capture_output=True, text=True,
            )
            if result.returncode == 0:
                click.echo(f"\nDaemon: running")
                click.echo(result.stdout)
            else:
                click.echo(f"\nDaemon: not loaded")
        except Exception:
            click.echo(f"\nDaemon: unable to check")


@cli.command()
@click.option("--target", default="primary", help="Google Calendar ID to purge")
@click.option("--dry-run", is_flag=True)
@click.confirmation_option(prompt="This will delete ALL calsync events from Google Calendar. Continue?")
def purge(target: str, dry_run: bool) -> None:
    """Remove all calsync-created events from Google Calendar."""
    from .google import check_gws_available, delete_event, list_synced_events
    from .state import SyncState

    check_gws_available()

    now = datetime.now()
    time_min = (now - timedelta(days=365)).isoformat() + "Z"
    time_max = (now + timedelta(days=365)).isoformat() + "Z"

    events = list_synced_events(target, time_min, time_max)
    click.echo(f"Found {len(events)} calsync events to purge")

    if dry_run:
        for e in events:
            click.echo(f"  Would delete: {e.get('summary', '(no title)')}")
        return

    deleted = 0
    for e in events:
        try:
            delete_event(target, e["id"])
            deleted += 1
        except Exception as err:
            logger.error("Failed to delete %s: %s", e["id"], err)

    state = SyncState.load()
    state.clear()
    state.save()
    click.echo(f"Purged {deleted} events, sync state cleared")


@cli.command()
@click.option("--target", default="primary", help="Google Calendar ID")
def rebuild(target: str) -> None:
    """Rebuild sync state from Google Calendar extendedProperties."""
    from .google import check_gws_available, list_synced_events
    from .state import SyncState

    check_gws_available()

    now = datetime.now()
    time_min = (now - timedelta(days=365)).isoformat() + "Z"
    time_max = (now + timedelta(days=365)).isoformat() + "Z"

    events = list_synced_events(target, time_min, time_max)
    state = SyncState()

    for e in events:
        props = e.get("extendedProperties", {}).get("private", {})
        apple_id = props.get("calsync_apple_id")
        apple_start = props.get("calsync_apple_start", "")
        fp = props.get("calsync_fingerprint", "")

        if apple_id:
            key = f"{apple_id}::{apple_start}" if apple_start else apple_id
            state.upsert(key, e["id"], fp)

    state.save()
    click.echo(f"Rebuilt sync state: {state.count} events recovered")


@cli.command()
@click.option("--interval", default=10, help="Sync interval in minutes")
def install(interval: int) -> None:
    """Install launchd daemon for background sync."""
    import sys

    calsync_path = shutil.which("calsync") or sys.executable
    if calsync_path == sys.executable:
        calsync_cmd = f"{sys.executable} -m calsync"
    else:
        calsync_cmd = calsync_path

    import os

    current_path = os.environ.get("PATH", "/usr/bin:/bin")

    plist_content = dedent(f"""\
        <?xml version="1.0" encoding="UTF-8"?>
        <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
          "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
        <plist version="1.0">
        <dict>
            <key>Label</key>
            <string>{PLIST_LABEL}</string>
            <key>EnvironmentVariables</key>
            <dict>
                <key>PATH</key>
                <string>{current_path}</string>
            </dict>
            <key>ProgramArguments</key>
            <array>
                <string>{calsync_cmd}</string>
                <string>sync</string>
                <string>--quiet</string>
            </array>
            <key>StartInterval</key>
            <integer>{interval * 60}</integer>
            <key>RunAtLoad</key>
            <true/>
            <key>StandardErrorPath</key>
            <string>{LOG_PATH}</string>
        </dict>
        </plist>
    """)

    PLIST_PATH.parent.mkdir(parents=True, exist_ok=True)
    PLIST_PATH.write_text(plist_content)

    subprocess.run(["launchctl", "load", str(PLIST_PATH)], check=True)
    click.echo(f"Daemon installed: syncing every {interval} minutes")
    click.echo(f"Plist: {PLIST_PATH}")


@cli.command()
def uninstall() -> None:
    """Remove launchd daemon."""
    if PLIST_PATH.exists():
        subprocess.run(["launchctl", "unload", str(PLIST_PATH)], check=False)
        PLIST_PATH.unlink()
        click.echo("Daemon removed")
    else:
        click.echo("No daemon installed")


