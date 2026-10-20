"""Tests for /read/obligations ligatory and capability-fulfillment synthesis."""

from __future__ import annotations

from fastapi.testclient import TestClient

from reins_obligations import read_obligations_summary
from reins_read import build_app, classify_air


def test_read_obligations_summary_synthesizes_owes_and_fulfillment() -> None:
    allowlist = ["kind", "relation", "state", "attention", "route_id", "capability_id", "mode", "model_id", "route_state"]
    lanebus = {
        "inboxes": [
            {"lane": "lane-alpha", "unacked_count": 3, "oldest_unacked_age_s": 120.5, "state": "unacked", "attention": "warning"},
            {"lane": "lane-beta", "unacked_count": 0, "state": "clear", "attention": "routine"},
        ]
    }
    grants = {
        "rows": [
            {"grant_id": "g-1", "lane": "lane-alpha", "route_id": "r.local", "state": "unmatched_receipt", "attention": "warning"},
            {"grant_id": "g-2", "lane": "lane-beta", "route_id": "r.cloud", "state": "reconciled", "attention": "routine"},
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
            {"role": "lane-gamma", "session": "sess-gamma", "pane_state": "absent", "claims": {"present": True}},
        ]
    }
    capabilities = {
        "routes": [
            {"route_id": "r.local", "capability_id": "cap.code", "mode": "local", "model_id": "qwen3.8-flash-next", "route_state": "ok"},
            {"route_id": "r.disk", "capability_id": "cap.embed", "mode": "local", "model_id": "bge-m3", "route_state": "ok"},
            {"route_id": "r.cloud", "capability_id": "cap.review", "mode": "metered", "model_id": "claude-opus-4-7", "route_state": "ok"},
        ]
    }
    context_bundle = {
        "facts": [
            {"id": "host:node-dark", "kind": "host", "state": "dark", "payload": {"host": "node-dark"}},
            {
                "id": "locus:node-a:8000",
                "kind": "locus",
                "state": "live",
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
                "payload": {
                    "loaded_models": ["stray-unexplained-model"],
                    "resident_models": ["bge-m3:latest", "stray-unexplained-model"],
                    "admitted_by": [],
                },
            },
        ]
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


def test_read_obligations_endpoint_wired_in_app() -> None:
    app = build_app("", ["kind", "state", "relation"])
    client = TestClient(app)
    resp = client.get("/read/obligations")
    assert resp.status_code == 200
    body = resp.json()
    assert body["dark"] is False
    assert "summary" in body
    assert "lanebus_owes" in body
    assert "capability_fulfillment" in body
