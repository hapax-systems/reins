"""U4 ROUTE read-fold honesty tests — reins projects, never mints a routing decision or scalar."""

import json

from reins_route import (
    NO_DECISION,
    REQVEC_DIMS,
    REQVEC_MAX,
    REQVEC_MIN,
    ROUTING_CLASSES_PINNED,
    read_route_candidates,
    read_route_posture,
)


def _write_events(path, rows):
    with open(path, "w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")


def test_reqvec_scale_pinned_to_producer_contract_not_sample():
    # the reqvec range is the PRODUCER's declared strict-int 0..5, NEVER inferred from the JSONL sample.
    # (design pack R1: a sample-derived range would clip honest 5s or mis-scale.)
    assert (REQVEC_MIN, REQVEC_MAX) == (0, 5)
    assert len(REQVEC_DIMS) == 8


def test_keyspace_is_the_frozen_11():
    # drift pin: the routing keyspace is exactly the frozen-11 (the EDT<->router<->reins anchor).
    assert len(ROUTING_CLASSES_PINNED) == 11
    assert "source_python" in ROUTING_CLASSES_PINNED and "verification" in ROUTING_CLASSES_PINNED


def test_posture_is_no_spine_decision_and_edt_dark(tmp_path):
    p = str(tmp_path / "gate-events.jsonl")
    _write_events(p, [
        {"routing_class": "source_other", "requirement_vector": {d: 3 for d in REQVEC_DIMS}},
        {"routing_class": "docs_planning", "requirement_vector": {d: 1 for d in REQVEC_DIMS}},
    ])
    post = read_route_posture(p)
    assert post["dark"] is False
    # reins mints NO routing decision.
    assert post["decision"] == NO_DECISION
    # keyspace coverage: observed subset of the pinned-11, no unknown drift.
    assert set(post["keyspace"]["observed"]) == {"source_other", "docs_planning"}
    assert post["keyspace"]["pinned_count"] == 11 and post["keyspace"]["unknown_observed"] == []
    # reqvec contract surfaced; edt source is DARK (no feed).
    assert post["reqvec"] == {"dims": list(REQVEC_DIMS), "min": 0, "max": 5, "range_source": "producer-contract"}
    assert any(s["name"] == "edt" and s["state"] == "dark" for s in post["sources"])
    assert any(s["name"] == "gate_events" and s["state"] == "live" for s in post["sources"])


def test_candidates_measured_demand_task_reqvec_absent_no_scalar(tmp_path):
    p = str(tmp_path / "gate-events.jsonl")
    _write_events(p, [
        {"routing_class": "source_python", "requirement_vector": {d: 5 for d in REQVEC_DIMS}},
        {"routing_class": "source_python", "requirement_vector": {d: 4 for d in REQVEC_DIMS}},  # latest wins
    ])
    cand = read_route_candidates(p)
    assert cand["dark"] is False and cand["decision"] == NO_DECISION
    # task-level reqvec has no producer -> ABSENT, never fabricated.
    assert cand["task_reqvec"] == "absent"
    row = next(c for c in cand["candidates"] if c["routing_class"] == "source_python")
    assert row["in_keyspace"] is True and row["measured_events"] == 2
    # measured demand is the LATEST raw vector (4s), honest 5s not clipped by any sample-scaling.
    assert row["dispatch_reqvec"]["quality_floor"] == 4
    # NO display scalar minted anywhere in the projection.
    blob = json.dumps(cand).lower()
    assert "aggregate_score" not in blob and "\"score\"" not in blob and "rank" not in blob


def test_dark_when_feed_absent(tmp_path):
    post = read_route_posture(str(tmp_path / "nope.jsonl"))
    assert post["dark"] is True and post["decision"] == NO_DECISION
    cand = read_route_candidates(str(tmp_path / "nope.jsonl"))
    assert cand["dark"] is True and cand["candidates"] == [] and cand["task_reqvec"] == "absent"


def test_unknown_routing_class_is_a_drift_signal(tmp_path):
    p = str(tmp_path / "gate-events.jsonl")
    _write_events(p, [{"routing_class": "brand_new_class", "requirement_vector": {d: 2 for d in REQVEC_DIMS}}])
    post = read_route_posture(p)
    assert post["keyspace"]["unknown_observed"] == ["brand_new_class"]  # surfaced, not silently absorbed
