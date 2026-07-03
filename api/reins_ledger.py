"""reins_ledger — the durable command ledger that makes the frontdoor WITNESSED (U3, CP-A).

Every command through the governed rail appends a DEMAND row and a VERDICT row to an append-only
JSONL under ~/.cache/hapax/reins/. A row is a DATOM: typed refs (task_id, session_role, route_id,
command_id, inflection_id) render through the one cell-grammar encoder; there is no bespoke
CommandResult path.

Authority note: the ledger records demand INTENT + verdict; it mints NO authority (the real transport
lands at U7 against the spine verify surfaces). Trust boundary (design pack A3.2): the ledger is a
filesystem-permission-guarded JSONL. Rows are now HASH-CHAINED + HMAC-SIGNED (A3.x / avsdlc-receipt-
integrity), so alteration/insert/delete/reorder is tamper-EVIDENT and forging a row requires the 0600
signing key — which a SECOND PRINCIPAL (the exact boundary this note named as "reopens on a second
principal / non-loopback bind") cannot read. Detection is via ``verify_chain()``; the key protects the
signature, not the JSONL's readability.

event_id = sha256(canonical_json({verb, target, idempotency_key})) — canonical JSON (sorted keys, no
whitespace), NEVER delimiter concatenation, so there is no id-collision class (A3.11). Durable
idempotency: a replayed idempotency_key resolves to the SAME event_id, and a reload of the ledger
rebuilds the seen-set, so a retried command across a restart is a duplicate, never a second demand.
"""

from __future__ import annotations

import hashlib
import hmac
import json
import os
import threading
from dataclasses import dataclass, field
from typing import Any


def canonical_event_id(verb: str, target: str, idempotency_key: str) -> str:
    payload = json.dumps(
        {"verb": verb, "target": target, "idempotency_key": idempotency_key},
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(payload.encode()).hexdigest()


@dataclass
class CommandRefs:
    task_id: str = ""
    session_role: str = ""
    route_id: str = ""
    command_id: str = ""
    inflection_id: str = ""

    def as_dict(self) -> dict[str, str]:
        return {
            "task_id": self.task_id,
            "session_role": self.session_role,
            "route_id": self.route_id,
            "command_id": self.command_id,
            "inflection_id": self.inflection_id,
        }


@dataclass
class CommandLedger:
    """An append-only command ledger. ``path`` is a JSONL file; ``clock`` returns an ISO-8601
    timestamp string (injected so tests are deterministic). Reloads rebuild the seen-set so
    idempotency survives a restart (the frontdoor's externalized state, design pack A3.9).

    Rows are hash-chained + HMAC-signed for tamper-evidence (``key`` injected for deterministic tests;
    loaded from the sibling 0600 key file when empty). ``verify_chain()`` walks the chain."""

    path: str
    clock: Any = None  # callable() -> iso ts; injected
    key: bytes = b""  # HMAC key signing each row; loaded from the sibling 0600 key file if empty
    _seen: dict[str, str] = field(default_factory=dict)  # event_id -> demand receipt_id (any attempt)
    # event_id -> the terminal-SUCCESS outcome (for idempotent replay). Idempotency dedups on SUCCESS,
    # not on demand: a refused/failed attempt is RETRYABLE (its condition may since have changed — e.g.
    # a generation that was absent is now staged); only a command that already SUCCEEDED is not re-run.
    _succeeded: dict[str, dict] = field(default_factory=dict)
    _head: str = field(default="", compare=False)  # chain head — the last row's hash (prev-link source)
    _lock: Any = field(default_factory=threading.Lock, compare=False, repr=False)

    def __post_init__(self) -> None:
        if not self.key:
            self.key = load_ledger_key(ledger_key_path(self.path))
        self._reload()

    def _reload(self) -> None:
        self._seen = {}
        self._succeeded = {}
        self._head = ""
        if not os.path.exists(self.path):
            return
        with open(self.path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    row = json.loads(line)
                except Exception:
                    continue  # honest-skip a corrupt line; never crash the ledger
                if not isinstance(row, dict):
                    continue  # valid JSON but non-object (null/true/42/"s"/[...]) — skip, never .get-crash
                eid = row.get("event_id")
                if row.get("kind") == "demand" and eid:
                    self._seen[eid] = row.get("receipt_id", "")
                elif row.get("kind") == "verdict" and eid and row.get("status") == "ok":
                    # a terminal-success verdict marks the command completed (replayable outcome).
                    self._succeeded[eid] = {"http": row.get("http", 200), "receipt_id": self._seen.get(eid, "")}
                h = row.get("hash")
                if h:
                    self._head = h  # chain head advances to the last hashed row (restart-durable link)

    def _ts(self) -> str:
        if callable(self.clock):
            return str(self.clock())
        return "1970-01-01T00:00:00Z"  # deterministic default; production injects a real clock

    def _append(self, row: dict[str, Any]) -> None:
        # fsync per row so the witness survives power loss — closing the file only flushes to the OS page
        # cache, which a hard power-off discards. This estate has a documented power-loss incident
        # (2026-06-08 UPS trip) AND a disarmed hardware watchdog, so "durable" must mean on-disk, not
        # in-cache. Per-row fsync is trivially cheap at the command write rate. Best-effort: an fsync that
        # raises (e.g. a filesystem without fsync) must not lose the write already flushed to the OS.
        #
        # Tamper-evidence (closes G8): link each row to the prior head, hash the {content+prev} body, and
        # HMAC-sign the hash. Any alteration/insert/delete/reorder breaks the recomputed hash or the
        # prev-link; forging needs the 0600 key a second principal cannot read (the named trust boundary).
        chained = {**row, "prev": self._head}
        row_hash = hashlib.sha256(
            json.dumps(chained, sort_keys=True, separators=(",", ":")).encode()
        ).hexdigest()
        chained["hash"] = row_hash
        chained["sig"] = hmac.new(self.key, row_hash.encode(), hashlib.sha256).hexdigest()
        os.makedirs(os.path.dirname(self.path) or ".", exist_ok=True)
        with open(self.path, "a", encoding="utf-8") as f:
            f.write(json.dumps(chained, sort_keys=True, separators=(",", ":")) + "\n")
            f.flush()
            try:
                os.fsync(f.fileno())
            except OSError:
                pass
        self._head = row_hash

    def record_demand(
        self, verb: str, target: str, idempotency_key: str, refs: CommandRefs | None = None
    ) -> dict[str, Any]:
        """Append a demand row, or — iff this event already reached a terminal SUCCESS — return the
        replayable duplicate (carrying the ORIGINAL success http, never a fabricated 200). A first
        attempt OR a retry after a non-success verdict is a FRESH demand (retryable). The check+append+
        mark is a critical section (the /command/{verb} endpoint is a sync def on Starlette's threadpool,
        so concurrent same-key requests genuinely interleave)."""
        event_id = canonical_event_id(verb, target, idempotency_key)
        with self._lock:
            prior = self._succeeded.get(event_id)
            if prior is not None:
                return {
                    "kind": "demand",
                    "event_id": event_id,
                    "receipt_id": prior["receipt_id"],
                    "duplicate": True,
                    "prior_http": prior["http"],  # replay the original success outcome, not a synthesized 200
                }
            receipt_id = "demand-" + event_id[:16]
            row = {
                "kind": "demand",
                "event_id": event_id,
                "receipt_id": receipt_id,
                "ts": self._ts(),
                "verb": verb,
                "target": target,
                "idempotency_key": idempotency_key,
                "refs": (refs or CommandRefs()).as_dict(),
                "duplicate": False,
            }
            self._append(row)
            self._seen[event_id] = receipt_id
            return row

    def record_verdict(
        self, event_id: str, status: str, http: int, reason: str = "", effect: dict | None = None
    ) -> dict[str, Any]:
        """Append a verdict row for a prior demand. ``status`` is the router verdict
        (ok/authority-rejected/preflight-failed/transport-failed/not-wired/...). A terminal-success
        verdict also arms idempotent replay for this event_id. ``effect`` is the APPLIED effect the
        transport produced (receipt_id / event_seq / fold_delta / spooled) — so the signed ledger
        carries preview→gate→APPLY, not just the gate verdict. Empty on a rejected/failed attempt
        (nothing was applied — an honest empty effect, never a fabricated one)."""
        row = {
            "kind": "verdict",
            "event_id": event_id,
            "ts": self._ts(),
            "status": status,
            "http": http,
            "reason": reason,
            "effect": effect or {},  # what the transport actually did — the APPLY half of the record
            # witness stays pending until the spine echoes command_id back (SA-3, wired at U7).
            "witness": "pending",
        }
        with self._lock:
            self._append(row)
            if status == "ok":
                self._succeeded[event_id] = {"http": http, "receipt_id": self._seen.get(event_id, "")}
        return row

    def rows(self) -> list[dict[str, Any]]:
        """Read the full ledger back (oldest -> newest). Missing file = empty."""
        out: list[dict[str, Any]] = []
        if not os.path.exists(self.path):
            return out
        with open(self.path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    obj = json.loads(line)
                except Exception:
                    continue
                if isinstance(obj, dict):  # skip valid-JSON-but-non-object lines (never .get-crash the fold)
                    out.append(obj)
        return out

    def verify_chain(self) -> tuple[bool, int, str]:
        """Walk the ledger and verify tamper-evidence: each hashed row's content-hash, its HMAC
        signature, and its prev-link to the row before it. Returns ``(ok, first_broken_index, reason)``
        — reason in {ok, hash-mismatch, bad-signature, broken-link, unhashed-row-in-chain}. Pre-chain
        rows (written before signing, no ``hash``) are a legal leading prefix and are skipped; once the
        chain has begun, a subsequent unhashed row is itself evidence of tampering."""
        prev = ""
        started = False
        for i, row in enumerate(self.rows()):
            if "hash" not in row:
                if started:
                    return (False, i, "unhashed-row-in-chain")
                continue  # legal pre-chain prefix
            started = True
            stored = row.get("hash", "")
            body = {k: v for k, v in row.items() if k not in ("hash", "sig")}
            recomputed = hashlib.sha256(
                json.dumps(body, sort_keys=True, separators=(",", ":")).encode()
            ).hexdigest()
            if recomputed != stored:
                return (False, i, "hash-mismatch")
            expected_sig = hmac.new(self.key, stored.encode(), hashlib.sha256).hexdigest()
            if not hmac.compare_digest(str(row.get("sig", "")), expected_sig):
                return (False, i, "bad-signature")
            if row.get("prev", "") != prev:
                return (False, i, "broken-link")
            prev = stored
        return (True, -1, "ok")

    def chain_head(self) -> str:
        """The current chain head (the last row's hash) — an anchor a caller can publish externally so
        even a full-chain rewrite by a key-holder becomes detectable against the published head."""
        return self._head


def ledger_path() -> str:
    env = os.environ.get("REINS_COMMAND_LEDGER", "").strip()
    if env:
        return env
    home = os.path.expanduser("~")
    return os.path.join(home, ".cache", "hapax", "reins", "commands.jsonl")


def ledger_key_path(led_path: str | None = None) -> str:
    """The signing-key path — a sibling ``ledger-key`` next to the ledger JSONL (or overridden by
    ``REINS_COMMAND_LEDGER_KEY``)."""
    env = os.environ.get("REINS_COMMAND_LEDGER_KEY", "").strip()
    if env:
        return env
    base = os.path.dirname(led_path or ledger_path())
    return os.path.join(base or ".", "ledger-key")


def load_ledger_key(path: str) -> bytes:
    """Load — or first-time create, mode 0600 — the HMAC key that signs ledger rows. Created atomically
    with ``O_EXCL`` so a concurrent first-run cannot race two keys; a loser re-reads the winner's key.
    A second principal without the operator's uid cannot read a 0600 key, so the signature is tamper-
    evidence exactly at the trust boundary the ledger names."""
    try:
        with open(path, "rb") as f:
            existing = f.read()
        if existing:
            return existing
    except FileNotFoundError:
        pass
    key = os.urandom(32)
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    try:
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        with open(path, "rb") as f:
            return f.read()
    with os.fdopen(fd, "wb") as f:
        f.write(key)
    return key


def iso_utc_now() -> str:
    """Production clock for the ledger — ISO-8601 UTC. Injected so tests stay deterministic."""
    from datetime import UTC, datetime

    return datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def read_commands(path: str | None, allowlist: list[str] | None = None, limit: int = 80) -> dict:
    """The /read/commands projection: demand+verdict datoms with an honest witness state and an
    `absent` enforcement cell (the gate observably does not exist yet — NEVER dark-conflated,
    design pack A3.8). AIR-classified default-deny like every other row kind. ``integrity`` reports the
    tamper-evidence verdict of the ledger's hash-chain (verified / a break reason) — honest, never green
    by default."""
    from reins_read import classify_air  # local import: avoid a cycle at module load

    p = path or ledger_path()
    allow = allowlist or []
    led = CommandLedger(p, clock=None)
    raw = led.rows()
    chain_ok, _broken_at, chain_reason = led.verify_chain()

    # fold demand+verdict pairs into command datoms keyed by event_id.
    demands: dict[str, dict] = {}
    verdicts: dict[str, dict] = {}
    for row in raw:
        if row.get("kind") == "demand":
            demands[row["event_id"]] = row
        elif row.get("kind") == "verdict":
            verdicts[row["event_id"]] = row  # last verdict wins

    commands = []
    for eid, d in list(demands.items())[-limit:]:
        v = verdicts.get(eid, {})
        refs = d.get("refs", {})
        fields = {
            "event_id": eid,
            "verb": d.get("verb", ""),
            "target": d.get("target", ""),
            "status": v.get("status", "pending"),  # no verdict yet -> honest pending
            "witness": v.get("witness", "pending"),  # spine echo pending until U7/SA-3
            # the APPLY indicator: did the transport produce an effect? (a receipt landed). Structural
            # boolean-ish — the effect CONTENT (fold_delta/receipt_id) stays in the signed ledger, never
            # surfaced raw here. Empty for a rejected/failed attempt (nothing applied) — honest.
            "applied": "yes" if (v.get("effect") or {}).get("receipt_id") else "",
            "task_id": refs.get("task_id", ""),
            "session_role": refs.get("session_role", ""),
            "route_id": refs.get("route_id", ""),
            "command_id": refs.get("command_id", ""),
        }
        air = classify_air(fields, allow)
        # design pack §9: a command TARGET is path-class (it can name a repo/worktree/path) —
        # force-deny it on air regardless of the allowlist, mirroring to_turn's summary deny. The
        # generic facet allowlist happens to classify `target` (an EDGES facet) as ok, which would
        # leak a path on the derived channel; command targets are SENSITIVE. (never leak on air)
        air["target"] = "deny"
        # the STRUCTURAL skeleton airs (closed, safe vocabulary — like routing_class on :route): verb,
        # status, witness carry no path/PII. The generic allowlist denies them (not facet names), which
        # would MISMATCH the renderer (RenderCommandRow shows the skeleton); classify them ok so the
        # projection contract matches the rendered surface. Path/id-class refs stay denied below.
        for structural in ("verb", "status", "witness", "applied"):
            air[structural] = "ok"
        commands.append({**fields, "air": air})

    return {
        "dark": False,
        "commands": commands,
        # enforcement is ABSENT, not dark: the dispatch-gate observably does not exist until U13/CP-E.
        "enforcement": "absent",
        # tamper-evidence of the witness itself: "verified" or a break reason — honest, never faked green.
        "integrity": "verified" if chain_ok else f"broken:{chain_reason}",
    }
