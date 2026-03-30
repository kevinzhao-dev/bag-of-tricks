from __future__ import annotations

import json
import logging
import os
import tempfile
from dataclasses import asdict, dataclass
from datetime import datetime, timezone

from .config import STATE_PATH, ensure_dirs

logger = logging.getLogger(__name__)


@dataclass
class SyncRecord:
    google_event_id: str
    fingerprint: str
    last_synced: str


class SyncState:
    def __init__(self, data: dict[str, SyncRecord] | None = None) -> None:
        self._data: dict[str, SyncRecord] = data or {}

    @classmethod
    def load(cls) -> SyncState:
        if not STATE_PATH.exists():
            logger.info("No sync state file found, starting fresh")
            return cls()

        try:
            raw = json.loads(STATE_PATH.read_text())
            data = {
                key: SyncRecord(**val)
                for key, val in raw.items()
            }
            return cls(data)
        except (json.JSONDecodeError, TypeError, KeyError) as e:
            logger.warning("Corrupt sync state file, starting fresh: %s", e)
            return cls()

    def save(self) -> None:
        ensure_dirs()
        raw = {key: asdict(rec) for key, rec in self._data.items()}
        content = json.dumps(raw, indent=2, ensure_ascii=False)

        fd, tmp_path = tempfile.mkstemp(dir=STATE_PATH.parent, suffix=".tmp")
        try:
            with os.fdopen(fd, "w") as f:
                f.write(content)
            os.replace(tmp_path, STATE_PATH)
        except BaseException:
            os.unlink(tmp_path)
            raise

    def get(self, key: str) -> SyncRecord | None:
        return self._data.get(key)

    def upsert(self, key: str, google_event_id: str, fp: str) -> None:
        self._data[key] = SyncRecord(
            google_event_id=google_event_id,
            fingerprint=fp,
            last_synced=datetime.now(timezone.utc).isoformat(),
        )

    def remove(self, key: str) -> None:
        self._data.pop(key, None)

    def all_keys(self) -> set[str]:
        return set(self._data.keys())

    @property
    def count(self) -> int:
        return len(self._data)

    @property
    def last_sync_time(self) -> str | None:
        if not self._data:
            return None
        return max(rec.last_synced for rec in self._data.values())

    def clear(self) -> None:
        self._data.clear()
