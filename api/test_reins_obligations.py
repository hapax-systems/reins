"""Tests for /read/obligations ligatory and capability-fulfillment synthesis."""

from __future__ import annotations

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
        allowlist=["kind", "state"],
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
