"""Bounded, process-local observations of quota snapshots; no admission decisions.

Only the instance supplies the ledger path. The sampler reads that file and appends
metadata transitions to an in-memory ring. It never repairs the producer, stores
receipts, or operates timers. Restart loses the window; polls cannot recover
captures overwritten between samples. Both limits are disclosed in the facet.
"""

from __future__ import annotations

import asyncio
import json
import math
import os
import re
import stat
import time
from collections import deque
from contextlib import asynccontextmanager
from datetime import UTC, datetime
from pathlib import Path
from threading import Lock

MAX_LEDGER_BYTES = 8 * 1024 * 1024
MAX_ROUTES = 512
MAX_EVENTS = 4096
_ROUTE = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}\Z")
_PHASE = re.compile(r"[a-z][a-z0-9_-]{0,63}\Z")
_QUOTA_STATES = {"fresh", "stale", "unknown", "exhausted"}


def _stamp(value):
    if not isinstance(value, str) or len(value) > 64:
        raise ValueError("timestamp")
    dt = datetime.fromisoformat(value)
    if dt.tzinfo is None:
        raise ValueError("timestamp timezone")
    return dt.astimezone(UTC).isoformat().replace("+00:00", "Z")


def _seconds(value):
    return datetime.fromisoformat(value).timestamp()


def _now():
    return datetime.now(UTC).isoformat().replace("+00:00", "Z")


def _read_capture(path):
    # Nonblocking open + regular-file check prevents a misbound FIFO/device from
    # wedging API startup/shutdown. Size is bounded on the actual descriptor.
    fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
            raise ValueError("ledger must be a regular file")
        raw = stream.read(MAX_LEDGER_BYTES + 1)
    if len(raw) > MAX_LEDGER_BYTES:
        raise ValueError("ledger too large")
    doc = json.loads(raw)
    if not isinstance(doc, dict) or not isinstance(doc.get("quota_snapshots"), list):
        raise TypeError("ledger shape")
    captured = _stamp(doc.get("captured_at"))
    snapshots = doc["quota_snapshots"]
    if len(snapshots) > MAX_ROUTES:
        raise ValueError("route limit")
    routes = {}
    for row in snapshots:
        if not isinstance(row, dict):
            raise TypeError("snapshot shape")
        route = row.get("route_id")
        # Undeclared measurements explicitly excluded by the producer are not
        # route admission snapshots. A missing route otherwise is malformed.
        if route is None and row.get("admission_compatible") is False:
            continue
        if not isinstance(route, str) or not _ROUTE.fullmatch(route) or route in routes:
            raise ValueError("route identity")
        phase = row.get("state")
        quota = row.get("subscription_quota_state")
        if phase is not None and (
            not isinstance(phase, str) or not _PHASE.fullmatch(phase)
        ):
            raise ValueError("phase")
        if not isinstance(quota, str) or quota not in _QUOTA_STATES:
            raise ValueError("quota phase")
        row_capture = _stamp(row.get("captured_at"))
        if _seconds(row_capture) > _seconds(captured):
            raise ValueError("snapshot newer than ledger")
        expiry = row.get("fresh_until")
        routes[route] = {
            "state": phase,
            "subscription_quota_state": quota,
            "fresh_until": _stamp(expiry) if expiry is not None else None,
            "captured_at": row_capture,
        }
    return captured, routes


class QuotaPhaseHistory:
    def __init__(self, path="", *, capacity=512, interval_s=60):
        self._lock = Lock()
        self.path = Path(path).expanduser() if isinstance(path, str) and path else None
        self.capacity = capacity
        self.interval_s = interval_s
        self._state, self._reason = "not-sampled", "awaiting-first-sample"
        if (
            type(capacity) is not int
            or not 1 <= capacity <= MAX_EVENTS
            or type(interval_s) not in (int, float)
            or not math.isfinite(interval_s)
            or not 0.01 <= interval_s <= 86400
            or not isinstance(path, str)
        ):
            self._state, self._reason = "unconfigured", "invalid-history-config"
            self.capacity, self.interval_s = 512, 60
        elif self.path is None:
            self._state, self._reason = "unconfigured", "ledger-path-not-configured"
        self._rows = deque(maxlen=self.capacity)
        self._routes = {}
        self._captured = None
        self._attempted = None
        self._succeeded = None
        self._started = _now()
        self._total = 0
        self._gap = False
        self._gaps = 0
        self._last_gap_started = None
        self._last_gap_ended = None

    @classmethod
    def from_config(cls, cfg):
        return cls(
            cfg.get("quota_spend_ledger_live", ""),
            capacity=cfg.get("quota_history_capacity", 512),
            interval_s=cfg.get("quota_history_interval_s", 60),
        )

    def sample(self):
        """Serial, all-or-nothing projection; bad captures retain the prior window."""
        with self._lock:
            if self._state == "unconfigured":
                return
            self._attempted = _now()
            try:
                captured, routes = _read_capture(self.path)
                if self._captured is not None:
                    if _seconds(captured) < _seconds(self._captured):
                        raise ValueError("capture regressed")
                    if captured == self._captured and routes != self._routes:
                        raise ValueError("conflicting capture")
                reason = None
            except FileNotFoundError:
                reason = "ledger-missing"
            except OSError:
                reason = "ledger-unreadable"
            except (TypeError, ValueError, UnicodeError, RecursionError, OverflowError):
                reason = "ledger-malformed"
            if reason:
                self._state, self._reason = "ledger-unavailable", reason
                if not self._gap:
                    self._gap = True
                    self._gaps += 1
                    self._last_gap_started = self._attempted
                return
            advanced = captured != self._captured
            for route in sorted(self._routes.keys() | routes.keys()):
                old, new = self._routes.get(route), routes.get(route)
                # A new capture alone is not a phase transition. Receipt expiry
                # renewal IS a change, even when both named phases stay fresh.
                fields = ("state", "subscription_quota_state", "fresh_until")
                if (
                    old is not None
                    and new is not None
                    and all(old[k] == new[k] for k in fields)
                ):
                    continue
                if old == new:
                    continue
                before, after = old or {}, new or {}
                a, b = before.get("fresh_until"), after.get("fresh_until")
                self._rows.append(
                    {
                        "route_id": route,
                        "captured_at": captured,
                        "snapshot_captured_at": after.get("captured_at"),
                        "sampled_at": self._attempted,
                        "kind": "initial"
                        if old is None
                        else ("removed" if new is None else "transition"),
                        "prior_state": before.get("state"),
                        "state": after.get("state"),
                        "prior_subscription_quota_state": before.get(
                            "subscription_quota_state"
                        ),
                        "subscription_quota_state": after.get(
                            "subscription_quota_state"
                        ),
                        "prior_fresh_until": a,
                        "fresh_until": b,
                        "fresh_until_delta_s": _seconds(b) - _seconds(a)
                        if a and b
                        else None,
                        "after_observation_gap": self._gap,
                    }
                )
                self._total += 1
            self._routes, self._captured = routes, captured
            self._state, self._reason = "observed", ""
            self._succeeded = self._attempted
            # Re-reading the old capture after an outage does not close the gap.
            if advanced and self._gap:
                self._gap = False
                self._last_gap_ended = self._attempted

    def snapshot(self, allowlist=()):
        """GET reads only the ring; returns copies so consumers cannot alter history."""
        with self._lock:
            rows = [
                {**row, "air": {k: "ok" if k in allowlist else "deny" for k in row}}
                for row in self._rows
            ]
            return {
                "schema_version": "quota-phase-history.read.v1",
                "state": self._state,
                "dark": self._state != "observed",
                "reason": self._reason,
                "retention": "process-local-bounded-window",
                "history_complete": False,
                "capture_coverage": "sampled-captures-only",
                "started_at": self._started,
                "sample_interval_s": self.interval_s,
                "capacity": self.capacity,
                "evicted_rows": self._total - len(rows),
                "route_count": len(self._routes),
                "last_attempt_at": self._attempted,
                "last_success_at": self._succeeded,
                "source_captured_at": self._captured,
                "source_age_s": time.time() - _seconds(self._captured)
                if self._captured
                else None,
                "observation_gap": self._gap,
                "observation_gap_count": self._gaps,
                "last_gap_started_at": self._last_gap_started,
                "last_gap_ended_at": self._last_gap_ended,
                "rows": rows,
            }

    async def _poll(self):
        while True:
            await asyncio.sleep(self.interval_s)
            await asyncio.to_thread(self.sample)

    @asynccontextmanager
    async def lifespan(self, app):
        if self._state == "unconfigured":
            yield
            return
        await asyncio.to_thread(self.sample)
        task = asyncio.create_task(self._poll())
        try:
            yield
        finally:
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass
