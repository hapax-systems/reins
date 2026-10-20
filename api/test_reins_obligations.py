"""Tests for /read/obligations ligatory and capability-fulfillment synthesis."""

from __future__ import annotations

from fastapi.testclient import TestClient

from reins_obligations import REDACTION_TOKEN, _classified, read_obligations_summary
from reins_read import build_app, classify_air


def test_read_obligations_summary_synthesizes_owes_and_fulfillment() -> None:
    allowlist = [
        "kind",
        "relation",
        "state",
        "attention",
        "route_id",
        "capability_id",
        "mode",
        "model_id",
        "route_state",
    ]
    lanebus = {
        "inboxes": [
            {
                "lane": "lane-alpha",
                "unacked_count": 3,
                "oldest_unacked_age_s": 120.5,
                "state": "unacked",
                "attention": "warning",
            },
            {
                "lane": "lane-beta",
                "unacked_count": 0,
                "state": "clear",
                "attention": "routine",
            },
        ]
    }
    grants = {
        "rows": [
            {
                "grant_id": "g-1",
                "lane": "lane-alpha",
                "route_id": "r.local",
                "state": "unmatched_receipt",
                "attention": "warning",
            },
            {
                "grant_id": "g-2",
                "lane": "lane-beta",
                "route_id": "r.cloud",
                "state": "reconciled",
                "attention": "routine",
            },
        ]
    }
    claim_custody = {
        "rows": [
            {"task_id": "task-1", "holder": "lane-alpha", "custody_state": "stale"},
            {"task_id": "task-2", "holder": "lane-beta", "custody_state": "settled"},
        ]
    }
    seat_succession = {
        "rows": [
            {
                "role": "lane-gamma",
                "session": "sess-gamma",
                "pane_state": "absent",
                "claims": {"present": True},
            },
        ]
    }
    capabilities = {
        "routes": [
            {
                "route_id": "r.local",
                "capability_id": "cap.code",
                "mode": "local",
                "model_id": "qwen3.8-flash-next",
                "route_state": "ok",
            },
            {
                "route_id": "r.disk",
                "capability_id": "cap.embed",
                "mode": "local",
                "model_id": "bge-m3",
                "route_state": "ok",
            },
            {
                "route_id": "r.cloud",
                "capability_id": "cap.review",
                "mode": "metered",
                "model_id": "claude-opus-4-7",
                "route_state": "ok",
            },
        ]
    }
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": [
            {
                "id": "host:node-dark",
                "kind": "host",
                "state": "dark",
                "observed_at": "2026-10-20T12:09:50Z",
                "payload": {"host": "node-dark"},
            },
            {
                "id": "locus:node-a:8000",
                "kind": "locus",
                "state": "live",
                "observed_at": "2026-10-20T12:09:50Z",
                "payload": {
                    "loaded_models": ["qwen3.8-flash-next"],
                    "resident_models": ["qwen3.8-flash-next"],
                    "admitted_by": ["r.local (active)"],
                    "topology": "tp=2,nnodes=2",
                },
            },
            {
                "id": "locus:node-b:11434",
                "kind": "locus",
                "state": "live",
                "observed_at": "2026-10-20T12:09:50Z",
                "payload": {
                    "loaded_models": ["stray-unexplained-model"],
                    "resident_models": ["bge-m3:latest", "stray-unexplained-model"],
                    "admitted_by": [],
                },
            },
        ],
    }

    res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus=lanebus,
        grants=grants,
        claim_custody=claim_custody,
        seat_succession=seat_succession,
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )

    assert res["dark"] is False
    assert res["summary"]["total_owes"] == 4
    assert res["summary"]["lanebus_owes_count"] == 1
    assert res["summary"]["grant_owes_count"] == 1
    assert res["summary"]["custody_owes_count"] == 2
    assert res["summary"]["locus_anomaly_count"] == 2
    assert res["summary"]["routes_total"] == 3
    assert res["summary"]["routes_occupied_in_memory"] == 1

    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["r.local"]["fulfillment_state"] == "occupied_in_memory"
    assert by_route["r.local"]["occupied_loci"] == ["locus:node-a:8000"]
    assert by_route["r.local"]["topology"] == "tp=2,nnodes=2"
    assert by_route["r.disk"]["fulfillment_state"] == "resident_on_disk_only"
    assert by_route["r.disk"]["resident_loci"] == ["locus:node-b:11434"]
    assert by_route["r.cloud"]["fulfillment_state"] == "admitted_remote_or_cloud"


def test_all_sources_dark_reports_dark_and_source_anomalies() -> None:
    """F01: when all six input facets are dark, obligations must report dark=True and surface source.dark anomalies."""
    dark_facet = {"dark": True}
    res = read_obligations_summary(
        allowlist=["kind", "state", "attention"],
        classify=classify_air,
        lanebus=dark_facet,
        grants=dark_facet,
        claim_custody=dark_facet,
        seat_succession=dark_facet,
        capabilities=dark_facet,
        context_bundle=dark_facet,
    )
    assert res["dark"] is True
    assert res["summary"]["dark_sources_count"] == 6
    assert len(res["locus_anomalies"]) == 6
    assert all(a["kind"] == "source.dark" for a in res["locus_anomalies"])


def test_stale_locus_and_blocked_route_do_not_claim_active_memory_fulfillment() -> None:
    """F01: a stale locus or a blocked route must never report occupied_in_memory."""
    allowlist = ["route_id", "fulfillment_state", "route_state", "kind", "state"]
    capabilities = {
        "routes": [
            {
                "route_id": "r.stale",
                "capability_id": "cap.a",
                "mode": "local",
                "model_id": "m-stale",
                "route_state": "ok",
            },
            {
                "route_id": "r.blocked",
                "capability_id": "cap.b",
                "mode": "local",
                "model_id": "m-live",
                "route_state": "blocked",
            },
        ]
    }
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": [
            {
                "id": "locus:stale:8000",
                "kind": "loaded_model",
                "observed_at": "2026-10-20T11:30:00Z",
                "freshness_state": "stale",
                "state": {"value_state": "stale", "reason_codes": []},
                "value": {
                    "host": "node-a",
                    "endpoint": "8000",
                    "model": "m-stale",
                    "admitting_row": "r.stale",
                },
            },
            {
                "id": "locus:live:8001",
                "kind": "loaded_model",
                "observed_at": "2026-10-20T12:09:50Z",
                "freshness_state": "fresh",
                "state": {"value_state": "lit", "reason_codes": []},
                "value": {
                    "host": "node-a",
                    "endpoint": "8001",
                    "model": "m-live",
                    "admitting_row": "r.blocked",
                },
            },
        ],
    }
    res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus={"inboxes": []},
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )
    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["r.stale"]["fulfillment_state"] == "stale_locus"
    assert by_route["r.stale"]["occupied_loci"] == []
    assert by_route["r.blocked"]["fulfillment_state"] == "route_blocked_locus_occupied"
    assert res["summary"]["routes_occupied_in_memory"] == 0


def test_strict_identity_joins_reject_fuzzy_substrings_and_unfulfilled_local_routes() -> (
    None
):
    """F03 + F08: route.a must not match route.a2; qwen3 must not match qwen3-embedding-8b; local unfulfilled route is not cloud."""
    allowlist = ["route_id", "fulfillment_state", "route_state"]
    capabilities = {
        "routes": [
            {
                "route_id": "route.a",
                "mode": "local",
                "model_id": "qwen3",
                "route_state": "ok",
            },
            {
                "route_id": "route.disk",
                "mode": "local",
                "model_id": "ornith-35b",
                "route_state": "ok",
            },
        ]
    }
    # Pass grouped producer facts dict directly (residual risk #4 + F08 disk residency on resource_vram)
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": {
            "resource_vram": [
                {
                    "fact_id": "resource_vram:host:node-a",
                    "fact_type": "resource_vram",
                    "subject_ref": "host:node-a",
                    "observed_at": "2026-10-20T12:09:50Z",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "node-a",
                        "total_mb": 64000,
                        "avail_mb": 32000,
                        "ollama_disk_resident_models": ["ornith-35b:latest"],
                    },
                }
            ],
            "loaded_model": [
                {
                    "fact_id": "loaded_model:host:node-a:8000/qwen3-embedding-8b",
                    "fact_type": "loaded_model",
                    "subject_ref": "host:node-a:8000/qwen3-embedding-8b",
                    "observed_at": "2026-10-20T12:09:50Z",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "node-a",
                        "endpoint": "8000",
                        "model": "qwen3-embedding-8b",
                        "admitting_row": "route.a2 (active)",
                    },
                }
            ],
        },
    }
    res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus={"inboxes": []},
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )
    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["route.a"]["fulfillment_state"] == "unfulfilled_local_locus"
    assert by_route["route.disk"]["fulfillment_state"] == "resident_on_disk_only"


def test_producer_lost_endpoint_and_partial_vram_contracts() -> None:
    """F04: declared_endpoint with value_state='absent' and reason_code='LOST' emits endpoint.lost action; partial VRAM emits host.partial_observation."""
    allowlist = ["kind", "state", "attention"]
    context_bundle = {
        "facts": [
            {
                "fact_id": "declared_endpoint:serving.node-a-5000",
                "fact_type": "declared_endpoint",
                "subject_ref": "serving.node-a-5000",
                "state": {
                    "value_state": "absent",
                    "reason_codes": ["LOST", "declared_endpoint_dead"],
                },
                "value": {"port": "5000", "alive": False},
            },
            {
                "fact_id": "resource_vram:host:node-a/RTX",
                "fact_type": "resource_vram",
                "subject_ref": "host:node-a/RTX",
                "state": {
                    "value_state": "partial",
                    "reason_codes": ["per_card_free_util_absent_backcompat_gpus"],
                },
                "value": {"card": "RTX", "used": "100 MiB"},
            },
        ]
    }
    res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus={"inboxes": []},
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={"routes": []},
        context_bundle=context_bundle,
    )
    kinds = {a["kind"]: a for a in res["locus_anomalies"]}
    assert "endpoint.lost" in kinds
    assert kinds["endpoint.lost"]["attention"] == "action"
    assert res["summary"]["action_count"] == 1
    assert "host.partial_observation" in kinds
    assert "host.dark" not in kinds


def test_explicit_zero_unacked_and_malformed_counters_handled_safely() -> None:
    """F10: unacked_count=0 must not fall back to unread_count=3; malformed counters must not raise ValueError."""
    allowlist = ["kind", "state", "count"]
    lanebus = {
        "inboxes": [
            {
                "lane": "lane-zero",
                "unacked_count": 0,
                "unread_count": 3,
                "state": "clear",
            },
            {"lane": "lane-bad", "unacked_count": "bad-counter-value", "state": "ok"},
        ]
    }
    res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus=lanebus,
        grants={
            "rows": [
                {
                    "grant_id": "g-bad",
                    "honor_count": "not-an-int",
                    "grant_record_present": True,
                }
            ]
        },
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={"routes": []},
        context_bundle={
            "facts": [
                {
                    "id": "h1",
                    "kind": "host",
                    "state": "live",
                    "observed_at": "2026-10-20T12:09:50Z",
                }
            ]
        },
        now="2026-10-20T12:10:00Z",
    )
    assert len(res["lanebus_owes"]) == 1
    assert res["lanebus_owes"][0]["kind"] == "lanebus.unparseable"
    assert len(res["grant_owes"]) == 1
    assert res["grant_owes"][0]["kind"] == "grant.unparseable_honor_count"


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


def test_vendor_namespace_isolation_and_dark_source_freshness_guards() -> None:
    """R2-F04, R2-F06, R2-F07, R2-F09, R3-F02: vendor-a/model-x != vendor-b/model-x, dark context_bundle yields 0 live loci, ancient bundles or missing observed_at never claim occupied_in_memory, timestamp-absent alive endpoint emits endpoint.unverified_freshness, and negative/huge ints are rejected."""
    allowlist = ["route_id", "fulfillment_state", "kind", "state", "attention", "count"]
    capabilities = {
        "routes": [
            {
                "route_id": "r.vendor.b",
                "mode": "local",
                "model_id": "vendor-b/model-x",
                "route_state": "ok",
            },
        ]
    }
    context_bundle = {
        "generated_at": "2026-10-20T12:10:00Z",
        "facts": [
            {
                "fact_id": "loaded_model:host:n1:8000/vendor-a/model-x",
                "fact_type": "loaded_model",
                "subject_ref": "host:n1:8000/vendor-a/model-x",
                "observed_at": "2026-10-20T12:09:50Z",
                "freshness_state": "fresh",
                "state": {"value_state": "lit", "reason_codes": []},
                "value": {
                    "host": "n1",
                    "endpoint": "8000",
                    "model": "vendor-a/model-x",
                    "admitting_row": "r.vendor.a",
                },
            },
            {
                "fact_id": "declared_endpoint:serving.n1-8000",
                "fact_type": "declared_endpoint",
                "subject_ref": "serving.n1-8000",
                "freshness_state": "absent",
                "state": {
                    "value_state": "absent",
                    "reason_codes": ["observation_timestamp_absent"],
                },
                "value": {"host": "n1", "port": "8000", "alive": True},
            },
        ],
    }
    res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus={
            "inboxes": [
                {"lane": "neg", "unacked_count": -5, "state": "ok"},
                {"lane": "huge", "unacked_count": "9" * 50, "state": "ok"},
            ]
        },
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities=capabilities,
        context_bundle=context_bundle,
        now="2026-10-20T12:10:05Z",
    )
    by_route = {r["route_id"]: r for r in res["capability_fulfillment"]}
    assert by_route["r.vendor.b"]["fulfillment_state"] == "unfulfilled_local_locus"
    kinds = {a["kind"]: a for a in res["locus_anomalies"]}
    assert "endpoint.unverified_freshness" in kinds
    assert "endpoint.lost" not in kinds
    assert len(res["lanebus_owes"]) == 2
    assert all(x["kind"] == "lanebus.unparseable" for x in res["lanebus_owes"])

    # R3-F02: Ancient internally-consistent bundle (generated_at and observed_at both 2000-01-01, marked fresh) must re-age against current read time to stale_locus
    ancient_res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus={"inboxes": []},
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={
            "routes": [
                {
                    "route_id": "r.vendor.a",
                    "mode": "local",
                    "model_id": "vendor-a/model-x",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={
            "generated_at": "2000-01-01T00:00:00Z",
            "facts": [
                {
                    "fact_id": "loaded_model:host:n1:8000/vendor-a/model-x",
                    "fact_type": "loaded_model",
                    "subject_ref": "host:n1:8000/vendor-a/model-x",
                    "observed_at": "2000-01-01T00:00:00Z",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "n1",
                        "endpoint": "8000",
                        "model": "vendor-a/model-x",
                        "admitting_row": "r.vendor.a",
                    },
                }
            ],
        },
        now="2026-10-20T12:10:05Z",
    )
    assert (
        ancient_res["capability_fulfillment"][0]["fulfillment_state"] == "stale_locus"
    )
    assert ancient_res["summary"]["routes_occupied_in_memory"] == 0

    # R3-F02: Explicit freshness_state='fresh' without observed_at must be rejected as absent
    no_ts_res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus={"inboxes": []},
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={
            "routes": [
                {
                    "route_id": "r.vendor.a",
                    "mode": "local",
                    "model_id": "vendor-a/model-x",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={
            "facts": [
                {
                    "fact_id": "loaded_model:host:n1:8000/vendor-a/model-x",
                    "fact_type": "loaded_model",
                    "subject_ref": "host:n1:8000/vendor-a/model-x",
                    "freshness_state": "fresh",
                    "state": {"value_state": "lit", "reason_codes": []},
                    "value": {
                        "host": "n1",
                        "endpoint": "8000",
                        "model": "vendor-a/model-x",
                        "admitting_row": "r.vendor.a",
                    },
                }
            ],
        },
        now="2026-10-20T12:10:05Z",
    )
    assert (
        no_ts_res["capability_fulfillment"][0]["fulfillment_state"]
        == "unfulfilled_local_locus"
    )
    assert no_ts_res["summary"]["routes_occupied_in_memory"] == 0

    # Dark context_bundle with residual facts must never mark routes occupied_in_memory
    dark_ctx_res = read_obligations_summary(
        allowlist=allowlist,
        classify=classify_air,
        lanebus={"inboxes": []},
        grants={"rows": []},
        claim_custody={"rows": []},
        seat_succession={"rows": []},
        capabilities={
            "routes": [
                {
                    "route_id": "r.vendor.a",
                    "mode": "local",
                    "model_id": "vendor-a/model-x",
                    "route_state": "ok",
                }
            ]
        },
        context_bundle={"dark": True, "facts": context_bundle["facts"]},
        now="2026-10-20T12:10:05Z",
    )
    assert dark_ctx_res["summary"]["routes_occupied_in_memory"] == 0
    assert dark_ctx_res["summary"]["local_loci_occupied_in_memory"] == 0


def test_read_obligations_endpoint_wired_in_app() -> None:
    app = build_app("", ["kind", "state", "relation"])
    client = TestClient(app)
    resp = client.get("/read/obligations")
    assert resp.status_code == 200
    body = resp.json()
    assert isinstance(body["dark"], bool)
    assert "source_status" in body
    assert "summary" in body
    assert "lanebus_owes" in body
    assert "capability_fulfillment" in body
