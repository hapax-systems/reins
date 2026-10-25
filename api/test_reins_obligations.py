"""Tests for /read/obligations ligatory and capability-fulfillment synthesis."""

from __future__ import annotations

import pytest

from reins_obligations import REDACTION_TOKEN, _classified, read_obligations_summary
from reins_read import classify_air


def test_deny_all_classifier_and_secret_scrubbing() -> None:
    """Residual risk #3, R2-F01 & R3-F01: both custom/selective AND default classify_air redact denied opaque fields, and secret/canary markers never survive."""
    # Direct R3-F01 check on _classified with default classify_air
    direct = _classified(
        {"kind": "x", "debtor": "v7q2z9-private-fixture"},
        ["kind"],
        classify_air,
    )
    assert direct["kind"] == "x"
    assert direct["air"]["kind"] == "ok"
    assert direct["debtor"] == REDACTION_TOKEN
    assert direct["air"]["debtor"] == "deny"

    res = read_obligations_summary(
        allowlist=["state"],
        classify=lambda fields, _allow: {
            k: ("ok" if k == "state" else "deny") for k in fields
        },
        lanebus={
            "inboxes": [
                {
                    "lane": "lane-secret_marker_123",
                    "unacked_count": 1,
                    "state": "unacked",
                }
            ]
        },
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={
            "routes": [
                {
                    "route_id": "r.local",
                    "capability_id": "cap",
                    "mode": "local",
                    "model_id": "vendor-a/m1",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={
            "generated_at": "2026-10-20T12:10:00Z",
            "facts": [
                {
                    "id": "locus:h1:8000",
                    "kind": "locus",
                    "state": "live",
                    "observed_at": "2026-10-20T12:09:50Z",
                    "payload": {
                        "host": "h1",
                        "port": "8000",
                        "loaded_models": ["vendor-a/m1"],
                        "resident_models": ["vendor-a/m1"],
                        "admitted_by": ["r.local"],
                    },
                }
            ],
        },
        now="2026-10-20T12:10:05Z",
    )
    row = res["lanebus_owes"][0]
    assert row["debtor"] == REDACTION_TOKEN
    assert row["subject"] == REDACTION_TOKEN
    assert row["state"] == "unacked"
    locus_row = res["local_substrate_loci"][0]
    assert locus_row["host"] == REDACTION_TOKEN
    assert locus_row["loaded_models"] == [REDACTION_TOKEN]
    cap_row = res["capability_fulfillment"][0]
    assert cap_row["occupied_loci"] == [REDACTION_TOKEN]
    assert cap_row["model_id"] == REDACTION_TOKEN
    assert "secret_marker" not in str(res)

    # Also verify default classify_air in read_obligations_summary redacts arbitrary non-allowlisted opaque strings
    default_air_res = read_obligations_summary(
        allowlist=["kind", "state", "action_count"],
        classify=classify_air,
        lanebus={
            "inboxes": [
                {
                    "lane": "v7q2z9-private-fixture",
                    "unacked_count": 2,
                    "state": "unacked",
                    "attention": "action",
                }
            ]
        },
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={"routes": []},
        context_bundle={"facts": []},
    )
    assert default_air_res["lanebus_owes"][0]["debtor"] == REDACTION_TOKEN
    assert default_air_res["lanebus_owes"][0]["subject"] == REDACTION_TOKEN
    assert default_air_res["summary"]["action_count"] == 1


def _locus_summary(freshness="fresh", loaded=True):
    ts = "2026-10-20T12:10:00Z"
    return read_obligations_summary(
        allowlist=["state", "loaded_models", "local_loci_occupied_in_memory"],
        classify=classify_air,
        lanebus={},
        grants={},
        claim_custody={},
        seat_succession={},
        capabilities={"routes": [{"route_id": "r.local", "model_id": "m1"}]},
        context_bundle={
            "generated_at": ts,
            "facts": [
                {
                    "id": "locus:fixture:8000",
                    "kind": "locus",
                    "state": "live",
                    "freshness_state": freshness,
                    "observed_at": ts,
                    "payload": {
                        "loaded_models": ["m1"] if loaded else [],
                        "resident_models": ["m1"],
                    },
                }
            ],
        },
        now=ts,
    )


@pytest.mark.parametrize(
    "loaded,field", [(True, "occupied_loci"), (False, "resident_loci")]
)
def test_default_air_denies_locus_identifiers(loaded, field):
    result = _locus_summary(loaded=loaded)
    row = result["capability_fulfillment"][0]
    assert row[field] == [REDACTION_TOKEN]
    assert row["air"][field] == "deny"


@pytest.mark.parametrize("value", [7, 1.5, True])
def test_denied_scalars_and_nested_values(value):
    fields = {"action_count": value, "nested": {"value": value}}
    result = _classified(fields, ["state"], classify_air)
    assert result == {
        "action_count": None,
        "nested": {"value": None},
        "air": {"action_count": "deny", "nested": "deny"},
    }


@pytest.mark.parametrize(
    "freshness", ["fresh", "dark", "unknown", "stale", "aging", "absent"]
)
def test_only_fresh_loci_are_occupied(freshness):
    result = _locus_summary(freshness)
    live = freshness == "fresh"
    assert result["summary"]["local_loci_occupied_in_memory"] == int(live)
    row = result["local_substrate_loci"][0]
    assert row["loaded_models"] == (["m1"] if live else [])
    assert (row["state"] == "live") == live
