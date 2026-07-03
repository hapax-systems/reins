"""reins_ledger — the durable command ledger that makes the frontdoor WITNESSED (U3, CP-A).

Every command through the governed rail appends a DEMAND row and a VERDICT row to an append-only
JSONL under ~/.cache/hapax/reins/. A row is a DATOM: typed refs (task_id, session_role, route_id,
command_id, inflection_id) render through the one cell-grammar encoder; there is no bespoke
CommandResult path.

Authority note: the ledger records demand INTENT + verdict; it mints NO authority (the real transport
lands at U7 against the spine verify surfaces). Trust boundary (design pack A3.2): the ledger is a
filesystem-permission-guarded JSONL. Rows are HASH-CHAINED + HMAC-SIGNED (A3.x / avsdlc-receipt-
integrity). HONEST SCOPE (adversarial review, 2026-07-03): ``verify_chain()`` detects in-place
ALTERATION, INSERTION, REORDER, and mid-chain DELETION of retained rows, and reports a chain-less
ledger as ``unsigned`` (never ``verified``) — so a no-key attacker cannot FORGE a ``verified`` verdict.
It does NOT, by itself, detect TAIL TRUNCATION (hiding the most recent rows leaves a valid prefix) or
KEY SUBSTITUTION: those need an OUT-OF-BAND anchor (``chain_head()`` + row count published where the
ledger's writer cannot rewrite it), because a second principal with write on the ledger DIRECTORY can
unlink/replace a same-dir key or anchor (dir-write governs unlink, so the 0600 mode protects READ, not
replacement). Full defense of the "second principal / non-loopback bind" boundary requires the key +
anchor in an operator-only directory or out-of-band.

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
import tempfile
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
        """Verify the signed hash-chain. Returns ``(ok, first_signed_index, reason)`` with reason in
        {ok, empty, unsigned, hash-mismatch, bad-signature, broken-link, unhashed-row-in-chain}.

        HONEST SCOPE (post-review): detects in-place ALTERATION, INSERTION, REORDER, and mid-chain
        DELETION of retained rows, and reports a chain-less ledger as ``unsigned`` / an empty file as
        ``empty`` — NEVER ``verified``. So a no-key attacker CANNOT forge a ``verified`` verdict by
        stripping every ``hash`` (that reads ``unsigned``) or by content edits. The first signed row
        must be genesis-anchored (``prev == ""``), so stripping the leading signed rows breaks the link.
        RESIDUAL — NOT defended here: TAIL TRUNCATION (hiding the most recent rows still leaves a valid
        prefix that reads ``ok``) and KEY SUBSTITUTION require an OUT-OF-BAND anchor — publish
        ``chain_head()`` + the row count where the ledger's writer cannot rewrite them; a second
        principal with write on the ledger DIRECTORY defeats any same-dir anchor (dir-write ⇒ unlink)."""
        rows = self.rows()
        if not rows:
            return (False, -1, "empty")  # no witnessed history — honest, NOT "verified"
        first_signed = next((i for i, r in enumerate(rows) if "hash" in r), None)
        if first_signed is None:
            # no signed rows at all: a genuine pre-signing legacy ledger OR a strip-all-hashes forgery —
            # indistinguishable, so NEVER "verified". An honest consumer treats "unsigned" as untrusted.
            return (False, -1, "unsigned")
        prev = ""  # the first signed row must be genesis (prev == "") — leading-strip breaks here
        for i in range(first_signed, len(rows)):
            row = rows[i]
            if "hash" not in row:
                return (False, i, "unhashed-row-in-chain")  # a gap after signing began = tampering
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
        return (True, first_signed, "ok")

    def chain_head(self) -> str:
        """The current chain head (the last row's hash). Publish this + ``chain_length()`` OUT-OF-BAND (a
        location the ledger's writer cannot rewrite) to detect the residual tail-truncation / whole-
        replacement that ``verify_chain`` alone cannot (a valid prefix of a valid chain still verifies)."""
        return self._head

    def chain_length(self) -> int:
        """The number of signed rows — the count half of the out-of-band anchor pair (with chain_head())."""
        return sum(1 for r in self.rows() if "hash" in r)

    def verify_against_anchor(self, expected_head: str, expected_count: int) -> tuple[bool, str]:
        """Detect the residual ``verify_chain`` cannot — TAIL-TRUNCATION and WHOLE-REPLACEMENT — by
        comparing the live chain head + signed-row count against an anchor the caller stored OUT-OF-BAND.
        Returns ``(ok, reason)``. This is the ONLY defense against a writer who can rewrite same-dir
        state: reins provides the MECHANISM; the caller must provide tamper-proof STORAGE (an operator-
        only directory / a second host / append-only log), which reins cannot guarantee on-box. Verifies
        the in-place chain first (a mismatch there is reported as its own reason), then the anchor."""
        chain_ok, _i, chain_reason = self.verify_chain()
        if not chain_ok:
            return (False, chain_reason)
        if self._head != expected_head:
            return (False, "anchor-head-mismatch")
        if self.chain_length() != expected_count:
            return (False, "anchor-count-mismatch")
        return (True, "ok")


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
    """Load — or first-time create, mode 0600 — the HMAC key that signs ledger rows.

    Fail-CLOSED on any weak/ambiguous key rather than silently downgrading to a forgeable one (the
    review found ``b""`` from a 0-byte file made the HMAC a no-op). A key shorter than 32 bytes is a
    HARD error — a missing/empty/short key raises, which degrades the command router to read-only-and-
    disclosed (never a silent public-key signer). The read uses ``O_NOFOLLOW`` (a pre-planted symlink
    key is refused, not followed); creation writes a temp file + fsync + atomic hard-link so a crash
    between create and write can never leave a 0-byte key. NOTE (honest): the 0600 mode protects READ,
    but if the ledger DIRECTORY is writable by a second principal, dir-write governs unlink — they can
    replace the key. Full protection needs the key in an operator-only directory or out-of-band."""
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    except FileNotFoundError:
        fd = None
    except OSError as e:  # ELOOP (symlink) / not-a-regular-file — refuse, never follow
        raise RuntimeError(f"ledger key {path!r} is not a regular file (refusing to follow): {e}") from e
    if fd is not None:
        with os.fdopen(fd, "rb") as f:
            existing = f.read()
        if len(existing) >= 32:
            return existing
        raise RuntimeError(
            f"ledger key {path!r} is {len(existing)}B (<32) — refusing a weak/empty key; "
            "remove it to regenerate (a truncated key would make row signatures forgeable)"
        )
    key = os.urandom(32)
    d = os.path.dirname(path) or "."
    os.makedirs(d, exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=".ledger-key.", dir=d)
    try:
        os.fchmod(fd, 0o600)
        os.write(fd, key)
        os.fsync(fd)
    finally:
        os.close(fd)
    try:
        os.link(tmp, path)  # atomic; FileExistsError if another instance won the create race
    except FileExistsError:
        os.unlink(tmp)
        return load_ledger_key(path)  # re-read the winner's key, with the same validation
    os.unlink(tmp)
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
        # tamper-evidence of the witness itself — honest, never faked green. "verified" ONLY for an intact
        # signed chain; "empty"/"unsigned" for no signed history (a strip-all-hashes forgery reads
        # "unsigned", NOT "verified"); "broken:<reason>" for detected in-place tampering. RESIDUAL: tail
        # truncation reads "verified" for the surviving prefix — detect it with an out-of-band chain_head
        # + row-count anchor (verify_chain's docstring). A consumer must treat non-"verified" as untrusted.
        "integrity": (
            "verified"
            if chain_ok
            else chain_reason
            if chain_reason in ("empty", "unsigned")
            else f"broken:{chain_reason}"
        ),
    }
