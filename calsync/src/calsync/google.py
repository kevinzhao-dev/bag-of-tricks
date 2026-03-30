from __future__ import annotations

import json
import logging
import shutil
import subprocess
from datetime import datetime

from .models import CalSyncEvent

logger = logging.getLogger(__name__)


class GWSError(Exception):
    pass


def _run_gws(args: list[str], timeout: int = 30) -> dict | list | None:
    cmd = ["gws", "calendar"] + args
    logger.debug("Running: %s", " ".join(cmd))

    result = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        timeout=timeout,
    )

    if result.returncode != 0:
        raise GWSError(f"gws failed (exit {result.returncode}): {result.stderr.strip()}")

    stdout = result.stdout.strip()
    if not stdout:
        return None

    try:
        return json.loads(stdout)
    except json.JSONDecodeError as e:
        raise GWSError(f"Failed to parse gws JSON output: {e}\nOutput: {stdout[:500]}")


def check_gws_available() -> None:
    if not shutil.which("gws"):
        raise GWSError(
            "gws CLI not found. Install with: brew install googleworkspace-cli"
        )
    try:
        _run_gws(["calendarList", "list", "--params", json.dumps({"maxResults": 1})])
    except GWSError as e:
        raise GWSError(
            f"gws authentication failed. Run: gws auth login\nDetails: {e}"
        )


def _build_event_body(event: CalSyncEvent, fp: str) -> dict:
    body: dict = {
        "summary": event.title,
        "extendedProperties": {
            "private": {
                "calsync_source": "apple",
                "calsync_apple_id": event.apple_id,
                "calsync_apple_start": event.start.isoformat(),
                "calsync_fingerprint": fp,
            }
        },
    }

    if event.is_all_day:
        body["start"] = {"date": event.start.strftime("%Y-%m-%d")}
        body["end"] = {"date": event.end.strftime("%Y-%m-%d")}
    else:
        tz = event.timezone or "Asia/Taipei"
        body["start"] = {"dateTime": event.start.isoformat(), "timeZone": tz}
        body["end"] = {"dateTime": event.end.isoformat(), "timeZone": tz}

    if event.location:
        body["location"] = event.location

    description_parts = []
    if event.notes:
        description_parts.append(event.notes)
    if event.url:
        description_parts.append(f"\n{event.url}")
    if description_parts:
        body["description"] = "\n".join(description_parts)

    return body


def list_synced_events(
    calendar_id: str,
    time_min: str,
    time_max: str,
) -> list[dict]:
    params = {
        "calendarId": calendar_id,
        "timeMin": time_min,
        "timeMax": time_max,
        "singleEvents": True,
        "privateExtendedProperty": "calsync_source=apple",
        "maxResults": 2500,
    }

    result = _run_gws([
        "events", "list",
        "--params", json.dumps(params),
    ])

    if result and isinstance(result, dict):
        return result.get("items", [])
    return []


def insert_event(
    calendar_id: str,
    event: CalSyncEvent,
    fp: str,
    dry_run: bool = False,
) -> str | None:
    body = _build_event_body(event, fp)

    args = [
        "events", "insert",
        "--params", json.dumps({"calendarId": calendar_id}),
        "--json", json.dumps(body),
    ]
    if dry_run:
        args.append("--dry-run")

    result = _run_gws(args, timeout=60)

    if dry_run:
        return None

    if result and isinstance(result, dict):
        event_id = result.get("id")
        logger.info("Inserted event '%s' → Google ID: %s", event.title, event_id)
        return event_id

    raise GWSError(f"No event ID returned from insert for '{event.title}'")


def patch_event(
    calendar_id: str,
    google_event_id: str,
    event: CalSyncEvent,
    fp: str,
    dry_run: bool = False,
) -> bool:
    body = _build_event_body(event, fp)

    args = [
        "events", "patch",
        "--params", json.dumps({
            "calendarId": calendar_id,
            "eventId": google_event_id,
        }),
        "--json", json.dumps(body),
    ]
    if dry_run:
        args.append("--dry-run")

    _run_gws(args, timeout=60)
    logger.info("Patched event '%s' (Google ID: %s)", event.title, google_event_id)
    return True


def delete_event(
    calendar_id: str,
    google_event_id: str,
    dry_run: bool = False,
) -> bool:
    args = [
        "events", "delete",
        "--params", json.dumps({
            "calendarId": calendar_id,
            "eventId": google_event_id,
        }),
    ]
    if dry_run:
        args.append("--dry-run")

    _run_gws(args, timeout=60)
    logger.info("Deleted Google event: %s", google_event_id)
    return True
