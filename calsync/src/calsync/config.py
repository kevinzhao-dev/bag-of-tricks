from __future__ import annotations

import tomllib
from dataclasses import dataclass, field
from pathlib import Path

CALSYNC_DIR = Path.home() / ".calsync"
CONFIG_PATH = CALSYNC_DIR / "config.toml"
STATE_PATH = CALSYNC_DIR / "sync_state.json"
LOG_PATH = CALSYNC_DIR / "calsync.log"


@dataclass
class CalendarMapping:
    apple_calendar: str
    google_calendar: str


@dataclass
class CalSyncConfig:
    sync_days: int = 30
    timezone: str = "Asia/Taipei"
    interval_minutes: int = 10
    mappings: list[CalendarMapping] = field(default_factory=list)


def ensure_dirs() -> None:
    CALSYNC_DIR.mkdir(parents=True, exist_ok=True)


def load_config() -> CalSyncConfig:
    if not CONFIG_PATH.exists():
        return CalSyncConfig(
            mappings=[CalendarMapping(apple_calendar="Calendar", google_calendar="primary")]
        )

    with open(CONFIG_PATH, "rb") as f:
        data = tomllib.load(f)

    default = data.get("default", {})
    mappings = [
        CalendarMapping(
            apple_calendar=m["apple_calendar"],
            google_calendar=m["google_calendar"],
        )
        for m in data.get("mappings", [])
    ]

    if not mappings:
        mappings = [CalendarMapping(apple_calendar="Calendar", google_calendar="primary")]

    return CalSyncConfig(
        sync_days=default.get("sync_days", 30),
        timezone=default.get("timezone", "Asia/Taipei"),
        interval_minutes=default.get("interval_minutes", 10),
        mappings=mappings,
    )


def init_config() -> None:
    ensure_dirs()
    if CONFIG_PATH.exists():
        return

    CONFIG_PATH.write_text(
        """\
# ~/.calsync/config.toml

[default]
sync_days = 30
timezone = "Asia/Taipei"
interval_minutes = 10

[[mappings]]
apple_calendar = "Calendar"
google_calendar = "primary"
"""
    )
