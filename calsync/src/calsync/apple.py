from __future__ import annotations

import logging
from datetime import datetime

import click

from .models import CalSyncEvent

logger = logging.getLogger(__name__)


def _get_store():
    try:
        import EventKit as _ek
        if not hasattr(_ek, "EKEventStore"):
            raise click.ClickException(
                "The 'eventkit' PyPI package is shadowing Apple's EventKit framework. "
                "Fix with: pip install --force-reinstall --no-deps pyobjc-framework-EventKit"
            )
        import maccal
        return maccal.CalendarStore()
    except ImportError:
        raise click.ClickException(
            "maccal is not installed. Run: pip install maccal"
        )
    except Exception as e:
        if "denied" in str(e).lower() or "access" in str(e).lower():
            raise click.ClickException(
                "Calendar access denied. Go to System Settings → Privacy & Security → Calendar and enable access for Terminal."
            )
        raise


def get_apple_calendars() -> list[dict]:
    store = _get_store()
    calendars = store.list_calendars()
    return [
        {
            "title": cal.title,
            "type": str(cal.type) if hasattr(cal, "type") else "unknown",
            "color": getattr(cal, "color", ""),
            "source": getattr(cal, "source", ""),
        }
        for cal in calendars
    ]


def _to_calsync_event(event) -> CalSyncEvent:
    return CalSyncEvent(
        apple_id=event.event_id,
        title=event.title or "",
        start=event.start,
        end=event.end,
        is_all_day=event.is_all_day,
        location=getattr(event, "location", None),
        notes=getattr(event, "notes", None),
        url=str(event.url) if getattr(event, "url", None) else None,
        calendar_name=str(event.calendar) if hasattr(event, "calendar") else "",
        timezone=getattr(event, "time_zone", None),
    )


def get_apple_events(
    calendar_name: str,
    start: datetime,
    end: datetime,
) -> list[CalSyncEvent]:
    store = _get_store()
    events = store.get_events(start, end, calendars=[calendar_name])
    result = [_to_calsync_event(e) for e in events]
    result.sort(key=lambda e: e.start)
    logger.info("Read %d events from Apple Calendar '%s'", len(result), calendar_name)
    return result
