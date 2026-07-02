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
    # drift pin — now a REAL cross-package contract check (was self-referential len==11): reins's pinned
    # keyspace must EQUAL the hapax-spine wheel's ROUTING_CLASSES. A spine 11->17 expansion surfaces here
    # as a wheel-version bump the test catches, never silent drift across the seam.
    from hapax.spine.edt_measure import ROUTING_CLASSES

    assert tuple(ROUTING_CLASSES_PINNED) == tuple(ROUTING_CLASSES)
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
    # NO display scalar minted anywhere in EITHER projection (candidates + posture).
    for blob in (json.dumps(read_route_candidates(p)).lower(), json.dumps(read_route_posture(p)).lower()):
        assert "aggregate_score" not in blob
        assert "\"score\"" not in blob and "_score" not in blob
        assert "rank" not in blob and "posterior" not in blob


def test_empty_or_partial_reqvec_renders_absent_not_null_dict(tmp_path):
    # A2.3 honesty: an empty/partial requirement_vector (the live `verification` row carries {}) must
    # render the word "absent", NEVER an 8-key null-dict (a null masquerading as measured).
    p = str(tmp_path / "gate-events.jsonl")
    _write_events(p, [
        {"routing_class": "verification", "requirement_vector": {}},  # empty -> absent
        {"routing_class": "coordination", "requirement_vector": {"quality_floor": 5}},  # partial -> absent
        {"routing_class": "runtime_ops"},  # no vector key at all -> absent
        {"routing_class": "source_python", "requirement_vector": {d: 2 for d in REQVEC_DIMS}},  # complete
    ])
    cand = read_route_candidates(p)
    by = {c["routing_class"]: c["dispatch_reqvec"] for c in cand["candidates"]}
    assert by["verification"] == "absent", "empty {} must render absent, not a null-dict"
    assert by["coordination"] == "absent", "partial vector must render absent"
    assert by["runtime_ops"] == "absent", "missing vector must render absent"
    assert by["source_python"] == {d: 2 for d in REQVEC_DIMS}, "complete vector renders measured"
    # never a null value anywhere in the projection (the absent-ambiguity A2.3 forbids)
    assert "null" not in json.dumps(cand) and None not in _flatten(cand)


def _flatten(obj):
    out = []
    if isinstance(obj, dict):
        for v in obj.values():
            out.extend(_flatten(v))
    elif isinstance(obj, list):
        for v in obj:
            out.extend(_flatten(v))
    else:
        out.append(obj)
    return out


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
