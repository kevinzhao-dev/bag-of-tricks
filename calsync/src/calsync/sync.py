from __future__ import annotations

import logging

from . import google
from .models import CalSyncEvent, SyncAction, SyncResult, fingerprint, sync_key
from .state import SyncState

logger = logging.getLogger(__name__)


def compute_sync_plan(
    apple_events: list[CalSyncEvent],
    state: SyncState,
) -> list[tuple[SyncAction, CalSyncEvent, dict]]:
    plan: list[tuple[SyncAction, CalSyncEvent, dict]] = []
    seen_keys: set[str] = set()

    for event in apple_events:
        key = sync_key(event)
        seen_keys.add(key)
        fp = fingerprint(event)
        record = state.get(key)

        if record is None:
            plan.append((SyncAction.INSERT, event, {"fingerprint": fp}))
        elif record.fingerprint != fp:
            plan.append((SyncAction.PATCH, event, {
                "fingerprint": fp,
                "google_event_id": record.google_event_id,
            }))
        else:
            plan.append((SyncAction.SKIP, event, {}))

    for key in state.all_keys() - seen_keys:
        record = state.get(key)
        if record:
            plan.append((SyncAction.ORPHAN, None, {
                "key": key,
                "google_event_id": record.google_event_id,
            }))

    return plan


def execute_sync_plan(
    plan: list[tuple[SyncAction, CalSyncEvent | None, dict]],
    calendar_id: str,
    state: SyncState,
    dry_run: bool = False,
) -> SyncResult:
    result = SyncResult()

    for action, event, meta in plan:
        if action == SyncAction.SKIP:
            result.skipped += 1
            continue

        if action == SyncAction.ORPHAN:
            result.orphaned += 1
            logger.warning("Orphan detected: %s", meta.get("key"))
            continue

        if action == SyncAction.INSERT:
            if dry_run:
                logger.info("[DRY-RUN] Would insert: %s", event.title)
                result.inserted += 1
                continue
            try:
                google_id = google.insert_event(
                    calendar_id, event, meta["fingerprint"],
                )
                if google_id:
                    state.upsert(sync_key(event), google_id, meta["fingerprint"])
                    state.save()
                result.inserted += 1
            except Exception as e:
                logger.error("Failed to insert '%s': %s", event.title, e)
                result.errors += 1

        elif action == SyncAction.PATCH:
            if dry_run:
                logger.info("[DRY-RUN] Would patch: %s", event.title)
                result.patched += 1
                continue
            try:
                google.patch_event(
                    calendar_id,
                    meta["google_event_id"],
                    event,
                    meta["fingerprint"],
                )
                state.upsert(sync_key(event), meta["google_event_id"], meta["fingerprint"])
                state.save()
                result.patched += 1
            except Exception as e:
                logger.error("Failed to patch '%s': %s", event.title, e)
                result.errors += 1

    return result
