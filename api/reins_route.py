"""reins_route — the honest ROUTE projection (U4, design pack §Design 2 / E6.2).

ROUTE is the prioritization + capability-routing PROJECTION for single-point dispatch. Reins RENDERS
spine routing evidence; it MINTS NOTHING — no routing decision, no display scalar, no local banding.

Two honest surfaces over the spine's measurement substrate (gate-events, consume-don't-fork):
- /route/posture: `NO SPINE DECISION ON FILE` (reins holds no routing decision — a decision is the
  spine's to make and echo; until then the posture says so, never a fabricated band/floor). Reports the
  keyspace coverage (observed routing_classes vs the pinned frozen-11), the source states (gate_events
  live/dark, edt DARK — no EDT feed yet), and the reqvec contract (8 dims, strict ints 0..5 — pinned to
  the PRODUCER's declared range, NEVER inferred from the sample).
- /route/candidates: the measured DEMAND evidence — the latest measured requirement_vector per observed
  routing_class (raw measured, not a computed score). task_reqvec is ABSENT (no producer) — rendered
  absent, never fabricated. Candidate RANKING is a spine decision (DARK until SA feeds land).

Honest-when-starved: an unreachable/absent gate-events feed renders dark, never empty-as-fine.
"""

from __future__ import annotations

import json
import os
import time
from datetime import UTC, datetime

# The frozen-11 routing keyspace — the EDT<->router<->reins integration anchor. Pinned as a runtime
# literal here (mirrors hapax-spine ROUTING_CLASSES) and drift-pinned by test.
# Reins does NOT expand this unilaterally; a spine 11->17 expansion is a NOTIFIED interface event.
ROUTING_CLASSES_PINNED = (
    "coordination", "research_support", "docs_planning", "source_python", "source_other",
    "source_governance", "runtime_ops", "public_surface", "provider_spend", "operator_action",
    "verification",
)

# The 8 requirement_vector dims + the PRODUCER-declared range (strict ints 0..5;
# hapax-spine). Pinned to the contract, never the JSONL sample.
REQVEC_DIMS = (
    "quality_floor", "information_scope", "context_length", "mutation_risk",
    "verification_demand", "ambiguity_novelty", "composition_coupling", "governance_sensitivity",
)
REQVEC_MIN = 0
REQVEC_MAX = 5

# reins holds NO routing decision — a decision is the spine's to make + echo (SA feeds). Until then the
# posture is honestly this; reins NEVER mints a band/floor/candidate ranking locally.
NO_DECISION = "NO SPINE DECISION ON FILE"


def gate_events_path() -> str:
    env = os.environ.get("REINS_GATE_EVENTS", "").strip()
    if env:
        return env
    return os.path.join(os.path.expanduser("~"), ".cache", "hapax", "sdlc-routing", "gate-events.jsonl")


#: A gate-events feed older than this is STALE: its rows are real but they no longer describe the
#: estate. Tied to the read model's own freshness convention (`_freshness` decays with tau = 6h in
#: reins_read.py), so a feed is stale once it is well past the point where any row would still read
#: as fresh. Overridable per-instance; engine holds no instance policy.
_GATE_EVENTS_STALE_AFTER_S_DEFAULT = 6 * 3600.0


def _stale_after_s(raw: str | None) -> float:
    """Parse the TTL override, falling back to the default on anything unusable.

    A threshold that is nan, inf, negative or unparseable cannot gate anything: nan loses every
    comparison, so a feed would ALWAYS read fresh — the exact failure this module exists to prevent
    — and inf means nothing is ever stale. Unparseable text previously raised at IMPORT, taking the
    whole module down. Misconfiguration must not silently disable the freshness gate, nor crash it.
    """
    if not raw or not raw.strip():
        return _GATE_EVENTS_STALE_AFTER_S_DEFAULT
    try:
        v = float(raw)
    except (TypeError, ValueError):
        return _GATE_EVENTS_STALE_AFTER_S_DEFAULT
    if v != v or v in (float("inf"), float("-inf")) or v < 0:  # nan / +-inf / negative
        return _GATE_EVENTS_STALE_AFTER_S_DEFAULT
    return v


GATE_EVENTS_STALE_AFTER_S = _stale_after_s(os.environ.get("REINS_GATE_EVENTS_TTL_S"))


def _newest_row_age_s(rows: list[dict], now: float) -> float | None:
    """Age of the most recent row by its own `ts`, or None if no row carries a parseable one.

    None is NOT "fresh" — the caller treats an unreadable age as stale. An age we cannot compute is
    an age we cannot vouch for, and the honest direction is starved (see the module's honest-dark
    discipline), never live.
    """
    newest = None
    for r in rows:
        # Defensive: the loader filters to dicts, but this helper is the one place an age is
        # derived, so it must not crash if a caller ever hands it something else.
        if not isinstance(r, dict):
            continue
        raw = str(r.get("ts") or "").strip()
        if not raw:
            continue
        try:
            t = datetime.fromisoformat(raw.replace("Z", "+00:00"))
        except ValueError:
            continue
        if t.tzinfo is None:
            t = t.replace(tzinfo=UTC)
        ts = t.timestamp()
        # A row stamped in the FUTURE is not evidence of freshness — clamping its age to 0
        # would let one bad row make a stopped feed read live, the very failure this gate
        # exists to stop. Ignore it; if nothing non-future remains the age is unverifiable,
        # and unverifiable means stale.
        if ts > now:
            continue
        if newest is None or ts > newest:
            newest = ts
    return None if newest is None else max(0.0, now - newest)


def _read_gate_events(
    path: str, now: float | None = None
) -> tuple[list[dict], bool, str, float | None]:
    """Return (rows, dark, error, age_s). Missing/unreadable feed => dark (honest, not empty-as-fine).

    A present-but-STALE feed is NOT dark — its rows exist and are real — but the caller must not
    report it live. Reins must not make stale rows look live; that rule is why `age_s` is returned
    rather than being folded into `dark`, which would erase the distinction between "no producer"
    and "a producer that stopped".
    """
    if not os.path.exists(path):
        return [], True, "gate-events feed absent (spine routing substrate not present)", None
    try:
        rows = []
        with open(path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    row = json.loads(line)
                except Exception:
                    continue  # skip a corrupt line, never crash
                # A JSONL feed's rows are objects. An array/string/null parses fine but carries no
                # `ts` and is not a row; keeping it would dilute the age computation with junk.
                if isinstance(row, dict):
                    rows.append(row)
        return rows, False, "", _newest_row_age_s(rows, now if now is not None else time.time())
    except OSError as e:
        return [], True, str(e), None


def read_route_posture(path: str | None = None) -> dict:
    rows, dark, err, age_s = _read_gate_events(path or gate_events_path())
    if dark:
        return {
            "dark": True,
            "error": err,
            "decision": NO_DECISION,
            "sources": [
                # Same shape as the non-dark branch so a consumer never special-cases dark.
                {
                    "name": "gate_events",
                    "state": "dark",
                    "events": 0,
                    "age_s": None,
                    "stale_after_s": GATE_EVENTS_STALE_AFTER_S,
                },
                {"name": "edt", "state": "dark"},
            ],
        }
    # Stale is its own state. "live" here previously meant only "the file exists".
    stale = age_s is None or age_s > GATE_EVENTS_STALE_AFTER_S
    gate_state = "stale" if stale else "live"
    observed = sorted({str(r.get("routing_class", "")) for r in rows if r.get("routing_class")})
    observed = [c for c in observed if c]
    unknown = [c for c in observed if c not in ROUTING_CLASSES_PINNED]
    return {
        "dark": False,
        # reins mints no routing decision — this is the honest posture until the spine echoes one.
        "decision": NO_DECISION,
        "keyspace": {
            "pinned": list(ROUTING_CLASSES_PINNED),
            "pinned_count": len(ROUTING_CLASSES_PINNED),
            "observed": observed,
            "observed_count": len(observed),
            "unknown_observed": unknown,  # a class outside the pinned-11 = a keyspace drift signal
        },
        "reqvec": {"dims": list(REQVEC_DIMS), "min": REQVEC_MIN, "max": REQVEC_MAX,
                   "range_source": "producer-contract"},
        "sources": [
            {
                "name": "gate_events",
                "state": gate_state,
                "events": len(rows),
                # age is reported even when fresh: a consumer that cannot see the age cannot
                # independently judge the state, and must then trust ours.
                "age_s": None if age_s is None else round(age_s, 1),
                "stale_after_s": GATE_EVENTS_STALE_AFTER_S,
            },
            {"name": "edt", "state": "dark"},  # no EDT feed yet (spine ask)
        ],
    }


def _measured_reqvec_or_absent(rv: object):
    """The measured 8-dim vector iff ALL dims are present with an int value; else the "absent" sentinel.
    An empty or partial vector is NOT a measured vector — rendering it as {dim: null} would be the
    absent-ambiguity A2.3 forbids (a null masquerading as measured)."""
    if not isinstance(rv, dict):
        return "absent"
    out = {}
    for d in REQVEC_DIMS:
        v = rv.get(d)
        if not isinstance(v, int) or isinstance(v, bool):  # missing / null / non-int -> not measured
            return "absent"
        out[d] = v
    return out


def read_route_candidates(path: str | None = None) -> dict:
    """Measured DEMAND evidence per routing_class — the latest measured requirement_vector per observed
    class (raw measured, no computed score). task_reqvec ABSENT (no producer). Candidate RANKING is a
    spine decision (DARK)."""
    rows, dark, err, _age_s = _read_gate_events(path or gate_events_path())
    if dark:
        return {"dark": True, "error": err, "decision": NO_DECISION, "candidates": [],
                "task_reqvec": "absent"}

    # latest COMPLETE measured reqvec per class (raw evidence; NO mean/aggregate/score — never mint a
    # scalar). Only a COMPLETE 8-dim vector overwrites `latest`: a later empty/partial vector must NOT
    # clobber an earlier real measurement into a fabricated ABSENT (last-COMPLETE-wins, not last-dict-wins).
    latest: dict[str, dict] = {}
    counts: dict[str, int] = {}
    for r in rows:
        rc = str(r.get("routing_class", ""))
        rv = r.get("requirement_vector")
        if not rc:
            continue
        counts[rc] = counts.get(rc, 0) + 1
        if isinstance(rv, dict) and _measured_reqvec_or_absent(rv) != "absent":
            latest[rc] = rv  # last COMPLETE write wins — most-recent fully-measured vector

    candidates = []
    for rc in sorted(counts):
        candidates.append({
            "routing_class": rc,
            "in_keyspace": rc in ROUTING_CLASSES_PINNED,
            "measured_events": counts[rc],
            # dispatch_reqvec: the measured demand vector, present ONLY when a COMPLETE measured vector
            # exists. An empty/partial vector (e.g. the live `verification` row's {}) renders the word
            # ABSENT, never an 8-key null-dict — a null-dict is the absent-ambiguity A2.3 forbids.
            "dispatch_reqvec": _measured_reqvec_or_absent(latest.get(rc)),
        })
    return {
        "dark": False,
        "decision": NO_DECISION,  # ranking is the spine's; reins shows measured demand, not a verdict
        "task_reqvec": "absent",  # no task-level reqvec producer yet — absent, never fabricated
        "candidates": candidates,
    }
