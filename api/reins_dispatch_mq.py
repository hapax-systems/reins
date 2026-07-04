"""reins_dispatch_mq — the methodology-dispatch MQ producer (the REAL apply transport).

The dispatch verb's `submit_dispatch` enqueues a ``DispatchIntent`` to the methodology-dispatch relay MQ
(a sqlite db shared with ``hapax-methodology-dispatch``). This module re-implements the small producer
contract from the upstream coordinator's ``relay_mq.send_message`` so reins stays DECOUPLED (it imports the
hapax-spine wheel for the SDLC-runtime mechanism, NOT the coordinator's relay substrate). The contract is stable
+ fully specified; the follow-up is to canonize ``relay_mq`` in hapax-spine so both consume one SSOT.

SPAWN BOUNDARY (load-bearing): this producer is a PURE SQLITE INSERT. It does NOT spawn a process, call
HTTP, or invoke a subprocess. The lane-launch spawn lives downstream in ``hapax-methodology-dispatch
--launch`` (invoked by the coordinator's tick on a matching cc-task, or an explicit call) — outside
reins's apply boundary. reins's REAL apply = compose + verify + ENQUEUE (a real governed write); the
launch is the daemon's correctness on the operator fleet. So ``applied=True`` is honest at enqueue
(reins did a real write), and the witness-echo (U7) later flips ``witness: pending -> echoed`` when the
``coord_dispatch.launch_*`` event confirms the downstream launch.

MQ medium: sqlite at ``$HAPAX_RELAY_MQ_DB`` (default ``~/.cache/hapax/relay/messages.db``). Two tables:
``messages`` (the envelope) + ``recipients`` (per-recipient state, starts ``offered``). Schema is
``CREATE TABLE IF NOT EXISTS`` (idempotent) so reins can write to a fresh or an existing MQ db.
"""
from __future__ import annotations

import hashlib
import json
import os
import sqlite3
import time
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any

# P0 freshness (mirrors the upstream relay_mq_envelope.PRIORITY_FRESHNESS[0]).
_P0_STALE_AFTER_S = 600
_P0_EXPIRES_AFTER_S = 3600


_SCHEMA_SQL = [
    """
    CREATE TABLE IF NOT EXISTS messages (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        message_id TEXT UNIQUE NOT NULL,
        version INTEGER NOT NULL DEFAULT 1,
        sender TEXT NOT NULL,
        message_type TEXT NOT NULL
            CHECK (message_type IN ('dispatch', 'advisory', 'escalation', 'query')),
        priority INTEGER NOT NULL DEFAULT 2
            CHECK (priority BETWEEN 0 AND 3),
        subject TEXT NOT NULL,
        authority_case TEXT,
        authority_item TEXT,
        parent_message_id TEXT,
        recipients_spec TEXT NOT NULL,
        payload TEXT,
        payload_path TEXT,
        payload_hash TEXT NOT NULL,
        created_at TEXT NOT NULL,
        expires_at TEXT,
        stale_after TEXT,
        tags TEXT,
        CHECK (message_type != 'dispatch' OR authority_case IS NOT NULL),
        CHECK ((payload IS NOT NULL AND payload_path IS NULL)
               OR (payload IS NULL AND payload_path IS NOT NULL))
    )
    """,
    """
    CREATE TABLE IF NOT EXISTS recipients (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        message_id TEXT NOT NULL REFERENCES messages(message_id),
        recipient TEXT NOT NULL,
        state TEXT NOT NULL DEFAULT 'offered'
            CHECK (state IN ('offered','read','accepted','processed','deferred','escalated')),
        reason TEXT,
        retry_count INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        UNIQUE(message_id, recipient)
    )
    """,
]


def default_mq_db_path() -> str:
    """The relay MQ sqlite path: ``$HAPAX_RELAY_MQ_DB`` (default ``~/.cache/hapax/relay/messages.db``).

    Overridable so tests inject a temp db (no live MQ, no spawn). Mirrors the dispatcher's
    ``_relay_mq_db_path`` (hapax-methodology-dispatch:831) so reins writes the db the daemon reads."""
    env = os.environ.get("HAPAX_RELAY_MQ_DB", "").strip()
    if env:
        return os.path.expanduser(env)
    return str(Path.home() / ".cache" / "hapax" / "relay" / "messages.db")


def _uuid7() -> str:
    """A UUIDv7 string (time-ordered, lexicographically sortable). Python 3.12 lacks uuid.uuid7().

    Verbatim from the upstream relay_mq_envelope._uuid7 so reins-minted ids are
    indistinguishable from coordinator-minted ones (the dispatcher accepts either)."""
    timestamp_ms = int(time.time() * 1000)
    rand_a = int.from_bytes(os.urandom(2), "big") & 0x0FFF
    rand_b = int.from_bytes(os.urandom(8), "big") & ((1 << 62) - 1)
    uuid_int = (timestamp_ms & 0xFFFFFFFFFFFF) << 80
    uuid_int |= 0x7 << 76
    uuid_int |= rand_a << 64
    uuid_int |= 0x2 << 62
    uuid_int |= rand_b
    return str(uuid.UUID(int=uuid_int))


def _iso(epoch_s: float) -> str:
    from datetime import datetime, timezone

    return datetime.fromtimestamp(epoch_s, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _normalize_lane(lane: str) -> str:
    """The recipient normalization the dispatcher expects (relay_mq:127): lower + dashes."""
    return lane.strip().lower().replace("_", "-")


def _ensure_schema(conn: sqlite3.Connection) -> None:
    for stmt in _SCHEMA_SQL:
        conn.execute(stmt)


@dataclass
class DispatchIntent:
    """The cockpit-owned dispatch intent (mirrors api/reins_command.DispatchIntent). Kept here too so
    the producer module is self-contained for testing without importing the FastAPI command layer."""

    task_id: str
    lane: str
    platform: str
    mode: str
    profile: str
    authority_case: str
    parent_spec: str | None
    message_id: str
    idempotency_key: str | None = None


def send_dispatch_message(req: Any, db_path: str | None = None) -> str:
    """Enqueue a DispatchIntent as a ``dispatch`` envelope in the relay MQ. PURE SQLITE INSERT (no spawn).

    Returns the envelope's ``message_id`` (the receipt_id reins records). ``db_path`` injects a temp db
    for tests; production resolves ``default_mq_db_path()``. Field-for-field mirrors the coordinator's
    ``send_message`` call (the upstream coordinator's send path) so the dispatcher consumes it."""
    db = Path(db_path or default_mq_db_path())
    db.parent.mkdir(parents=True, exist_ok=True)
    payload = json.dumps(
        {
            "kind": "reins_dispatch",
            "task_id": req.task_id,
            "lane": req.lane,
            "platform": req.platform,
            "mode": req.mode,
            "profile": req.profile,
            "parent_spec": req.parent_spec,
            "idempotency_key": req.idempotency_key,
        },
        sort_keys=True,
    )
    message_id = req.message_id or _uuid7()
    recipient = _normalize_lane(req.lane)
    now_epoch = time.time()
    now_iso = _iso(now_epoch)
    stale_iso = _iso(now_epoch + _P0_STALE_AFTER_S)
    expires_iso = _iso(now_epoch + _P0_EXPIRES_AFTER_S)
    payload_hash = hashlib.sha256(payload.encode()).hexdigest()
    conn = sqlite3.connect(str(db), timeout=5.0)
    conn.execute("PRAGMA busy_timeout=5000")
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA foreign_keys=ON")
    try:
        _ensure_schema(conn)
        conn.execute(
            """
            INSERT INTO messages
                (message_id, version, sender, message_type, priority, subject,
                 authority_case, authority_item, recipients_spec, payload,
                 payload_hash, created_at, expires_at, stale_after, tags)
            VALUES (?, ?, ?, 'dispatch', 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                message_id,
                1,
                "reins",
                req.task_id,
                req.authority_case,
                req.task_id,
                recipient,
                payload,
                payload_hash,
                now_iso,
                expires_iso,
                stale_iso,
                json.dumps(["sdlc", "reins", "dispatch"]),
            ),
        )
        conn.execute(
            """
            INSERT INTO recipients (message_id, recipient, state, created_at, updated_at)
            VALUES (?, ?, 'offered', ?, ?)
            """,
            (message_id, recipient, now_iso, now_iso),
        )
        conn.commit()
    finally:
        conn.close()
    return message_id


def read_dispatch_message(message_id: str, db_path: str | None = None) -> dict[str, Any] | None:
    """Read back one envelope + its recipient states (the test/demo asserts the enqueue contract)."""
    db = Path(db_path or default_mq_db_path())
    if not db.exists():
        return None
    conn = sqlite3.connect(str(db), timeout=5.0)
    conn.row_factory = sqlite3.Row
    try:
        row = conn.execute(
            "SELECT * FROM messages WHERE message_id = ?", (message_id,)
        ).fetchone()
        if row is None:
            return None
        d = dict(row)
        d["recipients"] = [
            dict(r)
            for r in conn.execute(
                "SELECT recipient, state FROM recipients WHERE message_id = ?", (message_id,)
            )
        ]
        return d
    finally:
        conn.close()
