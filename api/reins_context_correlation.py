"""reins_context_correlation — CORRECTED to the demand-shape PRODUCER sliver only.

CORRECTION (operator, 2026-07-02): tracking "what we encode into context vs. what a capability then does"
is NOT a new reins system, and reins is NOT the measurer. It is "a particular set of CAPABILITY-SURFACE
measurements" — the spine's EXISTING calculi (identity = measured behavior, REQ-015000; the CEI,
REQ-051500; the demand surface, REQ-061500). We are already doing it. And the reins contribution is the
CAPABILITY DEMAND SHAPE, not the surface: *what we encode into context is the demand shape we specify*.

So this module deliberately does NOT correlate, aggregate, or score behavior (that was an over-reach that
reimplemented the spine's measurer — removed). Its only job is to make the context reins ENCODED legible as
a DEMAND-SHAPE descriptor the spine's capability-surface measurement can key on: which strata were loaded
(fsm what/how/must · impingements · orienting-signals · the tri-audience projection) and which affordances
were offered. This is an INPUT fact, not a measurement. It rides the SHARED demand-shape / status-vector
contract — the spine owns the schema/witness; reins authors the projection rules — so this stays a stub
pending that contract rather than inventing a parallel schema.
"""
from __future__ import annotations

import hashlib
import json


def demand_shape_descriptor(
    *, session_ref: str, strategy: dict, offered_affordances: list[str] | None = None,
    strata: dict | None = None,
) -> dict:
    """What reins ENCODED into context — the demand shape the spine's capability-surface measurement
    consumes (to answer: is the FSM binding? do impingements broaden awareness? are the affordances used?).
    `strategy` = the aggregatable knobs (which strata loaded + their config); `offered_affordances` = the
    affordance_kinds reins presented (#1); `strata` = exact content (provenance). A producer fact — reins
    does not measure the behavioral outcome; the spine does."""
    fp = hashlib.sha256(json.dumps(strategy, sort_keys=True, default=str).encode()).hexdigest()
    return {
        "kind": "demand_shape",
        "session_ref": session_ref,
        "strategy": strategy,
        "demand_shape_fingerprint": fp,
        "offered_affordances": sorted(offered_affordances or []),
        "strata": strata or {},
    }
