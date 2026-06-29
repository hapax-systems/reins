import json
import os
from pathlib import Path
from urllib.parse import quote

from fastapi.testclient import TestClient

import reins_read
from reins_read import (
    _page_before,
    _raw_sessions,
    to_trace_row,
    _route_binding_index,
    _session_route_binding,
    _session_sort_key,
    score_event,
    classify_air,
    to_event,
    to_task,
    to_node,
    to_edge,
    to_session,
    to_session_detail,
    read_intake_summary,
    read_capability_summary,
    _capability_surface_pack_rows,
    read_dynamics_package,
    read_epistemics,
    read_lifecycle_registry_summary,
    read_domain_pack_summary,
    read_gate_summary,
    read_vault_summary,
    read_observe_summary,
    build_app,
    instance_config,
)


EXPECTED_DEFAULT_ALLOW = [
    "kind", "score", "ts", "task_id", "stage", "no_go",
    "id", "layer", "status", "source", "target", "relation", "res",
    "role", "platform", "state", "alive", "idle", "stalled", "output_age_s", "relay_age_s",
    "readiness", "blocker", "attention",
    "evidence_count", "resume_ready",
    "evidence_summary", "by_kind", "transcript_roots_observed", "transcript_roots_missing", "truncated",
    "count", "age_bucket", "coverage", "task_link_state", "severity", "privacy", "raw_access", "exists",
    "capability_id", "capability_class", "surface_family", "spend_model", "egress_class", "receipt_requirement",
    "route_count", "ok_count", "blocked_count", "hkp_posture", "source_refs", "source_ref_labels",
    "route_id", "mode", "profile", "model_id", "effort", "context_mode", "fast_mode", "quantization", "capacity_pool", "demand_vector", "hardening", "eval_plane", "review_obligation", "learning_eligibility", "benchmark_coverage", "fixed_overhead", "route_state", "authority_ceiling",
    "freshness_ok", "quota_state", "receipt_count", "blockers", "authority",
    "route_binding_state",
    "tool_id", "available", "authority_use", "observed_at", "stale_after",
    "schema_version", "row_id", "family", "subject_kind", "subject_ref", "posture", "map_kind", "map_id", "map_source", "map_target", "map_relation",
    "gate_id", "domain", "evidence", "missing", "action",
    "detail", "generated_at", "package_hash", "default_lens",
    "domain_id", "lifecycle", "terrain", "depth", "scope", "claim_ceiling", "windows", "surfaces", "parity", "source_refs",
    "lifecycle_id", "owner", "plant", "posture", "maturity", "adapter_id", "claim_surface", "mutation_surface", "dark_policy", "freshness_policy", "air_class", "commands", "receipt_contracts", "next_evidence",
]


def test_score_recency_and_kind():
    a = score_event({"kind": "pr.merged"}, age_s=5)
    b = score_event({"kind": "review.fail"}, age_s=5)  # escalation outranks
    assert 0.0 <= a <= 1.0 and b > a


def test_to_trace_row_folds_langfuse_shape_without_pii():
    # the LLM-trace fold (WS#2): a Langfuse trace -> an operational Reins row with
    # model/tokens/cost/latency. Input/output (operator content = PII) NEVER enter the row.
    row = to_trace_row(
        {
            "id": "trace-1",
            "timestamp": "2026-06-26T12:00:00Z",
            "metadata": {"model": "claude-opus-4"},
            "tokenUsage": {"prompt": 100, "completion": 50, "total": 150},
            "totalPrice": 0.012,
            "duration": 2.5,
            "input": "secret operator prompt",
            "output": "secret completion",
        },
        EXPECTED_DEFAULT_ALLOW,
    )
    assert row["trace_id"] == "trace-1"
    assert row["model"] == "claude-opus-4"
    assert row["prompt_tok"] == 100 and row["completion_tok"] == 50 and row["total_tok"] == 150
    assert row["cost"] == 0.012
    assert row["latency_ms"] == 2500  # duration seconds -> ms
    # PII guard: prompt/completion text must never enter the operational row
    assert "input" not in row and "output" not in row
    assert "secret" not in str(row)


def test_page_before_returns_strictly_older_window():
    # scrollback cursor: page backward through ordered events by timestamp. The coord
    # spine replays oldest->newest; _page_before returns up to `limit` events strictly
    # older than `before` (or the newest `limit` when before is None).
    def _ev(ts, subject):
        return {"timestamp": ts, "event_type": "coord.test", "subject": subject,
                "actor": "alpha", "payload": {}}

    events = [
        _ev("2026-06-26T10:00:00Z", "t1"),
        _ev("2026-06-26T11:00:00Z", "t2"),
        _ev("2026-06-26T12:00:00Z", "t3"),
        _ev("2026-06-26T13:00:00Z", "t4"),
        _ev("2026-06-26T14:00:00Z", "t5"),
    ]
    # no cursor: newest `limit`, oldest->newest order preserved
    assert [p["subject"] for p in _page_before(events, before=None, limit=2)] == ["t4", "t5"]
    # cursor at t4's ts: strictly-older events, newest `limit` of them
    assert [p["subject"] for p in _page_before(events, before="2026-06-26T13:00:00Z", limit=2)] == ["t2", "t3"]
    # page again from t2's ts
    assert [p["subject"] for p in _page_before(events, before="2026-06-26T11:00:00Z", limit=2)] == ["t1"]
    # before older than everything -> empty (no fabrication)
    assert _page_before(events, before="2026-06-26T10:00:00Z", limit=2) == []
    # limit larger than the strictly-older set returns all of it
    assert [p["subject"] for p in _page_before(events, before="2026-06-26T14:00:00Z", limit=99)] == ["t1", "t2", "t3", "t4"]


def test_air_is_default_deny():
    air = classify_air({"subject": "4284", "summary": "secret name"}, allowlist=["subject"])
    assert air["subject"] == "ok" and air["summary"] == "deny"


def test_to_event_shape():
    raw = {"type": "coord.pr.merged", "subject": "4284", "actor": "alpha",
           "payload": {"summary": "PR#4284 merged"}, "ts": "2026-06-24T14:22:00Z"}
    ev = to_event(raw, allowlist=["kind", "subject"], age_s=2)
    assert ev["kind"] == "pr.merged" and ev["subject"] == "4284"
    assert ev["air"]["actor"] == "deny" and ev["air"]["subject"] == "ok"


def test_read_session_turns_returns_typed_receipts_with_bimodal_air():
    app = build_app("", EXPECTED_DEFAULT_ALLOW)
    endpoint = next(route.endpoint for route in app.routes if getattr(route, "path", "") == "/read/session/{role}/turns")

    data = endpoint("cc-reins", None, 2)
    assert data["dark"] is False and data["error"] == ""
    assert data["oldest_ts"] == "2026-06-26T18:40:05Z"
    assert len(data["turns"]) == 2

    turn = data["turns"][0]
    assert set(turn) == {"ts", "role", "kind", "prov", "summary", "magnitude", "model", "route", "gate", "air"}
    for field in ("ts", "role", "kind", "magnitude", "model", "route", "gate"):
        assert turn["air"][field] == "ok"
    assert turn["summary"] == "fixture assistant response body"
    assert turn["air"]["summary"] == "deny"
    assert turn["prov"] == "model" and turn["air"]["prov"] == "ok"

    # Cursor pages strictly older than `before`; the operator receipt proves prov class forcing.
    data = endpoint("cc-reins", "2026-06-26T18:40:05Z", 1)
    assert data["oldest_ts"] == "2026-06-26T18:40:01Z"
    assert data["turns"][0]["prov"] == "operator"
    assert data["turns"][0]["air"]["prov"] == "deny"


def test_read_session_turns_unknown_role_is_honest_dark():
    app = build_app("", EXPECTED_DEFAULT_ALLOW)
    endpoint = next(route.endpoint for route in app.routes if getattr(route, "path", "") == "/read/session/{role}/turns")

    data = endpoint("no-such-role", None, 80)
    assert data["dark"] is True
    assert data["turns"] == []
    assert data["oldest_ts"] == ""
    assert "no turn replay fixture" in data["error"]


def test_read_session_turn_blocks_are_honest_empty_for_any_role_and_ts():
    app = build_app("", EXPECTED_DEFAULT_ALLOW)
    endpoint = next(
        route.endpoint
        for route in app.routes
        if getattr(route, "path", "") == "/read/session/{role}/turns/{ts}/blocks"
    )

    for role, ts in (
        ("cc-reins", "2026-06-26T18:40:05Z"),
        ("no-such-role", "not-a-fixture-turn"),
    ):
        data = endpoint(role, ts)
        assert data["dark"] is True
        assert data["blocks"] == []
        assert "no turn block stream" in data["error"]


def test_to_task_shape_and_air():
    t = {"task_id": "x-1", "stage": "S6", "authority_case": "CASE-1", "no_go": {"blocked": True, "ok": False}}
    out = to_task("x-1", t, allowlist=["task_id", "stage", "no_go"])
    assert out["task_id"] == "x-1" and out["stage"] == "S6" and out["no_go"] == "blocked"
    assert out["air"]["task_id"] == "ok" and out["air"]["authority_case"] == "deny"


def test_to_node_shape_and_air():
    n = {"id": "rdf-owl-kg", "label": "RDF / OWL KG", "kind": "backbone",
         "layer": "semantic-backbone", "status": "asserted", "resolution": 1,
         "summary": "Canonical semantic backbone",
         "context": "Identity and evidence stay separate.",
         "docs": [{"label": "W3C RDF", "url": "SECRET_URL"}],
         "hardening": ["Use named graphs"],
         "aliases": ["knowledge graph"],
         "tags": ["semantic-core"]}
    out = to_node(n, allowlist=["id", "layer", "status"])
    assert out["id"] == "rdf-owl-kg" and out["layer"] == "semantic-backbone" and out["status"] == "asserted"
    assert out["summary"] == "Canonical semantic backbone"
    assert out["docs"] == "W3C RDF"
    assert out["source_refs"] == "docs:1 refs"
    assert out["source_ref_labels"] == ["W3C RDF"]
    assert out["hardening_notes"] == "Use named graphs"
    assert "SECRET_URL" not in json.dumps(out)
    assert out["air"]["id"] == "ok" and out["air"]["kind"] == "deny"  # kind not allowlisted -> deny
    assert out["air"]["summary"] == "deny" and out["air"]["docs"] == "deny"
    assert out["air"]["source_refs"] == "deny" and out["air"]["source_ref_labels"] == "deny"


def test_to_edge_shape_and_air():
    e = {"id": "dmn-to-drd", "source": "dmn", "target": "drd", "relation": "defines", "status": "asserted",
         "confidence": 1.0, "summary": "DMN includes DRDs.", "docs": [{"label": "OMG DMN", "url": "SECRET_URL", "source_ref": "docs/specs/omg-dmn.md#drd"}]}
    out = to_edge(e, allowlist=["source", "target", "relation", "source_refs", "source_ref_labels"])
    assert out["source"] == "dmn" and out["target"] == "drd" and out["relation"] == "defines"
    assert out["id"] == "dmn-to-drd" and out["confidence"] == "1.0" and out["docs"] == "OMG DMN"
    assert out["source_refs"] == "docs:1 refs"
    assert out["source_ref_labels"] == ["omg-dmn.md#drd"]
    assert "SECRET_URL" not in json.dumps(out)
    assert out["air"]["relation"] == "ok" and out["air"]["status"] == "deny"
    assert out["air"]["summary"] == "deny" and out["air"]["docs"] == "deny"
    assert out["air"]["source_refs"] == "ok" and out["air"]["source_ref_labels"] == "ok"


def test_read_dynamics_package_projects_metadata_without_raw_bodies(tmp_path):
    arch = tmp_path / "docs" / "architecture"
    arch.mkdir(parents=True)
    (arch / "system-dynamics-map.package.json").write_text(json.dumps({
        "authority_case": "CASE-DYN",
        "generated_at": "2026-06-18T00:00:00Z",
        "validation": {"package_gate": "scripts/system-dynamics-map-gate SECRET_RAW_COMMAND"},
        "artifacts": [{"path": "docs/architecture/system-dynamics-map.seed.json"}],
    }))
    (arch / "system-dynamics-map.view-manifest.json").write_text(json.dumps({
        "source_snapshot": {"node_count": 2, "edge_count": 1},
        "validation": {"pytest": "uv run pytest"},
        "workbench_contract": {
            "defaults": {
                "inquiry_mode": "release-gates",
                "audience_mode": "operator",
                "explanation_path": "release-readiness",
            },
            "inquiry_modes": [{
                "id": "release-gates",
                "label": "What gates release?",
                "lens": "operating-slice",
                "prompt": "SECRET_WORKBENCH_PROMPT",
                "answer_shape": ["ordered gate path", "scope caveat"],
                "focus_node_ids": ["sdlc-intake", "review-dossier"],
                "focus_edge_ids": ["review-dossier-to-pr-ci"],
            }],
            "audience_modes": [{
                "id": "operator",
                "label": "Operator",
                "emphasis": "diagnostic next action",
            }],
            "explanation_paths": [{
                "id": "release-readiness",
                "label": "Release readiness path",
                "summary": "Teach release readiness.",
                "must_include": ["what this does not prove"],
                "scene_count": 1,
                "scenes": [{
                    "title": "State what this does not prove",
                    "lens": "evidence-risk",
                    "selection": {"group": "nodes", "id": "view-manifest"},
                    "takeaway": "SECRET_WORKBENCH_TAKEAWAY",
                    "caveat": "SECRET_WORKBENCH_CAVEAT",
                }],
            }],
            "follow_on_tranches": ["bitemporal snapshot registry"],
        },
        "lenses": [{
            "id": "topology",
            "label": "Topology",
            "state_mode": "topology",
            "layout": "cose",
            "visible_node_count": 2,
            "visible_edge_count": 1,
            "aggregation": {"lossy": False, "reversible": True},
        }],
    }))
    (arch / "system-dynamics-map.lock.json").write_text(json.dumps({
        "package_hash": "abcdef1234567890",
        "generated_at": "2026-06-18T00:00:00Z",
    }))
    (arch / "system-dynamics-map.claims.json").write_text(json.dumps({
        "claims": [{
            "element_kind": "node",
            "claim_type": "asserted",
            "subject": "SECRET_CLAIM_SUBJECT",
            "provenance": {"authority_ceiling": "architecture_contract", "source_ref": "SECRET_REF"},
            "freshness": {"state": "timeless"},
        }]
    }))
    (arch / "system-dynamics-map.lenses.json").write_text(json.dumps({"default_lens": "topology", "lenses": []}))
    (arch / "system-dynamics-map.observations.jsonl").write_text(json.dumps({
        "state": "stale_fixture",
        "source_type": "fixture",
        "freshness": "stale",
        "evidence": [{"label": "SECRET_OBSERVATION_BODY"}],
    }) + "\n")
    (arch / "system-dynamics-map.relations.json").write_text(json.dumps({
        "relations": [{
            "category": "governance",
            "allowed_claim_types": ["asserted"],
            "edge_ids": ["edge-1"],
            "semantics": "SECRET_RELATION_BODY",
        }]
    }))
    (arch / "system-dynamics-map.canonical.trig").write_text("SECRET_TRIG_BODY")
    (arch / "system-dynamics-map.shacl.ttl").write_text("SECRET_SHACL_BODY")

    summary = read_dynamics_package(str(tmp_path), EXPECTED_DEFAULT_ALLOW)
    assert summary["totals"]["sources"] == 9
    assert summary["totals"]["claims"] == 1
    assert summary["totals"]["observations"] == 1
    assert summary["totals"]["relations"] == 1
    assert summary["authority_case"] == "CASE-DYN"
    assert summary["package_hash"] == "abcdef1234567890"
    assert summary["sources"][0]["air"]["path"] == "deny"
    assert summary["sources"][0]["privacy"] == "metadata-only"
    assert summary["sources"][0]["air"]["privacy"] == "ok"
    workbench = summary["workbench_contract"]
    assert workbench["status"] == "observed"
    assert workbench["defaults"]["inquiry_mode"] == "release-gates"
    assert workbench["defaults"]["audience_mode"] == "operator"
    assert workbench["inquiry_modes"][0]["label"] == "What gates release?"
    assert workbench["inquiry_modes"][0]["air"]["prompt"] == "deny"
    assert workbench["audience_modes"][0]["label"] == "Operator"
    assert workbench["explanation_paths"][0]["label"] == "Release readiness path"
    assert workbench["explanation_paths"][0]["scenes"][0]["air"]["takeaway"] == "deny"
    assert any(row["id"] == "topology" and row["status"] == "lossless" for row in summary["lenses"])
    dumped = json.dumps(summary)
    for secret in ["SECRET_CLAIM_SUBJECT", "SECRET_REF", "SECRET_OBSERVATION_BODY", "SECRET_RELATION_BODY", "SECRET_TRIG_BODY", "SECRET_SHACL_BODY"]:
        assert secret not in dumped


def test_read_dynamics_package_handles_missing_or_malformed_workbench(tmp_path):
    arch = tmp_path / "docs" / "architecture"
    arch.mkdir(parents=True)
    (arch / "system-dynamics-map.package.json").write_text("{}")
    (arch / "system-dynamics-map.lock.json").write_text("{}")
    (arch / "system-dynamics-map.view-manifest.json").write_text(json.dumps({}))
    summary = read_dynamics_package(str(tmp_path), EXPECTED_DEFAULT_ALLOW)
    assert summary["workbench_contract"]["status"] == "missing"
    assert summary["workbench_contract"]["missing"] == "workbench_contract"

    (arch / "system-dynamics-map.view-manifest.json").write_text(json.dumps({"workbench_contract": ["bad"]}))
    summary = read_dynamics_package(str(tmp_path), EXPECTED_DEFAULT_ALLOW)
    assert summary["workbench_contract"]["status"] == "malformed"
    assert summary["workbench_contract"]["missing"] == "workbench_contract"


def _write_epistemics_dynamics_fixture(root: Path) -> None:
    arch = root / "docs" / "architecture"
    arch.mkdir(parents=True)
    (arch / "system-dynamics-map.seed.json").write_text(json.dumps({
        "map_id": "dyn-map",
        "thesis": "SECRET MAP BODY",
        "layers": [{"id": "runtime", "label": "SECRET LAYER LABEL"}],
        "nodes": [{
            "id": "node-alpha",
            "label": "SECRET NODE LABEL",
            "kind": "capability",
            "layer": "runtime",
            "status": "asserted",
            "summary": "SECRET NODE SUMMARY",
            "docs": [{
                "label": "SECRET NODE DOC LABEL",
                "url": "https://secret.example/node-alpha",
                "source_ref": "docs/sources/node.md#node-alpha",
            }],
        }],
        "edges": [{
            "id": "edge-alpha-beta",
            "source": "node-alpha",
            "target": "node-beta",
            "relation": "depends_on",
            "status": "asserted",
            "summary": "SECRET EDGE SUMMARY",
            "docs": [{
                "label": "SECRET EDGE DOC LABEL",
                "url": "https://secret.example/edge-alpha-beta",
                "source_refs": ["docs/sources/edge.md#alpha-beta"],
            }],
        }],
    }))
    (arch / "system-dynamics-map.package.json").write_text(json.dumps({
        "authority_case": "CASE-DYN",
        "generated_at": "2026-06-25T12:00:00Z",
        "validation": {"package_gate": "scripts/gate SECRET_COMMAND"},
        "artifacts": [{"path": "docs/architecture/system-dynamics-map.seed.json"}],
    }))
    (arch / "system-dynamics-map.view-manifest.json").write_text(json.dumps({
        "source_snapshot": {"node_count": 1, "edge_count": 1},
        "lenses": [{
            "id": "topology",
            "label": "SECRET LENS LABEL",
            "state_mode": "topology",
            "layout": "cose",
            "visible_node_count": 1,
            "visible_edge_count": 1,
            "aggregation": {"lossy": False, "reversible": True},
        }],
    }))
    (arch / "system-dynamics-map.lock.json").write_text(json.dumps({
        "package_hash": "sha256:test-package",
        "generated_at": "2026-06-25T12:00:00Z",
    }))
    (arch / "system-dynamics-map.claims.json").write_text(json.dumps({
        "claims": [{
            "element_kind": "node",
            "element_id": "node-alpha",
            "claim_type": "asserted",
            "subject": "SECRET_CLAIM_SUBJECT",
            "provenance": {
                "authority_ceiling": "architecture_contract",
                "source_ref": "docs/claims/node-claim.md#authority",
                "raw_ref": "SECRET_CLAIM_REF",
            },
            "freshness": {"state": "timeless"},
        }]
    }))
    (arch / "system-dynamics-map.observations.jsonl").write_text("\n".join([
        json.dumps({
            "subject_kind": "node",
            "subject": "node-alpha",
            "state": "observed",
            "source_type": "fixture",
            "freshness": "recent",
            "source_ref": "observations/node-alpha.jsonl#latest",
            "evidence": [{"label": "node-alpha-observed"}],
            "body": "SECRET_OBSERVATION_BODY",
        }),
        json.dumps({
            "subject_kind": "edge",
            "subject": "edge-alpha-beta",
            "state": "observed",
            "source_type": "fixture",
            "freshness": "recent",
            "source_ref": "observations/edge-alpha-beta.jsonl#latest",
            "evidence": [{"label": "edge-alpha-beta-observed"}],
            "body": "SECRET_OBSERVATION_BODY",
        }),
    ]) + "\n")
    (arch / "system-dynamics-map.relations.json").write_text(json.dumps({
        "relations": [{
            "category": "governance",
            "allowed_claim_types": ["asserted"],
            "edge_ids": ["edge-alpha-beta"],
            "semantics": "SECRET_RELATION_BODY",
        }]
    }))
    (arch / "system-dynamics-map.canonical.trig").write_text("SECRET_TRIG_BODY")
    (arch / "system-dynamics-map.shacl.ttl").write_text("SECRET_SHACL_BODY")


def test_read_epistemics_endpoint_shape_for_dynamics(tmp_path):
    _write_epistemics_dynamics_fixture(tmp_path)
    app = build_app(str(tmp_path), EXPECTED_DEFAULT_ALLOW, {})
    response = TestClient(app).get("/read/epistemics?scope=dynamics")

    assert response.status_code == 200
    body = response.json()
    assert body["dark"] is False
    assert body["error"] == ""
    epistemics = body["epistemics"]
    assert epistemics["schema_version"] == "epistemics.read.v1"
    assert epistemics["scope"] == "dynamics"
    assert epistemics["authority_case"] == "CASE-DYN"
    assert epistemics["generated_at"] == "2026-06-25T12:00:00Z"
    assert epistemics["package_hash"] == "sha256:test-package"
    assert epistemics["totals"]["rows"] == 7
    assert epistemics["totals"]["map_nodes"] == 1
    assert epistemics["totals"]["map_edges"] == 1
    assert epistemics["totals"]["package_rows"] == 5
    assert epistemics["sources"][0]["id"] == "seed"
    assert epistemics["rows"][0]["raw_access"] is False


def test_read_epistemics_projects_node_and_edge_source_ref_rows(tmp_path):
    _write_epistemics_dynamics_fixture(tmp_path)
    summary = read_epistemics(str(tmp_path), EXPECTED_DEFAULT_ALLOW)
    by_id = {row["row_id"]: row for row in summary["rows"]}

    node = by_id["map-node:node-alpha"]
    assert node["family"] == "dynamics"
    assert node["subject_kind"] == "map-node"
    assert node["subject_ref"] == "node-alpha"
    assert node["subject"] == "node-alpha"
    assert node["status"] == "asserted"
    assert node["posture"] == "source-backed"
    assert node["evidence_count"] == 4
    assert node["source_refs"] == "seed:4 refs"
    assert node["source_ref_labels"] == [
        "node.md#node-alpha",
        "node-claim.md#authority",
        "node-alpha.jsonl#latest",
        "node-alpha-observed",
    ]
    assert node["map_kind"] == "node"
    assert node["map_id"] == "node-alpha"
    assert node["privacy"] == "metadata-only"
    assert node["raw_access"] is False

    edge = by_id["map-edge:edge-alpha-beta"]
    assert edge["subject_kind"] == "map-edge"
    assert edge["map_kind"] == "edge"
    assert edge["map_id"] == "edge-alpha-beta"
    assert edge["map_source"] == "node-alpha"
    assert edge["map_target"] == "node-beta"
    assert edge["map_relation"] == "depends_on"
    assert edge["source_ref_labels"] == [
        "edge.md#alpha-beta",
        "edge-alpha-beta.jsonl#latest",
        "edge-alpha-beta-observed",
    ]
    assert edge["evidence"] == "source_refs:3"


def test_read_epistemics_projects_package_rows_as_metadata(tmp_path):
    _write_epistemics_dynamics_fixture(tmp_path)
    summary = read_epistemics(str(tmp_path), EXPECTED_DEFAULT_ALLOW)
    by_id = {row["row_id"]: row for row in summary["rows"]}

    validation = by_id["validation:package_gate"]
    assert validation["subject_kind"] == "package-row"
    assert validation["posture"] == "declared"
    assert validation["evidence"] == "count:1"
    assert validation["source_refs"] == "package:1 records"
    assert validation["map_kind"] == "package-row"
    assert validation["privacy"] == "metadata-only"
    assert validation["raw_access"] is False

    assert by_id["lens:topology"]["posture"] == "source-backed"
    assert by_id["lens:topology"]["detail"] == "mode=topology; layout=cose; edges=1; reversible=True"
    assert by_id["claim:node:asserted:architecture_contract:freshness-timeless"]["source_refs"] == "claims:1 records"
    assert by_id["observation:observed:fixture:recent:state-observed-source_type-fixture"]["source_refs"] == "observations:2 records"
    assert by_id["relation:governance:asserted"]["source_refs"] == "relations:1 records"


def test_read_epistemics_does_not_leak_raw_map_bodies(tmp_path):
    _write_epistemics_dynamics_fixture(tmp_path)
    dumped = json.dumps(read_epistemics(str(tmp_path), EXPECTED_DEFAULT_ALLOW))
    for secret in [
        "SECRET MAP BODY",
        "SECRET LAYER LABEL",
        "SECRET NODE LABEL",
        "SECRET NODE SUMMARY",
        "SECRET NODE DOC LABEL",
        "https://secret.example/node-alpha",
        "SECRET EDGE SUMMARY",
        "SECRET EDGE DOC LABEL",
        "https://secret.example/edge-alpha-beta",
        "SECRET_COMMAND",
        "SECRET LENS LABEL",
        "SECRET_CLAIM_SUBJECT",
        "SECRET_CLAIM_REF",
        "SECRET_OBSERVATION_BODY",
        "SECRET_RELATION_BODY",
        "SECRET_TRIG_BODY",
        "SECRET_SHACL_BODY",
    ]:
        assert secret not in dumped


def test_read_epistemics_air_default_denies_unallowlisted_fields(tmp_path):
    _write_epistemics_dynamics_fixture(tmp_path)
    row = read_epistemics(str(tmp_path), ["row_id", "source_refs"])["rows"][0]
    assert row["air"]["row_id"] == "ok"
    assert row["air"]["source_refs"] == "ok"
    assert row["air"]["subject"] == "deny"
    assert row["air"]["map_id"] == "deny"
    assert row["air"]["detail"] == "deny"


def test_to_session_shape_state_precedence_and_air():
    lane = {
        "role": "cx-p0", "session": "hapax-codex-cx-p0", "platform": "codex",
        "alive": True, "idle": True, "stalled": True, "claimed_task": "private-task",
        "output_age_s": "12.34", "relay_age_s": None,
    }
    out = to_session("cx-p0", lane, allowlist=["role", "platform", "state", "output_age_s"])
    assert out["role"] == "cx-p0" and out["state"] == "stalled"
    assert out["output_age_s"] == 12.3 and out["relay_age_s"] == 0.0
    assert out["readiness"] == "stall" and out["blocker"] == "stalled" and out["attention"] >= 0.95
    assert out["air"]["role"] == "ok"
    assert out["air"]["session"] == "deny" and out["air"]["claimed_task"] == "deny"
    assert out["air"]["readiness"] == "deny" and out["air"]["attention"] == "deny"


def test_session_attention_sort_prioritizes_cutover_lanes():
    sessions = [
        to_session("alpha", {"role": "alpha", "platform": "codex", "alive": False}, EXPECTED_DEFAULT_ALLOW),
        to_session("beta", {
            "role": "beta", "session": "tmux-beta", "platform": "codex",
            "alive": True, "claimed_task": "task-beta", "output_age_s": 30, "relay_age_s": 40,
        }, EXPECTED_DEFAULT_ALLOW),
    ]
    ranked = sorted(sessions, key=_session_sort_key)
    assert [s["role"] for s in ranked] == ["beta", "alpha"]
    assert ranked[0]["readiness"] == "claim" and ranked[0]["blocker"] == "none"


def test_read_sessions_uses_configured_state_path(monkeypatch, tmp_path):
    state = tmp_path / "state.json"
    state.write_text(json.dumps({
        "lanes": {
            "beta": {"role": "beta", "platform": "claude", "alive": False, "idle": True},
            "alpha": {"role": "alpha", "session": "tmux-alpha", "platform": "codex", "alive": True, "idle": False},
        }
    }))
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(state))
    raw = _raw_sessions()
    sessions = [to_session(name, lane, ["role", "platform", "state"]) for name, lane in raw]
    assert [s["role"] for s in sessions] == ["alpha", "beta"]
    assert sessions[0]["state"] == "active"
    assert sessions[0]["air"]["session"] == "deny"


def test_session_route_binding_prefers_launch_receipt_and_marks_policy_only(tmp_path):
    route_decisions = tmp_path / "route-decisions.jsonl"
    route_decisions.write_text(
        "\n".join([
            json.dumps({
                "created_at": "2026-06-25T10:00:00Z",
                "decision_id": "rd-alpha",
                "lane": "alpha",
                "task_id": "task-a",
                "platform": "codex",
                "mode": "headless",
                "profile": "full",
                "route_id": "codex.headless.full",
                "action": "launch",
                "launch_allowed": True,
            }),
            json.dumps({
                "created_at": "2026-06-25T10:01:00Z",
                "decision_id": "rd-beta",
                "lane": "beta",
                "task_id": "task-b",
                "platform": "claude",
                "mode": "headless",
                "profile": "full",
                "route_id": "claude.headless.full",
                "action": "launch",
                "launch_allowed": True,
            }),
            json.dumps({
                "created_at": "2026-06-25T10:02:00Z",
                "decision_id": "rd-gamma",
                "lane": "gamma",
                "task_id": "task-c",
                "platform": "claude",
                "mode": "headless",
                "profile": "full",
                "route_id": "claude.headless.full",
                "action": "launch",
                "launch_allowed": True,
            }),
            json.dumps({
                "created_at": "2026-06-25T10:04:00Z",
                "decision_id": "rd-alpha-later-policy",
                "lane": "alpha",
                "task_id": "task-a",
                "platform": "codex",
                "mode": "headless",
                "profile": "full",
                "route_id": "codex.headless.full",
                "action": "launch",
                "launch_allowed": True,
            }),
        ]) + "\n"
    )
    methodology = tmp_path / "methodology-dispatch.jsonl"
    methodology.write_text(json.dumps({
        "timestamp": "2026-06-25T10:03:00Z",
        "route_decision_id": "rd-alpha",
        "lane": "alpha",
        "task_id": "task-a",
        "platform": "codex",
        "mode": "headless",
        "profile": "full",
        "dimensional_selected_route_id": "codex.headless.full",
        "route_policy_action": "launch",
        "ok": True,
        "launched": True,
    }) + "\n")

    bindings, source_state, source_path = _route_binding_index({"orchestration_ledger_dir": str(tmp_path)})
    assert source_state == "observed"
    assert source_path == tmp_path

    alpha = {"role": "alpha", "platform": "codex", "claimed_task": "task-a"}
    beta = {"role": "beta", "platform": "claude", "claimed_task": "task-b"}
    gamma = {"role": "gamma", "platform": "codex", "claimed_task": "task-c"}
    idle = {"role": "idle", "platform": "codex"}

    alpha_binding = _session_route_binding("alpha", alpha, bindings, source_state, source_path)
    assert alpha_binding["route_binding_state"] == "bound"
    assert alpha_binding["route_evidence_ref"] == "methodology-dispatch.jsonl:rd-alpha"
    assert _session_route_binding("beta", beta, bindings, source_state, source_path)["route_binding_state"] == "policy_only"
    assert _session_route_binding("gamma", gamma, bindings, source_state, source_path)["route_binding_state"] == "platform_mismatch"
    assert _session_route_binding("idle", idle, bindings, source_state, source_path)["route_binding_state"] == "no_claim"

    session = to_session(
        "alpha",
        {**alpha, "alive": True},
        ["role", "platform", "state", "route_id", "mode", "profile", "route_binding_state"],
        _session_route_binding("alpha", alpha, bindings, source_state, source_path),
    )
    assert session["route_id"] == "codex.headless.full"
    assert session["route_binding_state"] == "bound"
    assert session["air"]["route_binding_state"] == "ok"
    assert session["air"]["route_evidence_ref"] == "deny"


def test_session_route_binding_reports_missing_source(tmp_path):
    bindings, source_state, source_path = _route_binding_index({"orchestration_ledger_dir": str(tmp_path)})
    lane = {"role": "alpha", "platform": "codex", "claimed_task": "task-a"}
    route = _session_route_binding("alpha", lane, bindings, source_state, source_path)
    assert route["route_binding_state"] == "source_missing"
    assert route["route_id"] == ""


def test_session_route_binding_allows_guarded_long_task_prefix(tmp_path):
    route_decisions = tmp_path / "route-decisions.jsonl"
    route_decisions.write_text(json.dumps({
        "created_at": "2026-06-25T10:00:00Z",
        "decision_id": "rd-prefix",
        "lane": "cx-fugu-1",
        "task_id": "p0-incident-sdlc-dispatch-refusal-p0-incident-sdlc-dispatch-refu-77cbfee6-sdlc-dispatch-refusal-circuit-breaker",
        "platform": "codex",
        "mode": "headless",
        "profile": "full",
        "route_id": "codex.headless.full",
        "action": "launch",
        "launch_allowed": True,
    }) + "\n")
    bindings, source_state, source_path = _route_binding_index({"orchestration_ledger_dir": str(tmp_path)})
    lane = {
        "role": "cx-fugu-1",
        "platform": "codex",
        "claimed_task": "p0-incident-sdlc-dispatch-refusal-p0-incident-sdlc-dispatch-refu-77cbfee6",
    }
    route = _session_route_binding("cx-fugu-1", lane, bindings, source_state, source_path)
    assert route["route_binding_state"] == "policy_only"
    assert route["route_id"] == "codex.headless.full"
    assert route["route_evidence_ref"] == "route-decisions.jsonl:rd-prefix"


def test_to_session_detail_refs_metadata_without_transcript_content(tmp_path):
    tasks = tmp_path / "tasks"
    tasks.mkdir()
    note = tasks / "secret-task.md"
    note.write_text("""---
task_id: secret-task
status: claimed
assigned_to: cx-p0
authority_case: CASE-PRIVATE
parent_spec: SPEC-PRIVATE
mutation_surface: source
---
body is not parsed
""")
    transcripts = tmp_path / "transcripts"
    transcripts.mkdir()
    transcript = transcripts / "cx-p0-session.jsonl"
    transcript.write_text('{"message":"SECRET_TRANSCRIPT_SENTINEL"}\n')
    lane = {
        "role": "cx-p0", "session": "tmux-private", "platform": "codex",
        "alive": True, "idle": False, "claimed_task": "secret-task",
        "output_age_s": 1, "relay_age_s": 2,
    }
    detail = to_session_detail("cx-p0", lane, allowlist=["role", "platform", "state", "evidence_count"], cfg={
        "cc_tasks_active": str(tasks),
        "session_transcript_roots": [str(transcripts)],
    })
    assert detail["task"]["status"] == "claimed"
    assert detail["readiness"] == "claim" and detail["blocker"] == "none"
    assert detail["resume"]["ready"] is False
    assert {r["kind"] for r in detail["evidence_refs"]} == {"cc_task_note", "transcript_candidate"}
    assert detail["evidence_summary"]["total"] == 2
    assert detail["evidence_summary"]["by_kind"] == {"cc_task_note": 1, "transcript_candidate": 1}
    assert detail["evidence_summary"]["transcript_roots_observed"] == 1
    assert detail["evidence_summary"]["transcript_roots_missing"] == 0
    assert detail["evidence_summary"]["raw_access"] is False
    assert "metadata-only" in detail["evidence_summary"]["privacy"]
    dumped = json.dumps(detail)
    assert "SECRET_TRANSCRIPT_SENTINEL" not in dumped
    assert "body is not parsed" not in dumped
    assert str(transcript) in dumped  # metadata ref only
    assert detail["air"]["path"] == "deny"
    assert detail["air"]["authority_case"] == "deny"


def test_read_intake_summary_uses_bounded_metadata_only(tmp_path):
    request_state = tmp_path / "request-state.json"
    request_state.write_text(json.dumps({
        "combined_attention_count": 7,
        "malformed_count": 1,
        "unread_count": 0,
        "stale_count": 0,
    }))
    planning_feed = tmp_path / "planning-feed.json"
    planning_feed.write_text(json.dumps({
        "total_requests": 9,
        "coverage_summary": {"untracked": 3, "task_offered": 2},
        "stale_summary": {"stale_offered": 2},
        "attention_required": [{"request_id": "SECRET-REQUEST-ID"}],
    }))
    p0_state = tmp_path / "p0-state.json"
    p0_state.write_text(json.dumps({
        "incidents": {
            "fp": {
                "kind": "systemd_service_failed",
                "last_message": "SECRET_NOTIFICATION_BODY",
                "task_path": "/private/task/path",
            }
        }
    }))
    p0_events = tmp_path / "p0-events.jsonl"
    p0_events.write_text(json.dumps({
        "kind": "p0_incident_notification",
        "message": "SECRET_LEDGER_MESSAGE",
        "task_path": "/private/ledger/path",
    }) + "\n")
    security_state = tmp_path / "security.json"
    security_state.write_text(json.dumps({
        "total_signals": 2,
        "requests": [{
            "kind": "github-dependabot-alert",
            "status": "skipped_existing",
            "url": "https://secret.example/alert",
        }],
    }))
    summary = read_intake_summary({
        "request_intake_state": str(request_state),
        "planning_feed_state": str(planning_feed),
        "p0_incident_state": str(p0_state),
        "p0_incident_events": str(p0_events),
        "security_signal_state": str(security_state),
    }, EXPECTED_DEFAULT_ALLOW)

    assert summary["totals"]["request_attention"] == 7
    assert summary["totals"]["planning_attention"] == 1
    assert summary["totals"]["p0_incidents"] == 1
    assert summary["totals"]["security_signals"] == 2
    assert any(r["kind"] == "coverage:untracked" for r in summary["rows"])
    assert any(r["kind"] == "incident:systemd_service_failed" for r in summary["rows"])
    assert any(r["kind"] == "security:github-dependabot-alert" for r in summary["rows"])
    coverage = next(r for r in summary["rows"] if r["kind"] == "coverage:untracked")
    assert coverage["id"] == "planning_feed:coverage:untracked"
    assert coverage["authority"] == "planning-observation"
    assert coverage["evidence"] == "count:3"
    assert coverage["source_refs"] == "planning_feed"
    assert coverage["action"] == "triage"
    assert coverage["next_evidence"]
    incident = next(r for r in summary["rows"] if r["kind"] == "incident:systemd_service_failed")
    assert incident["missing"] == "triage receipt · blocker resolution"
    assert incident["action"] == "triage-critical"
    assert incident["next_evidence"] == "attach blocker disposition and governed receipt"
    for field in ["id", "authority", "evidence", "missing", "action", "detail", "source_refs", "next_evidence"]:
        assert coverage["air"][field] == "ok"
    dumped = json.dumps(summary)
    for secret in ["SECRET-REQUEST-ID", "SECRET_NOTIFICATION_BODY", "SECRET_LEDGER_MESSAGE", "https://secret.example/alert"]:
        assert secret not in dumped
    assert summary["sources"][0]["air"]["path"] == "deny"
    assert summary["sources"][0]["air"]["count"] == "ok"


def test_read_capability_summary_uses_registry_metadata_only(tmp_path, monkeypatch):
    council = tmp_path / "council"
    shared = council / "shared"
    config = council / "config"
    shared.mkdir(parents=True)
    config.mkdir()
    (shared / "__init__.py").write_text("")
    registry_path = config / "platform-capability-registry.json"
    registry_path.write_text("{}")
    receipts = tmp_path / "receipts"
    route_auth = receipts / "route-authority"
    route_auth.mkdir(parents=True)
    (receipts / "codex.json").write_text("{}")
    (route_auth / "runtime_actuation-codex.json").write_text("{}")
    quota = tmp_path / "quota.json"
    quota.write_text("{}")

    (shared / "platform_capability_receipts.py").write_text(f"""
from pathlib import Path
DEFAULT_PLATFORM_CAPABILITY_RECEIPT_DIR = Path({str(receipts)!r})
""")
    (shared / "quota_spend_ledger.py").write_text(f"""
from pathlib import Path
DEFAULT_QUOTA_SPEND_LEDGER_LIVE = Path({str(quota)!r})
""")
    (shared / "platform_capability_registry.py").write_text(f"""
from pathlib import Path
PLATFORM_CAPABILITY_REGISTRY = Path({str(registry_path)!r})
class ScoreBag:
    def model_dump(self):
        return {{
            "grounding": {{"observed_at": "2026-06-25T00:00:00Z", "evidence_refs": ["registry:grounding"]}},
            "source_editing": {{"observed_at": None, "evidence_refs": []}},
        }}
class Enumish:
    def __init__(self, value): self.value = value
class Telemetry:
    quota_source = Enumish("live")
class Descriptor:
    model_id = "gpt-test"
    effort = "high"
    context_mode = "large"
    fast_mode = "disabled"
    quantization = "none"
    capacity_pool = "priority"
class ToolState:
    tool_id = "filesystem"
    available = True
    authority_use = [Enumish("read"), Enumish("write")]
    observed_at = "2026-06-25T00:00:00Z"
    stale_after = "24h"
    evidence_ref = "platform-capability-registry:codex.headless.full:tool_state.filesystem"
class Route:
    route_id = "codex.headless.full"
    platform = "codex"
    mode = "headless"
    profile = "full"
    demand_vector = "implementation"
    hardening = "standard"
    eval_plane = "deterministic"
    review_obligation = "review-team"
    learning_eligibility = "independent-gate"
    benchmark_coverage = "registry"
    fixed_overhead = "session-bootstrap"
    route_state = "active"
    blocked_reasons = []
    authority_ceiling = Enumish("authoritative")
    telemetry = Telemetry()
    execution_descriptor = Descriptor()
    model_or_engine = "fallback"
    capability_scores = ScoreBag()
    tool_state = [ToolState()]
class Registry:
    routes = [Route()]
def check_registry_freshness(registry):
    class Check:
        route_id = "codex.headless.full"
        ok = True
        errors = ()
        evidence_refs = ("receipt:codex",)
    class Freshness:
        routes = (Check(),)
    return Freshness()
""")
    (shared / "dispatcher_policy.py").write_text("""
from pathlib import Path
ROUTE_AUTHORITY_RECEIPT_DIRNAME = "route-authority"
class Sources:
    registry_error = None
    quota_error = None
    quota_live_error = None
    quota_ledger_source = "live"
    quota_ledger = object()
    route_authority_receipts = (object(),)
def load_dispatch_policy_sources(receipt_dir=None):
    from shared.platform_capability_registry import Registry
    s = Sources()
    s.registry = Registry()
    return s
""")
    monkeypatch.syspath_prepend(str(council))
    for name in list(__import__("sys").modules):
        if name == "shared" or name.startswith("shared."):
            __import__("sys").modules.pop(name, None)

    hkp_shadow = tmp_path / "hkp-shadow"
    hkp_index = tmp_path / "hkp-shadow-index"
    hkp_reports = tmp_path / "hkp-reports"
    hkp_dir = hkp_shadow / "sdlc" / "_hkp"
    concepts = hkp_shadow / "sdlc" / "concepts"
    hkp_dir.mkdir(parents=True)
    concepts.mkdir()
    hkp_index.mkdir()
    report_dir = hkp_reports / "r1"
    report_dir.mkdir(parents=True)
    (hkp_dir / "manifest.yaml").write_text("""bundle_uid: hkp:bundle:sdlc
hkp_schema: 1
cache_only: true
allowed_consumers:
- research_viewer
- local_prompt_context
forbidden_consumers:
- close_gate
- dispatcher
- provider_spend_gate
- public_export
- release_gate
- runtime_loader
generated_at: '2026-06-25T18:35:16Z'
""")
    (hkp_dir / "consumer_policy.yaml").write_text("""hkp_schema: 1
consumers:
- consumer: dispatcher
  default: deny
- consumer: release_gate
  default: deny
- consumer: provider_spend_gate
  default: deny
- consumer: public_export
  default: deny
- consumer: runtime_loader
  default: deny
- consumer: close_gate
  default: deny
- consumer: local_prompt_context
  default: allow_with_ceiling
- consumer: research_viewer
  default: allow_read_only
""")
    (hkp_dir / "snapshot.json").write_text(json.dumps({
        "bundle_uid": "hkp:bundle:sdlc",
        "concept_count": 2,
        "edge_count": 3,
        "generated_at": "2026-06-25T18:35:16Z",
    }))
    (hkp_dir / "events.jsonl").write_text("{}\n{}\n")
    (hkp_dir / "edges.jsonl").write_text("{}\n{}\n{}\n")
    (hkp_index / "sdlc.jsonl").write_text("\n".join("{}" for _ in range(7)) + "\n")
    (report_dir / "report.json").write_text("{}")
    (concepts / "secret.md").write_text("SECRET_HKP_CONCEPT_BODY")

    summary = read_capability_summary(str(council), EXPECTED_DEFAULT_ALLOW, {
        "capability_receipt_dir": str(receipts),
        "quota_spend_ledger_live": str(quota),
        "hkp_shadow_root": str(hkp_shadow),
        "hkp_index_root": str(hkp_index),
        "hkp_report_root": str(hkp_reports),
        "hkp_bundles": ["sdlc"],
    })

    assert summary["totals"]["routes"] == 1
    assert summary["totals"]["capabilities"] >= 5
    assert any(row["capability_id"] == "grounding" and row["status"] == "observed" for row in summary["rows"])
    assert any(row["capability_id"] == "source_editing" and row["status"] == "read-missing" for row in summary["rows"])
    hkp = next(row for row in summary["rows"] if row["capability_id"] == "hkp_support_context")
    assert hkp["hkp_posture"] == "support_only"
    assert hkp["status"] == "support-only"
    assert hkp["evidence_count"] == 7
    assert hkp["source_refs"] == "hkp:sdlc:7 refs"
    assert "not source truth" in hkp["blocker"]
    assert any(row["capability_id"] == "source_acquisition" and row["status"] == "admission-incomplete" for row in summary["rows"])
    assert any(row["capability_id"] == "publication_egress" and "publication authority" in row["blocker"] for row in summary["rows"])
    assert any(row["capability_id"] == "provider_gateway" and row["status"] == "spend-forbidden" for row in summary["rows"])
    assert any(row["capability_id"] == "tavily_source_acquisition" and "usage telemetry" in row["blocker"] for row in summary["rows"])
    assert any(row["capability_id"] == "cohere_embed_rerank" and row["authority"] == "source/verifier support" for row in summary["rows"])
    assert any(row["capability_id"] == "elevenlabs_audio_generation" and "audio/public-egress" in row["authority"] for row in summary["rows"])
    assert any(row["capability_id"] == "openrouter_break_glass" and row["status"] == "spend-forbidden" for row in summary["rows"])
    glm = next(row for row in summary["rows"] if row["capability_id"] == "glm_coding_plan_tool_surface")
    assert glm["status"] == "manual-bakeoff"
    assert "S15 proves invocation" in glm["blocker"]
    fugu = next(row for row in summary["rows"] if row["capability_id"] == "fugu_raw_codex")
    assert fugu["status"] == "raw-manual"
    assert fugu["authority"] == "not dispatchable"
    assert fugu["route_count"] == 0
    assert "no governed Fugu route" in fugu["blocker"]
    fugu_ultra = next(row for row in summary["rows"] if row["capability_id"] == "fugu_ultra_raw_codex")
    assert fugu_ultra["status"] == "raw-manual"
    assert fugu_ultra["route_count"] == 0
    assert "clean-host preflight" in fugu_ultra["receipt_requirement"]
    assert any(src["id"] == "capability_surface_compiled_fallback" and src["status"] == "support-only" for src in summary["sources"])
    hkp_bundle = next(src for src in summary["sources"] if src["id"] == "hkp_bundle:sdlc")
    assert hkp_bundle["path"] == "hkp-shadow:sdlc/_hkp/manifest.yaml"
    assert hkp_bundle["status"] == "support-only"
    assert hkp_bundle["count"] == 7
    assert hkp_bundle["privacy"] == "metadata-only"
    assert str(hkp_shadow) not in hkp_bundle["path"]
    dumped = json.dumps(summary)
    assert "SECRET_HKP_CONCEPT_BODY" not in dumped
    tavily = next(row for row in summary["rows"] if row["capability_id"] == "tavily_source_acquisition")
    assert tavily["capability_class"] == "source_acquisition"
    assert tavily["surface_family"] == "tavily"
    assert tavily["spend_model"] == "api_spend_budgeted"
    assert tavily["egress_class"] == "source_query"
    assert "usage" in tavily["receipt_requirement"]
    assert tavily["source_refs"] == "compiled_fallback"
    source_class = next(row for row in summary["rows"] if row["capability_id"] == "source_acquisition")
    assert source_class["capability_class"] == "source_acquisition"
    assert source_class["surface_family"] == "source_acquisition"
    assert any(row["capability_id"] == "codex_worker_reviewer_surface" and row["surface_family"] == "worker_reviewer" for row in summary["rows"])
    assert any(row["capability_id"] == "glmcp_review_quota_admission" and row["status"] == "observed" for row in summary["rows"])
    assert any(row["capability_id"] == "fugu_raw_codex" and row["surface_family"] == "sakana_fugu" for row in summary["rows"])
    assert any(row["capability_id"] == "codeql_status_floor" and row["surface_family"] == "codeql_status" for row in summary["rows"])
    assert any(row["capability_id"] == "google_workspace_youtube_connector" and row["egress_class"] == "public_or_workspace" for row in summary["rows"])
    assert any(row["capability_id"] == "network_admin_tailscale" and row["capability_class"] == "infrastructure_control" for row in summary["rows"])
    assert summary["routes"][0]["route_id"] == "codex.headless.full"
    assert summary["routes"][0]["effort"] == "high"
    assert summary["routes"][0]["context_mode"] == "large"
    assert summary["routes"][0]["fast_mode"] == "disabled"
    assert summary["routes"][0]["quantization"] == "none"
    assert summary["routes"][0]["capacity_pool"] == "priority"
    assert summary["routes"][0]["demand_vector"] == "implementation"
    assert summary["routes"][0]["hardening"] == "standard"
    assert summary["routes"][0]["eval_plane"] == "deterministic"
    assert summary["routes"][0]["review_obligation"] == "review-team"
    assert summary["routes"][0]["learning_eligibility"] == "independent-gate"
    assert summary["routes"][0]["benchmark_coverage"] == "registry"
    assert summary["routes"][0]["fixed_overhead"] == "session-bootstrap"
    assert summary["routes"][0]["air"]["demand_vector"] == "ok"
    assert summary["totals"]["tools"] == 1
    assert summary["tools"][0]["tool_id"] == "filesystem"
    assert summary["tools"][0]["status"] == "observed"
    assert summary["tools"][0]["available"] is True
    assert summary["tools"][0]["authority_use"] == "read,write"
    assert summary["tools"][0]["raw_access"] is False
    assert summary["tools"][0]["air"]["evidence_ref"] == "deny"
    assert summary["sources"][0]["air"]["path"] == "deny"


def test_capability_surface_pack_rows_are_source_backed(tmp_path):
    pack = tmp_path / "capability-pack.json"
    pack.write_text(json.dumps({
        "pack_id": "cap-pack",
        "authority_case": "support_non_authoritative",
        "capability_classes": [{
            "capability_id": "source_acquisition",
            "status": "admission-incomplete",
            "authority": "sub-router",
            "route_count": 2,
            "ok_count": 1,
            "blocked_count": 1,
            "evidence_count": 2,
            "blocker": "usage receipt missing",
            "source_refs": ["handoff#source"],
        }],
        "surfaces": [{
            "capability_id": "tavily_source_acquisition",
            "status": "admission-incomplete",
            "routing_meaning": "source-acquisition",
            "route_count": 1,
            "ok_count": 0,
            "blocked_count": 1,
            "evidence_count": 4,
            "blocker": "usage schema validation failure on /usage limit null",
            "source_refs": ["handoff#tavily", "docs#usage"],
        }],
    }))
    sources, rows = _capability_surface_pack_rows({"capability_surface_pack_paths": [str(pack)]}, EXPECTED_DEFAULT_ALLOW)
    assert sources[0]["id"] == "cap-pack"
    assert sources[0]["status"] == "observed"
    source_class = next(row for row in rows if row["capability_id"] == "source_acquisition")
    assert source_class["capability_class"] == "source_acquisition"
    assert source_class["surface_family"] == "source_acquisition"
    tavily = next(row for row in rows if row["capability_id"] == "tavily_source_acquisition")
    assert tavily["capability_class"] == "source_acquisition"
    assert tavily["authority"] == "source-acquisition"
    assert tavily["source_refs"] == "cap-pack:2 refs"
    assert tavily["source_ref_labels"] == ["handoff#tavily", "docs#usage"]
    assert tavily["air"]["source_refs"] == "ok"
    assert tavily["air"]["source_ref_labels"] == "ok"
    dumped = json.dumps(rows)
    assert "read-missing doctrine" not in dumped
    assert "/tmp/" not in dumped


def test_checked_in_capability_surface_pack_carries_explicit_ontology():
    pack = Path(__file__).resolve().parents[1] / "docs" / "capability-surface-packs" / "hapax-capability-surface-pack-20260625.json"
    doc = json.loads(pack.read_text())
    rows = [*doc["capability_classes"], *doc["surfaces"]]
    required = ["capability_class", "surface_family", "spend_model", "egress_class", "receipt_requirement"]
    missing = [
        f"{row.get('capability_id')}:{field}"
        for row in rows
        for field in required
        if not str(row.get(field) or "").strip()
    ]
    assert missing == []

    by_id = {row["capability_id"]: row for row in rows}
    assert by_id["source_acquisition"]["capability_class"] == "source_acquisition"
    assert by_id["tavily_source_acquisition"]["surface_family"] == "tavily"
    assert by_id["github_repo_ci"]["capability_class"] == "verifier_floor_checker"
    assert by_id["google_workspace_youtube_connector"]["egress_class"] == "public_or_workspace"
    assert by_id["network_admin_tailscale"]["capability_class"] == "infrastructure_control"
    assert by_id["glm_coding_plan_tool_surface"]["status"] == "manual-bakeoff"
    assert "S15 proves invocation" in by_id["glm_coding_plan_tool_surface"]["blocker"]
    assert "example-ref.yaml#note" in by_id["glm_coding_plan_tool_surface"]["source_refs"]
    assert by_id["fugu_raw_codex"]["status"] == "raw-manual"
    assert by_id["fugu_raw_codex"]["authority"] == "not dispatchable"
    assert by_id["fugu_raw_codex"]["route_count"] == 0
    assert by_id["fugu_ultra_raw_codex"]["status"] == "raw-manual"
    assert by_id["fugu_ultra_raw_codex"]["route_count"] == 0


def test_read_gate_summary_preserves_no_go_names_and_lane_blockers(monkeypatch, tmp_path):
    monkeypatch.setattr("reins_read._projection", lambda _root: {"tasks": {
        "task-a": {
            "task_id": "task-a",
            "stage": "S7_RELEASE",
            "authority_case": "CASE-A",
            "no_go": {
                "release_authorized": False,
                "source_mutation_authorized": True,
                "public_current": False,
            },
        },
        "task-b": {
            "task_id": "task-b",
            "stage": "S4_BUILD",
            "authority_case": "",
            "no_go": {
                "implementation_authorized": False,
                "release_authorized": True,
            },
        },
    }})
    monkeypatch.setattr("reins_read._raw_tail", lambda _root, _limit: [{"kind": "coord_dispatch.launch_failed"}])
    state = tmp_path / "state.json"
    state.write_text(json.dumps({"lanes": {
        "cx-stale": {
            "role": "cx-stale",
            "session": "tmux-stale",
            "platform": "codex",
            "alive": True,
            "relay_age_s": 30000,
            "claimed_task": "task-a",
        },
        "cx-off": {
            "role": "cx-off",
            "platform": "claude",
            "alive": False,
        },
    }}))
    (tmp_path / "route-decisions.jsonl").write_text(json.dumps({
        "decision_id": "rd-gate",
        "created_at": "2026-06-25T00:00:00Z",
        "lane": "cx-stale",
        "task_id": "task-a",
        "platform": "codex",
        "mode": "headless",
        "profile": "full",
        "route_id": "codex.headless.full",
        "action": "launch",
    }) + "\n")
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(state))

    summary = read_gate_summary("/fake/council", EXPECTED_DEFAULT_ALLOW, {"orchestration_ledger_dir": str(tmp_path)})

    assert summary["totals"]["tasks"] == 2
    assert summary["totals"]["lanes"] == 2
    assert summary["totals"]["blocked"] == 6
    assert summary["totals"]["preview"] == 6
    ids = {row["gate_id"] for row in summary["rows"]}
    assert "task.no_go.release_authorized" in ids
    assert "task.no_go.public_current" in ids
    assert "task.no_go.implementation_authorized" in ids
    assert "task.authority_case" in ids
    assert "lane.blocker.stale_relay" in ids
    assert "lane.blocker.offline" in ids
    assert "route.binding.policy_only" in ids
    release = next(row for row in summary["rows"] if row["gate_id"] == "task.no_go.release_authorized")
    assert release["missing"] == "release_authorized"
    assert release["state"] == "blocked"
    assert release["air"]["gate_id"] == "ok"
    route = next(row for row in summary["rows"] if row["gate_id"] == "route.binding.policy_only")
    assert route["domain"] == "route"
    assert route["missing"] == "launch receipt/session confirmation"
    assert "codex.headless.full" in route["evidence"]
    route_source = next(source for source in summary["sources"] if source["id"] == "route_binding")
    assert route_source["count"] == 2
    assert "ledger_records=1" in route_source["detail"]
    dumped = json.dumps(summary)
    assert "SECRET" not in dumped


def test_read_domain_pack_summary_is_source_backed_without_raw_bodies(tmp_path):
    pack = tmp_path / "domains.json"
    registry = tmp_path / "lifecycles.json"
    registry.write_text(json.dumps({
        "registry_id": "hapax-lifecycle-registry",
        "authority_case": "CASE-LC",
        "generated_at": "2026-06-25T09:00:00Z",
        "default_lens": "tenant-lifecycle",
        "lifecycles": [{
            "lifecycle_id": "ldlc",
            "label": "SECRET LIFE LABEL",
            "owner": "operator-instance",
            "scope": "tenant",
            "plant": "life-management",
            "posture": "future-tenant",
            "state": "dark_specified",
            "maturity": "declared-not-modeled",
            "adapter_id": "adapter.ldlc.pending",
            "authority_ceiling": "support_non_authoritative",
            "claim_surface": "practical loops",
            "mutation_surface": "none until receipts exist",
            "dark_policy": "show declared future lifecycle only",
            "freshness_policy": "requires source inventory",
            "air_class": "private-life",
            "windows": ["domains", "intake"],
            "surfaces": ["hapax-exclusive-chat"],
            "commands": ["note", "show-route"],
            "receipt_contracts": ["consent", "privacy"],
            "blocker": "SECRET RAW BLOCKER",
            "next_evidence": "LDLC parent spec",
            "source_refs": ["SECRET-LIFE-REF"],
            "notes": "SECRET RAW LIFE BODY",
        }],
    }))
    pack.write_text(json.dumps({
        "pack_id": "rdlc-pack",
        "authority_case": "CASE-RDLC",
        "generated_at": "2026-06-25T10:00:00Z",
        "default_lens": "lifecycle",
        "domains": [{
            "domain_id": "rdlc-labrack",
            "label": "SECRET LABRACK LABEL",
            "lifecycle": "RDLC",
            "terrain": "bedrock",
            "depth": "stratum",
            "scope": "tenant",
            "state": "candidate",
            "authority_ceiling": "support_non_authoritative",
            "claim_ceiling": "navigation",
            "windows": ["domains", "dynamics"],
            "surfaces": ["wide-context"],
            "parity": "labrack",
            "source_refs": ["SECRET-RAW-REF"],
            "notes": "SECRET RAW BODY",
        }],
        "relations": [{
            "source": "rdlc-labrack",
            "target": "research-rdlc",
            "relation": "extends",
            "source_refs": ["SECRET-REL-REF"],
        }],
    }))
    summary = read_domain_pack_summary({"domain_pack_paths": [str(pack)], "lifecycle_registry_paths": [str(registry)]}, EXPECTED_DEFAULT_ALLOW)
    assert summary["authority"] == "CASE-RDLC"
    assert summary["generated_at"] == "2026-06-25T10:00:00Z"
    assert summary["totals"]["rows"] == 1
    assert summary["totals"]["relations"] == 1
    assert summary["lifecycle_authority"] == "CASE-LC"
    assert summary["lifecycle_totals"]["rows"] == 1
    assert summary["lifecycles"][0]["lifecycle_id"] == "ldlc"
    assert summary["lifecycles"][0]["air"]["label"] == "deny"
    assert summary["lifecycles"][0]["source_refs"] == "hapax-lifecycle-registry:1 refs"
    assert summary["rows"][0]["domain_id"] == "rdlc-labrack"
    assert summary["rows"][0]["source_refs"] == "rdlc-pack:1 refs"
    assert summary["rows"][0]["air"]["label"] == "deny"
    assert summary["rows"][0]["air"]["domain_id"] == "ok"
    dumped = json.dumps(summary)
    for secret in ["SECRET RAW BODY", "SECRET-RAW-REF", "SECRET-REL-REF", "SECRET RAW LIFE BODY", "SECRET-LIFE-REF"]:
        assert secret not in dumped


def test_read_lifecycle_registry_summary_missing_config_is_explicit():
    summary = read_lifecycle_registry_summary({}, EXPECTED_DEFAULT_ALLOW)
    assert summary["authority"] == "compiled-fallback"
    assert summary["totals"]["missing_sources"] == 1
    assert summary["sources"][0]["status"] == "missing"
    assert summary["rows"] == []


def test_read_domain_pack_summary_missing_config_is_explicit():
    summary = read_domain_pack_summary({}, EXPECTED_DEFAULT_ALLOW)
    assert summary["authority"] == "compiled-fallback"
    assert summary["totals"]["missing_sources"] == 1
    assert summary["lifecycle_totals"]["missing_sources"] == 1
    assert summary["sources"][0]["status"] == "missing"
    assert summary["rows"] == []



def test_read_vault_endpoint_returns_metadata_only(tmp_path):
    vault = tmp_path / "vault"
    projects = vault / "Projects"
    areas = vault / "Areas"
    hidden = vault / ".obsidian"
    projects.mkdir(parents=True)
    areas.mkdir()
    hidden.mkdir()
    note_a = projects / "First Note.md"
    note_b = areas / "Second Note.md"
    note_a.write_text("SECRET FIRST BODY", encoding="utf-8")
    note_b.write_text("SECRET SECOND BODY", encoding="utf-8")
    (hidden / "Hidden.md").write_text("SECRET HIDDEN BODY", encoding="utf-8")
    os.utime(note_a, (1_700_000_000, 1_700_000_000))
    os.utime(note_b, (1_700_000_100, 1_700_000_100))

    app = build_app("", EXPECTED_DEFAULT_ALLOW, {"vault_root": str(vault)})
    endpoint = next(route.endpoint for route in app.routes if getattr(route, "path", "") == "/read/vault")

    data = endpoint()
    assert data["vault_root"] == str(vault)
    assert data["dark"] is False
    assert [note["title"] for note in data["notes"]] == ["Second Note", "First Note"]
    by_title = {note["title"]: note for note in data["notes"]}
    assert by_title["First Note"]["rel_path"] == "Projects/First Note.md"
    assert by_title["First Note"]["folder"] == "Projects"
    assert by_title["First Note"]["obsidian_uri"] == "obsidian://open?path=" + quote(str(note_a.resolve()), safe="")
    assert by_title["Second Note"]["folder"] == "Areas"
    dumped = json.dumps(data)
    assert "SECRET" not in dumped
    assert "Hidden" not in dumped


def test_read_vault_missing_root_is_dark(tmp_path):
    missing = tmp_path / "missing-vault"
    data = read_vault_summary({"vault_root": str(missing)})

    assert data == {"vault_root": str(missing), "dark": True, "notes": []}


def test_read_observe_summary_is_per_dimension_honest_dark(tmp_path, monkeypatch):
    state = tmp_path / "coordinator-state.json"
    state.write_text(json.dumps({
        "lanes": {
            "cc-live": {
                "role": "cc-live",
                "session": "tmux-live",
                "platform": "codex",
                "alive": True,
                "idle": False,
                "stalled": False,
                "claimed_task": "",
                "output_age_s": 1,
                "relay_age_s": 2,
            }
        }
    }))
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(state))

    def dark_http(_url, _timeout):
        raise OSError("blocked test source")

    monkeypatch.setattr(reins_read, "_observe_http_json", dark_http)

    data = read_observe_summary({"council_root": "", "observe_api_url": "http://127.0.0.1:9"})
    dims = {dim["key"]: dim for dim in data["dimensions"]}

    assert data["dark"] is False
    assert dims["agents"]["status"] == "live"
    assert dims["agents"]["count"] == 1
    assert "lanes=1" in dims["agents"]["summary"]
    assert dims["gpu"]["status"] == "dark"
    assert dims["gpu"]["count"] is None  # no fabricated zero when the source is unreachable
    assert dims["drift"]["status"] == "dark"
    assert dims["drift"]["count"] is None


def test_read_observe_endpoint_returns_summary(tmp_path, monkeypatch):
    state = tmp_path / "coordinator-state.json"
    state.write_text(json.dumps({"lanes": {}}))
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(state))
    monkeypatch.setattr(reins_read, "_observe_http_json", lambda _url, _timeout: {"status": "ok", "count": 2})

    app = build_app("", EXPECTED_DEFAULT_ALLOW, {"observe_api_url": "http://observe.test"})
    endpoint = next(route.endpoint for route in app.routes if getattr(route, "path", "") == "/read/observe")

    data = endpoint()
    dims = {dim["key"]: dim for dim in data["dimensions"]}
    assert data["dark"] is False
    assert dims["health"]["status"] == "live"  # local coordinator source was reachable, even empty
    assert dims["health"]["count"] == 0
    assert dims["gpu"]["status"] == "live"     # HTTP-backed dimension used the mocked observe API
    assert dims["gpu"]["count"] == 2

def test_instance_config_neutral_defaults_no_baked_path(monkeypatch):
    monkeypatch.setenv("REINS_CONFIG", "/no/such/reins.toml")  # force the no-file path
    for k in ("REINS_COUNCIL_ROOT", "REINS_AIR_ALLOWLIST", "REINS_PORT", "REINS_LIFECYCLE_REGISTRIES", "REINS_DOMAIN_PACKS", "REINS_CAPABILITY_SURFACE_PACKS", "REINS_HKP_SHADOW_ROOT", "REINS_HKP_INDEX_ROOT", "REINS_HKP_REPORT_ROOT", "REINS_HKP_BUNDLES", "REINS_VAULT_ROOT"):
        monkeypatch.delenv(k, raising=False)
    cfg = instance_config()
    assert cfg["council_root"] == ""  # NO baked operator path
    assert cfg["port"] == 8799
    assert cfg["lifecycle_registry_paths"] == []
    assert cfg["capability_surface_pack_paths"] == []
    assert cfg["hkp_shadow_root"] == ""
    assert cfg["hkp_index_root"] == ""
    assert cfg["hkp_report_root"] == ""
    assert cfg["hkp_bundles"] == []
    assert cfg["vault_root"] == ""
    assert "kind" in cfg["allowlist"] and "subject" not in cfg["allowlist"]  # conservative on-air default
    # the default AIR allowlist now derives from the facet-cut SSOT (operator-approved 2026-06-26),
    # which airs the structural skeleton + denies free-text/PII (proven safe by test_facet_registry).
    import facet_registry
    assert cfg["allowlist"] == facet_registry.air_allowlist()


def test_instance_config_env_overrides(monkeypatch):
    monkeypatch.setenv("REINS_CONFIG", "/no/such/reins.toml")
    monkeypatch.setenv("REINS_COUNCIL_ROOT", "/env/root")
    monkeypatch.setenv("REINS_AIR_ALLOWLIST", "kind,subject")
    monkeypatch.setenv("REINS_LIFECYCLE_REGISTRIES", "/lc1:/lc2")
    monkeypatch.setenv("REINS_DOMAIN_PACKS", "/d1:/d2")
    monkeypatch.setenv("REINS_CAPABILITY_SURFACE_PACKS", "/c1:/c2")
    monkeypatch.setenv("REINS_HKP_SHADOW_ROOT", "/hkp-shadow")
    monkeypatch.setenv("REINS_HKP_INDEX_ROOT", "/hkp-index")
    monkeypatch.setenv("REINS_HKP_REPORT_ROOT", "/hkp-reports")
    monkeypatch.setenv("REINS_HKP_BUNDLES", "sdlc,rdlc")
    monkeypatch.setenv("REINS_VAULT_ROOT", "/vault")
    cfg = instance_config()
    assert cfg["council_root"] == "/env/root"
    assert cfg["allowlist"] == ["kind", "subject"]
    assert cfg["lifecycle_registry_paths"] == ["/lc1", "/lc2"]
    assert cfg["domain_pack_paths"] == ["/d1", "/d2"]
    assert cfg["capability_surface_pack_paths"] == ["/c1", "/c2"]
    assert cfg["hkp_shadow_root"] == "/hkp-shadow"
    assert cfg["hkp_index_root"] == "/hkp-index"
    assert cfg["hkp_report_root"] == "/hkp-reports"
    assert cfg["hkp_bundles"] == ["sdlc", "rdlc"]
    assert cfg["vault_root"] == "/vault"
