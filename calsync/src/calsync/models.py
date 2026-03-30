from __future__ import annotations

import hashlib
from dataclasses import dataclass
from datetime import datetime
from enum import Enum


@dataclass(frozen=True)
class CalSyncEvent:
    apple_id: str
    title: str
    start: datetime
    end: datetime
    is_all_day: bool
    location: str | None = None
    notes: str | None = None
    url: str | None = None
    calendar_name: str = ""
    timezone: str | None = None


class SyncAction(Enum):
    INSERT = "insert"
    PATCH = "patch"
    SKIP = "skip"
    ORPHAN = "orphan"


@dataclass
class SyncResult:
    inserted: int = 0
    patched: int = 0
    skipped: int = 0
    orphaned: int = 0
    errors: int = 0


def fingerprint(event: CalSyncEvent) -> str:
    parts = [
        event.title or "",
        event.start.isoformat(),
        event.end.isoformat(),
        event.location or "",
        event.notes or "",
    ]
    return hashlib.sha256("|".join(parts).encode()).hexdigest()


def sync_key(event: CalSyncEvent) -> str:
    """Composite key for dedup — handles recurring events with same apple_id."""
    return f"{event.apple_id}::{event.start.isoformat()}"
