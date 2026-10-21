"""Contract test for the facet registry (the keystone of the representational framework).

Pins EXHAUSTIVENESS (every live read attribute resolves to exactly one facet / edge / body /
envelope — no UNRESOLVED limbo), the 9-facet vocabulary, the per-facet AIR policy, the field-level
homonym resolution (status = provenance on graph/epistemic, posture elsewhere), and the A6 fact that
the registry REPAIRS the flat _DEFAULT_ALLOW gap (skeleton fields that default-deny today now air)."""

import facet_registry as fr

# The LIVE read-domain field universe (from the read API to_* projections + structs, 2026-06-26).
# Attribute NAMES only; this is the ground the registry must exhaustively bin.
INVENTORY: dict[str, list[str]] = {
    "Event": ["ts", "kind", "subject", "actor", "summary", "score", "air"],
    "Trace": ["ts", "trace_id", "model", "prompt_tok", "completion_tok", "total_tok", "cost", "latency_ms", "air"],
    "Turn": ["ts", "role", "kind", "summary", "magnitude", "model", "route", "gate", "air"],
    "TurnBlock": ["kind", "summary", "magnitude", "meta", "air"],
    "Task": ["task_id", "stage", "authority_case", "no_go", "prior_stage", "predicted_stage", "owner",
             "freshness", "criticality", "rel_count", "air"],
    "Session": ["role", "session", "platform", "state", "alive", "idle", "stalled", "claimed_task",
                "output_age_s", "relay_age_s", "readiness", "blocker", "attention", "route_id", "mode",
                "profile", "route_binding_state", "route_evidence_ref", "air"],
    "SessionHealth": ["alive", "idle", "stalled", "output_age_s", "relay_age_s"],
    "SessionTmux": ["session", "exists", "attached", "activity_age_s"],
    "SessionTaskDetail": ["task_id", "status", "assigned_to", "authority_case", "parent_spec",
                          "mutation_surface", "updated_at"],
    "EvidenceRef": ["kind", "path", "mtime", "size", "privacy", "raw_access"],
    "ResumeContext": ["intent", "ready", "authority", "blocked_reasons"],
    "SessionEvidenceSummary": ["total", "by_kind", "transcript_roots_observed", "transcript_roots_missing",
                               "truncated", "privacy", "raw_access"],
    "SessionDetail": ["role", "platform", "state", "session", "readiness", "blocker", "attention", "task_id",
                      "status", "assigned_to", "authority_case", "parent_spec", "mutation_surface", "updated_at",
                      "path", "evidence_count", "resume_ready", "health", "tmux", "task", "evidence_refs",
                      "evidence_summary", "resume", "air"],
    "Node": ["id", "label", "kind", "layer", "status", "res", "summary", "context", "docs", "hardening_notes",
             "aliases", "tags", "source_refs", "source_ref_labels", "air"],
    "Edge": ["id", "source", "target", "relation", "status", "layer", "res", "confidence", "summary", "docs",
             "source_refs", "source_ref_labels", "air"],
    "Layer": ["id", "label"],
    "DynamicsSource": ["id", "status", "count", "detail", "age_bucket", "path", "privacy", "raw_access", "air"],
    "DynamicsRow": ["kind", "id", "source", "status", "severity", "count", "detail", "air"],
    "DynamicsWorkbenchDefaults": ["inquiry_mode", "audience_mode", "explanation_path", "air"],
    "DynamicsWorkbenchInquiry": ["id", "label", "lens", "prompt", "answer_shape", "focus_node_ids",
                                 "focus_edge_ids", "air"],
    "DynamicsWorkbenchAudience": ["id", "label", "emphasis", "air"],
    "DynamicsWorkbenchScene": ["title", "lens", "selection_group", "selection_id", "takeaway", "caveat", "air"],
    "DynamicsWorkbenchExplanation": ["id", "label", "summary", "must_include", "scene_count", "scenes", "air"],
    "DynamicsWorkbench": ["status", "missing", "defaults", "inquiry_modes", "audience_modes",
                          "explanation_paths", "follow_on_tranches", "air"],
    "IntakeSource": ["id", "path", "exists", "mtime", "age_bucket", "status", "count", "privacy", "raw_access", "air"],
    "IntakeRow": ["id", "source", "kind", "status", "severity", "count", "blocker", "coverage", "task_link_state",
                  "evidence_count", "age_bucket", "authority", "evidence", "missing", "action", "detail",
                  "source_refs", "next_evidence", "air"],
    "CapabilitySource": ["id", "path", "exists", "mtime", "age_bucket", "status", "count", "detail", "privacy",
                         "raw_access", "air"],
    "CapabilityRow": ["capability_id", "status", "authority", "capability_class", "surface_family", "spend_model",
                      "egress_class", "receipt_requirement", "route_count", "ok_count", "blocked_count",
                      "evidence_count", "blocker", "hkp_posture", "source_refs", "source_ref_labels", "air"],
    "CapabilityRoute": ["route_id", "capability_id", "platform", "mode", "profile", "model_id", "effort",
                        "context_mode", "fast_mode", "quantization", "capacity_pool", "demand_vector", "hardening",
                        "eval_plane", "review_obligation", "learning_eligibility", "benchmark_coverage",
                        "fixed_overhead", "route_state", "authority_ceiling", "freshness_ok", "quota_state",
                        "receipt_count", "blockers", "evidence_count", "air"],
    "CapabilityTool": ["route_id", "platform", "tool_id", "status", "available", "authority_use", "observed_at",
                       "stale_after", "evidence_ref", "privacy", "raw_access", "air"],
    "GateSource": ["id", "status", "count", "detail", "age_bucket", "path", "raw_access", "air"],
    "GateRow": ["gate_id", "domain", "source", "subject", "state", "severity", "authority", "evidence",
                "missing", "action", "air"],
    "DomainSource": ["id", "path", "exists", "status", "count", "age_bucket", "authority", "detail", "privacy",
                     "raw_access", "air"],
    "DomainRow": ["domain_id", "label", "lifecycle", "terrain", "depth", "scope", "state", "authority_ceiling",
                  "claim_ceiling", "windows", "surfaces", "parity", "evidence_count", "blocker", "source_refs", "air"],
    "DomainRelation": ["source", "target", "relation", "authority_ceiling", "source_refs", "air"],
    "LifecycleRow": ["lifecycle_id", "label", "owner", "scope", "plant", "posture", "state", "maturity",
                     "adapter_id", "authority_ceiling", "claim_surface", "mutation_surface", "dark_policy",
                     "freshness_policy", "air_class", "windows", "surfaces", "commands", "receipt_contracts",
                     "evidence_count", "blocker", "next_evidence", "source_refs", "air"],
    "EpistemicSource": ["id", "status", "count", "detail", "age_bucket", "path", "privacy", "raw_access", "air"],
    "EpistemicReadRow": ["row_id", "family", "subject_kind", "subject_ref", "subject", "status", "posture",
                         "authority", "authority_case", "evidence_count", "evidence", "source", "source_refs",
                         "source_ref_labels", "freshness", "privacy", "raw_access", "missing", "action", "detail",
                         "map_kind", "map_id", "map_source", "map_target", "map_relation", "air"],
    # Reins slice 1 — the lanebus unread projection (read-only, metadata only).
    "LanebusInbox": ["inbox", "path", "exists", "state", "read_dir", "files_present", "files_acked",
                     "unread_count", "oldest_unread_age_s", "unread", "unread_rows_truncated", "ack", "air"],
    "LanebusUnread": ["basename", "mtime", "age_s", "air"],
    "LanebusTotals": ["inboxes", "unknown_inboxes", "files_present", "files_acked", "unread_count",
                      "unmeasured_inboxes", "oldest_unread_age_s"],
    # Reins #50 — the read-only grant-consumption projection.
    "GrantConsumption": ["dark", "error", "authority", "coverage", "grants"],
    "GrantRow": ["grant_id", "grant_record_present", "minting_lane", "minting_session", "binding", "lease",
                 "issued_at", "expires_at", "state", "honors", "honor_count", "air"],
    "GrantHonor": ["role", "session", "ts", "air"],
    # Vault summary note rows (path-like fields vault_root, rel_path, folder are SENSITIVE -> deny on air).
    "VaultSummary": ["vault_root", "notes"],
    "VaultNote": ["rel_path", "folder", "title", "modified", "size", "status", "tags", "obsidian_uri"],
    # Reins #53 — the read-only quota-phase-history projection.
    "QuotaPhaseHistory": [
        "schema_version", "state", "dark", "reason", "retention", "history_complete",
        "capture_coverage", "started_at", "sample_interval_s", "capacity", "evicted_rows",
        "route_count", "last_attempt_at", "last_success_at", "source_captured_at",
        "source_age_s", "observation_gap", "observation_gap_count", "last_gap_started_at",
        "last_gap_ended_at", "rows",
    ],
    "QuotaPhaseHistoryRow": [
        "route_id", "captured_at", "snapshot_captured_at", "sampled_at", "kind",
        "prior_state", "state", "prior_subscription_quota_state", "subscription_quota_state",
        "prior_fresh_until", "fresh_until", "fresh_until_delta_s", "after_observation_gap", "air",
    ],
    # Reins #51 — the read-only seat-succession projection.
    "SeatSuccession": ["state", "rows"],
    "SeatSuccessionRow": ["role", "session", "pane_state", "panes", "claims", "charter_state", "assigned_to", "air"],
    "SeatSuccessionPane": ["id", "pid", "command", "air"],
    "SeatSuccessionClaims": ["state", "present", "paths", "air"],
}


def _collect_payload_attrs(value: object, out: set[str], parent_key: str = "") -> None:
    """Recursively collect every attribute key emitted in a /read/* JSON payload.

    Skips keys inside dynamic map envelopes whose keys are data values (e.g., `air` per-field
    verdict maps, `by_kind` dynamic counters, `projections` keyed by audience name) while still
    walking nested projection objects inside `projections`.
    """
    if isinstance(value, list):
        for item in value:
            _collect_payload_attrs(item, out, parent_key)
    elif isinstance(value, dict):
        if parent_key in ("air", "by_kind", "facet_by_name", "facets"):
            return
        if parent_key == "projections":
            for nested in value.values():
                _collect_payload_attrs(nested, out, "")
            return
        for key, item in value.items():
            key_str = str(key)
            out.add(key_str)
            _collect_payload_attrs(item, out, key_str)


# Routes that emit populated domain rows/keys beyond their dark envelope in _populated_read_route_cfg,
# vs. the two routes whose live producers are external (Langfuse HTTP and CapabilityIO block stream)
# and therefore remain intentionally honest-dark in offline unit tests.
POPULATED_ROUTES: frozenset[str] = frozenset({
    "/read/capabilities",
    "/read/claim-custody",
    "/read/context",
    "/read/domains",
    "/read/dynamics",
    "/read/epistemics",
    "/read/events",
    "/read/gates",
    "/read/grant-consumption",
    "/read/intake",
    "/read/lanebus",
    "/read/observe",
    "/read/quota-history",
    "/read/seat-succession",
    "/read/session/{role}",
    "/read/session/{role}/turns",
    "/read/sessions",
    "/read/tasks",
    "/read/vault",
})

DARK_ONLY_ROUTES: frozenset[str] = frozenset({
    "/read/session/{role}/turns/{ts}/blocks",
    "/read/traces",
})


def _populated_read_route_cfg(tmp_path, monkeypatch) -> tuple[str, dict]:
    """Build a temporary read-only fixture config so all 16 POPULATED_ROUTES emit populated rows
    and keys beyond their unconfigured/dark envelopes (including empty bound HAPAX_SPINE_CONFIG_DIR
    and capability_inventory_path)."""
    import json
    import os
    from hapax.spine.coord_event_log import CoordEvent, CoordWriter, default_event_log

    council_root = tmp_path / "council"
    (council_root / "config").mkdir(parents=True, exist_ok=True)
    monkeypatch.setenv("HAPAX_SPINE_CONFIG_DIR", str(council_root / "config"))

    coord_dir = tmp_path / "coord"
    coord_dir.mkdir(parents=True, exist_ok=True)
    monkeypatch.setenv("HAPAX_COORD_DIR", str(coord_dir))
    log = default_event_log()
    log.append(
        CoordEvent(
            event_id="evt-fixture-1",
            event_type="sdlc.stage_transition",
            timestamp="2026-10-20T00:00:00Z",
            actor="cc-reins",
            subject="task-fixture-1",
            authority_case="A1",
            payload={"from_stage": "S1", "to_stage": "S2", "summary": "fixture transition"},
        ),
        writer=CoordWriter(kind="daemon", name="coord-daemon"),
    )

    state_file = tmp_path / "coordinator-state.json"
    state_file.write_text(
        json.dumps(
            {
                "lanes": {
                    "cc-reins": {
                        "platform": "claude",
                        "state": "active",
                        "session": "tmux-reins",
                        "claimed_task": "task-fixture-1",
                    }
                }
            }
        ),
        encoding="utf-8",
    )
    os.utime(state_file, (1000.0, 1000.0))
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(state_file))

    grants_dir = tmp_path / "grants"
    grants_dir.mkdir()
    gid = "a" * 32
    (grants_dir / "first.grant").write_text(
        json.dumps(
            {
                "kind": "escape",
                "grant_id": gid,
                "role": "lane-mint",
                "session": "session-mint",
                "issued_at": "2026-10-20T00:00:00Z",
                "expires_at": "2026-10-20T01:00:00Z",
                "lease_state": "active",
            }
        ),
        encoding="utf-8",
    )
    honor_ledger = tmp_path / "honors.jsonl"
    honor_ledger.write_text(
        json.dumps(
            {
                "kind": "escape_grant_honored",
                "grant_id": gid,
                "role": "lane-use",
                "session": "session-use",
                "ts": "2026-10-20T00:05:00Z",
            }
        )
        + "\n",
        encoding="utf-8",
    )

    inbox_dir = tmp_path / "inbox-alpha"
    (inbox_dir / "read").mkdir(parents=True)
    (inbox_dir / "20261020T000000Z-sample.md").write_text("hello\n", encoding="utf-8")

    vault_root = tmp_path / "vault"
    (vault_root / "30-areas").mkdir(parents=True)
    (vault_root / "30-areas" / "note.md").write_text(
        "---\nstatus: active\ntags: [reins]\n---\n# Note\n",
        encoding="utf-8",
    )

    bundle_path = council_root / "reins-context-bundle.json"
    bundle_path.write_text(
        json.dumps(
            {
                "evaluation": {"bundle_state": "lit"},
                "facts": {
                    "tasks": [
                        {
                            "fact_id": "f-1",
                            "subject_ref": "task-1",
                            "freshness_state": "fresh",
                            "confidence_word": "observed",
                            "text_domain": True,
                            "state": {"value_state": "lit", "reason_codes": []},
                            "air": {
                                "operator_private": "allow",
                                "yard_context": "allow",
                                "hapax_substrate": "allow",
                                "public_or_air": "deny",
                            },
                        }
                    ]
                },
            }
        ),
        encoding="utf-8",
    )
    monkeypatch.setenv("REINS_CONTEXT_BUNDLE", str(bundle_path))

    arch_dir = council_root / "docs" / "architecture"
    arch_dir.mkdir(parents=True, exist_ok=True)
    (arch_dir / "system-dynamics-map.seed.json").write_text(
        json.dumps(
            {
                "map_id": "sdm-1",
                "thesis": "fixture thesis",
                "layers": [{"id": "L1", "label": "Layer 1"}],
                "nodes": [
                    {
                        "id": "N1",
                        "label": "Node 1",
                        "kind": "stock",
                        "layer": "L1",
                        "status": "observed",
                        "resolution": "high",
                        "summary": "s",
                        "context": "c",
                        "docs": ["doc-1"],
                    }
                ],
                "edges": [
                    {
                        "id": "E1",
                        "source": "N1",
                        "target": "N1",
                        "relation": "drives",
                        "status": "observed",
                        "layer": "L1",
                        "resolution": "high",
                        "confidence": "high",
                        "summary": "e",
                        "docs": ["doc-1"],
                    }
                ],
            }
        ),
        encoding="utf-8",
    )
    (arch_dir / "system-dynamics-map.claims.json").write_text(
        json.dumps({"claims": [{"id": "C1", "status": "observed", "summary": "claim"}]}),
        encoding="utf-8",
    )

    cap_inv = tmp_path / "capability-inventory.json"
    cap_inv.write_text(
        json.dumps(
            {
                "total": 1,
                "with_validation_gaps": 0,
                "freshness_counts": {"dark": 0},
                "rows": [
                    {
                        "capability_id": "cap-fixture-1",
                        "shape": "cli",
                        "domain": "sdlc",
                        "freshness_state": "fresh",
                        "authority_ceiling": "metadata-only",
                        "gaps": [],
                    }
                ],
            }
        ),
        encoding="utf-8",
    )

    domain_pack = tmp_path / "domain-pack.json"
    domain_pack.write_text(
        json.dumps(
            {
                "authority": "metadata-only",
                "generated_at": "2026-10-20T00:00:00Z",
                "default_lens": "overview",
                "domains": [
                    {
                        "domain_id": "dom-1",
                        "label": "Domain 1",
                        "lifecycle": "sdlc",
                        "terrain": "code",
                        "depth": "deep",
                        "scope": "instance",
                        "state": "observed",
                        "windows": ["w1"],
                        "surfaces": ["s1"],
                        "parity": "full",
                    }
                ],
                "relations": [{"source": "dom-1", "target": "dom-1", "relation": "contains"}],
            }
        ),
        encoding="utf-8",
    )

    req_intake = tmp_path / "request-intake.json"
    req_intake.write_text(
        json.dumps({"combined_attention_count": 1, "malformed_count": 1, "unread_count": 0, "stale_count": 0}),
        encoding="utf-8",
    )
    plan_intake = tmp_path / "planning-feed.json"
    plan_intake.write_text(
        json.dumps({"total_requests": 2, "attention_count": 1}),
        encoding="utf-8",
    )
    p0_state = tmp_path / "p0-state.json"
    p0_state.write_text(json.dumps({"open_count": 1}), encoding="utf-8")
    p0_events = tmp_path / "p0-events.jsonl"
    p0_events.write_text(json.dumps({"id": "p0-1", "status": "open"}) + "\n", encoding="utf-8")
    sec_state = tmp_path / "security-signals.json"
    sec_state.write_text(json.dumps({"open_count": 1}), encoding="utf-8")

    quota_ledger = tmp_path / "quota-spend-ledger-live.json"
    quota_ledger.write_text(
        json.dumps(
            {
                "captured_at": "2026-10-20T00:05:00Z",
                "quota_snapshots": [
                    {
                        "route_id": "claude.max2",
                        "state": "admitted",
                        "subscription_quota_state": "fresh",
                        "fresh_until": "2026-10-20T05:00:00Z",
                        "captured_at": "2026-10-20T00:04:00Z",
                    }
                ],
            }
        ),
        encoding="utf-8",
    )
    charter_path = tmp_path / "charter-alpha.md"
    charter_path.write_text("---\nassigned_to: alpha\n---\n", encoding="utf-8")
    marker_path = tmp_path / "cc-active-task-alpha"
    marker_path.write_text("task-1\n", encoding="utf-8")
    custody_rows = tmp_path / "custody-rows"
    custody_rows.mkdir(parents=True, exist_ok=True)
    (custody_rows / "work-a-slug.md").write_text(
        "---\ntask_id: work-a\nstatus: ready\nassigned_to: alpha\nbranch: feat/a\npr: null\ndepends_on: [work-b]\n---\n"
        "## Session log\n- 2026-10-20T00:00:00Z alpha claimed (cc-claim, session=s1)\n",
        encoding="utf-8",
    )
    custody_marker = tmp_path / "custody-claim"
    custody_marker.write_text("work-a\n", encoding="utf-8")

    cfg = {
        "council_root": str(council_root),
        "grant_dir": str(grants_dir),
        "grant_honor_ledger": str(honor_ledger),
        "quota_spend_ledger_live": str(quota_ledger),
        "lanebus_inboxes": [
            {"inbox": "alpha", "path": str(inbox_dir), "count_only": False, "ack": "read-dir"}
        ],
        "seat_succession": {
            "socket": str(tmp_path / "dead.sock"),
            "lanes": [
                {
                    "role": "alpha",
                    "session": "hapax-claude-alpha",
                    "claim_marker_pattern": str(tmp_path / "cc-active-task-alpha*"),
                    "charter_path": str(charter_path),
                }
            ],
        },
        "claim_custody": {
            "rows_dirs": [str(custody_rows)],
            "task_ids": ["work-a", "work-missing"],
            "claim_markers": [{"role": "alpha", "path": str(custody_marker)}],
        },
        "vault_root": str(vault_root),
        "capability_inventory_path": str(cap_inv),
        "domain_pack_paths": [str(domain_pack)],
        "request_intake_state": str(req_intake),
        "planning_feed_state": str(plan_intake),
        "p0_incident_state": str(p0_state),
        "p0_incident_events": str(p0_events),
        "security_signal_state": str(sec_state),
    }
    return str(council_root), cfg


def test_every_live_attr_resolves(tmp_path, monkeypatch):
    """Exhaustiveness: no live (domain, attr) in INVENTORY or registered /read/* route payload key
    lands in UNRESOLVED limbo.

    Walks all registered GET /read/* routes (excluding /read/facets) in both deterministic dark
    mode and populated fixture mode:
      - routes in POPULATED_ROUTES must emit populated keys beyond their dark envelope (with
        non-empty grants + honors pinned on /read/grant-consumption, and empty bound
        HAPAX_SPINE_CONFIG_DIR + capability_inventory_path pinned on /read/capabilities);
      - 2 routes in DARK_ONLY_ROUTES (/read/traces and /read/session/{role}/turns/{ts}/blocks) are
        explicitly declared dark-only in offline unit tests, so no newly added route can silently
        pass as an unpopulated envelope."""
    import reins_read as rr
    import reins_seat_succession as ss
    from fastapi.testclient import TestClient

    unresolved = []
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            r = fr.classify(domain, attr)
            if r.startswith("UNRESOLVED"):
                unresolved.append(f"{domain}.{attr}")

    # 1. Deterministic dark walk (unbound spine config and absent coordinator/context files).
    monkeypatch.delenv("HAPAX_SPINE_CONFIG_DIR", raising=False)
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(tmp_path / "absent-coordinator-state.json"))
    monkeypatch.setenv("REINS_CONTEXT_BUNDLE", str(tmp_path / "absent-context-bundle.json"))
    dark_app = rr.build_app("", fr.air_allowlist(), {})
    read_routes = sorted(
        r.path
        for r in dark_app.routes
        if getattr(r, "path", "").startswith("/read/") and r.path != "/read/facets"
    )
    assert set(read_routes) == (POPULATED_ROUTES | DARK_ONLY_ROUTES), (
        f"Every /read/* route must be explicitly classified in POPULATED_ROUTES or DARK_ONLY_ROUTES; "
        f"diff={set(read_routes) ^ (POPULATED_ROUTES | DARK_ONLY_ROUTES)}"
    )
    assert not (POPULATED_ROUTES & DARK_ONLY_ROUTES)

    dark_attrs_by_route: dict[str, set[str]] = {}
    all_walked_attrs: set[str] = set()
    with TestClient(dark_app) as client:
        for route_path in read_routes:
            url = route_path.replace("{role}", "absent-role").replace("{ts}", "2026-06-26T18:40:01Z")
            resp = client.get(url)
            assert resp.status_code == 200, f"{route_path} (dark) returned {resp.status_code}"
            attrs: set[str] = set()
            _collect_payload_attrs(resp.json(), attrs)
            assert attrs, f"{route_path} (dark) emitted an empty payload"
            dark_attrs_by_route[route_path] = attrs
            all_walked_attrs |= attrs
            for attr in sorted(attrs):
                if fr.classify("", attr).startswith("UNRESOLVED"):
                    unresolved.append(f"{route_path}[dark].{attr}")

    # 2. Populated fixture walk (bound HAPAX_SPINE_CONFIG_DIR + populated fixtures for all POPULATED_ROUTES).
    council_root, pop_cfg = _populated_read_route_cfg(tmp_path, monkeypatch)
    monkeypatch.setattr(
        ss,
        "_pane_census",
        lambda _spec: {"hapax-claude-alpha": [{"id": "%1", "pid": 101, "command": "bash"}]},
    )
    pop_app = rr.build_app(council_root, fr.air_allowlist(), pop_cfg)
    with TestClient(pop_app) as client:
        for route_path in read_routes:
            url = route_path.replace("{role}", "cc-reins").replace("{ts}", "2026-06-26T18:40:01Z")
            resp = client.get(url)
            assert resp.status_code == 200, f"{route_path} (populated) returned {resp.status_code}"
            payload = resp.json()
            if route_path == "/read/grant-consumption":
                assert payload.get("dark") is False
                assert len(payload.get("grants") or []) >= 1, "populated /read/grant-consumption must emit grants"
                assert len(payload["grants"][0].get("honors") or []) >= 1, "populated grant must emit honors"
            if route_path == "/read/capabilities":
                assert payload.get("dark") is False
            attrs = set()
            _collect_payload_attrs(payload, attrs)
            assert attrs, f"{route_path} (populated) emitted an empty payload"
            all_walked_attrs |= attrs
            if route_path in POPULATED_ROUTES:
                if route_path == "/read/gates":
                    assert payload.get("dark") is False and len((payload.get("gates") or {}).get("rows") or []) >= 1
                elif route_path == "/read/observe":
                    assert len(payload.get("dimensions") or []) >= 1
                else:
                    added = attrs - dark_attrs_by_route[route_path]
                    assert added, (
                        f"Populated route {route_path} emitted no keys beyond its dark shape: {sorted(attrs)}"
                    )
            for attr in sorted(attrs):
                if fr.classify("", attr).startswith("UNRESOLVED"):
                    unresolved.append(f"{route_path}[populated].{attr}")

    # Pin that every newly registered attribute in #56 is actually emitted by the route walk (R2).
    newly_registered_56 = {
        "affordance_kind", "affordances", "artifacts", "audience", "audiences", "binding",
        "blocked", "blocks", "bundle_state", "capabilities", "confidence_word", "dark",
        "dimensions", "domains", "epistemics", "error", "events", "expires_at", "fact_count",
        "fact_id", "facts", "false_gates", "fixture_only", "folder", "freshness_state", "gates",
        "grant_id", "grant_record_present", "grants", "honor_count", "honors", "intake",
        "issued_at", "key", "lanes", "lease", "lifecycle", "map_edges", "map_nodes",
        "minting_lane", "minting_session", "missing_evidence", "missing_sources", "modified",
        "notes", "obsidian_uri", "oldest_ts", "p0_incidents", "p0_ledger_tail", "package_rows",
        "partial", "planning_attention", "planning_requests", "preview", "prior_stage_state",
        "producer", "projections", "prov", "reason", "reason_codes", "receipts", "rel_path",
        "request_attention", "request_malformed", "security_signals", "sessions", "shape",
        "stale", "tasks", "text_domain", "traces", "turns", "value_state", "vault_root", "why",
    }
    unemitted_56 = newly_registered_56 - all_walked_attrs
    assert not unemitted_56, f"Newly registered #56 attrs not emitted by any fixture: {sorted(unemitted_56)}"

    assert not unresolved, (
        f"{len(unresolved)} attrs do not resolve to a facet/edge/body/envelope: {unresolved}"
    )


def test_exactly_nine_facets_in_citation_order():
    assert len(fr.FACETS) == 9
    assert set(fr.CITATION_ORDER) == set(fr.FACETS), "citation order must cover exactly the 9 facets"


def test_each_facet_fully_specified():
    for k, f in fr.FACETS.items():
        for field in ("gloss", "question", "role", "channel", "air"):
            assert f.get(field), f"facet {k} missing {field}"


def _channel_kind(prose: str) -> str | None:
    """Mirror of the Go ChannelFromProse precedence (internal/grammar/encoder.go): the cell-grammar
    encoder binds on this channel prose, so a re-wording this cannot classify silently drops a meaning
    channel on air. Keep in lockstep with the Go parser."""
    p = prose.lower()
    if "criticality-hue" in p:
        return "criticality-hue"
    if "family-hue" in p:
        return "family-hue"
    if "block bar" in p or "eighth-block" in p:
        return "magnitude-bar"
    if "freshness" in p:
        return "freshness-dim"
    if "pip" in p:
        return "provenance-pip"
    if "text" in p:
        return "text"
    return None


def test_every_facet_channel_prose_is_encoder_recognized():
    """Every facet's channel prose must bind to exactly one cell channel — the Go encoder reads THIS
    prose as its SSOT, so an unrecognized re-wording would drop a channel on air (Gate-13). The
    Go-side cross-language counterpart is TestChannelBindingMatchesPythonRegistry."""
    expected = {
        "identity": "text", "posture": "criticality-hue", "action": "text",
        "ownership": "family-hue", "place": "family-hue", "time": "freshness-dim",
        "provenance": "provenance-pip", "measure": "magnitude-bar", "variant": "text",
    }
    assert set(expected) == set(fr.FACETS), "the channel-binding expectation must cover exactly the 9 facets"
    for facet, f in fr.FACETS.items():
        kind = _channel_kind(f["channel"])
        assert kind is not None, f"facet {facet} channel {f['channel']!r} is unrecognized by the encoder"
        assert kind == expected[facet], f"facet {facet} channel {f['channel']!r} -> {kind}, expected {expected[facet]}"


def test_air_defaults_are_consistent():
    assert fr.air_default("body") == "deny"           # bodies never air
    assert fr.air_default("provenance") == "gate"      # provenance gates
    assert fr.air_default("posture") == "air"          # skeleton facets air
    assert fr.air_default("measure") == "air"
    assert fr.air_default("edge") == "air"             # edge existence airs (endpoints gate themselves)
    assert fr.air_default("envelope") == "n/a"


def test_status_homonym_resolves_per_domain():
    # the field-level homonym (same disease as the authority_ceiling knot): status carries the
    # provenance confidence-ladder on the graph/epistemic domains, a posture state elsewhere.
    assert fr.classify("Node", "status") == "provenance"
    assert fr.classify("Edge", "status") == "provenance"
    assert fr.classify("EpistemicReadRow", "status") == "provenance"
    assert fr.classify("IntakeRow", "status") == "posture"
    assert fr.classify("GateRow", "state") == "posture"


def test_a6_repairs_default_allow_skeleton_gap():
    # the decision-support audit found these structural-skeleton fields are MISSING from _DEFAULT_ALLOW
    # and thus default-DENY on air today. Under per-facet AIR they resolve to air-able facets => the
    # bimodal-AIR "skeleton airs" intent is realized by construction.
    skeleton_gap = {
        ("Task", "criticality"), ("Task", "freshness"), ("Task", "prior_stage"),
        ("Task", "predicted_stage"), ("Task", "rel_count"),
        ("Turn", "magnitude"), ("Turn", "route"), ("Turn", "gate"), ("TurnBlock", "meta"),
    }
    for domain, attr in skeleton_gap:
        r = fr.classify(domain, attr)
        assert fr.air_default(r) == "air", f"{domain}.{attr} -> {r} should air as skeleton, not deny"


def test_relations_are_edges_not_facets():
    # framework §3: a relation is a property of a PAIR (an entity reference), kept OUTSIDE the facets.
    for domain, attr in [("Edge", "source"), ("Edge", "target"), ("Edge", "relation"),
                         ("Session", "claimed_task"), ("DomainRow", "lifecycle"),
                         ("EpistemicReadRow", "map_target")]:
        assert fr.classify(domain, attr) == "edge", f"{domain}.{attr} should be an edge"


def test_sensitive_attrs_deny_on_air_regardless_of_facet():
    # path (filesystem PII), session/minting_session (tmux/session name), and vault path fields
    # (vault_root, rel_path, folder) are place-facet but DENY on air (per-attr override).
    assert fr.classify("EvidenceRef", "path") == "place"
    assert fr.air_policy("EvidenceRef", "path") == "deny"
    assert fr.air_policy("Session", "session") == "deny"
    for pii_place in ("rel_path", "vault_root", "folder", "minting_session"):
        assert fr.classify("", pii_place) == "place", f"{pii_place} must classify as place"
        assert fr.air_policy("", pii_place) == "deny", f"{pii_place} must DENY on air"
        assert pii_place not in fr.air_allowlist(), f"{pii_place} must NOT enter air_allowlist()"
    # a non-sensitive place attr still airs:
    assert fr.air_policy("Session", "platform") == "air"


def test_bodies_default_deny():
    for domain, attr in [("Event", "summary"), ("Node", "context"), ("IntakeRow", "detail")]:
        assert fr.classify(domain, attr) == "body"
        assert fr.air_default("body") == "deny"


# Mirrors reins_read._DEFAULT_ALLOW (keep in sync; the safety parity test below depends on it).
CURRENT_ALLOW = frozenset((
    "kind,score,ts,task_id,stage,no_go,id,layer,status,source,target,relation,res,role,platform,"
    "state,alive,idle,stalled,output_age_s,relay_age_s,readiness,blocker,attention,evidence_count,"
    "resume_ready,evidence_summary,by_kind,transcript_roots_observed,transcript_roots_missing,truncated,"
    "count,age_bucket,coverage,task_link_state,severity,privacy,raw_access,exists,capability_id,"
    "capability_class,surface_family,spend_model,egress_class,receipt_requirement,route_count,ok_count,"
    "blocked_count,hkp_posture,source_refs,source_ref_labels,route_id,mode,profile,model_id,effort,"
    "context_mode,fast_mode,quantization,capacity_pool,demand_vector,hardening,eval_plane,review_obligation,"
    "learning_eligibility,benchmark_coverage,fixed_overhead,route_state,authority_ceiling,freshness_ok,"
    "quota_state,receipt_count,blockers,authority,route_binding_state,tool_id,available,authority_use,"
    "observed_at,stale_after,schema_version,row_id,family,subject_kind,subject_ref,posture,map_kind,map_id,"
    "map_source,map_target,map_relation,gate_id,domain,evidence,missing,action,detail,generated_at,"
    "package_hash,default_lens,domain_id,lifecycle,terrain,depth,scope,claim_ceiling,windows,surfaces,"
    "parity,lifecycle_id,owner,plant,maturity,adapter_id,claim_surface,mutation_surface,dark_policy,"
    "freshness_policy,air_class,commands,receipt_contracts,next_evidence").split(","))


def test_safety_registry_never_airs_a_body():
    """SAFETY INVARIANT (on-air egress, highest blast radius): no free-text BODY field may air under
    the registry's per-attribute AIR policy — bodies are default-deny, always."""
    leaks = []
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            if fr.classify(domain, attr) == "body" and fr.air_policy(domain, attr) != "deny":
                leaks.append(f"{domain}.{attr}")
    assert not leaks, f"registry would AIR a free-text body (PII risk): {leaks}"


def test_safety_newly_aired_fields_are_only_safe_structural():
    """vs the live _DEFAULT_ALLOW, the registry may NEWLY-AIR only safe structural/operational fields
    (the documented skeleton repair + structured identity/ownership/variant) — NEVER a free-text or
    PII-bearing field, and NEVER a financial one. Pins that adopting the registry as the live
    allowlist cannot leak.

    Vets EVERY name that the whole allowlist (`fr.air_allowlist()`) newly airs vs `CURRENT_ALLOW`
    (not only `INVENTORY`), so removing any entry from `SENSITIVE` (e.g., `rel_path`, `vault_root`,
    `folder`, `minting_session`) immediately turns this test red.

    Two deny-reasons, not one: PII (who) and FINANCIAL (how much). This test originally modelled only
    PII, which is exactly how "cost" came to be vetted as safe — it is numbers with no PII, and it
    still must not broadcast."""
    # VETTED safe newly-aired set across the WHOLE air_allowlist() (each eyeballed: structural/
    # operational, NO PII/free-text). A NEW field that newly-airs trips this test for re-vetting.
    VETTED_NEWLY_AIR = {
        # the skeleton repair (were denied, operational metadata):
        "criticality", "freshness", "prior_stage", "predicted_stage", "rel_count",
        "magnitude", "gate", "meta",
        # Trace LLM-metadata repair (numbers, no PII) — "cost" is FINANCIAL-denied:
        "latency_ms", "prompt_tok", "completion_tok", "total_tok", "size", "total",
        # recency timestamps (Time facet):
        "activity_age_s", "mtime", "updated_at", "modified", "oldest_ts",
        # structured identity/ownership/action/variant/provenance (ids, lane names, verbs, enum tokens — not free-text):
        "actor", "assigned_to", "model", "trace_id", "intent", "ready", "attached",
        "lens", "inquiry_mode", "audience_mode", "explanation_path",
        "authority_case", "confidence", "prov",
        # edge ref IDs (task/node/route ids — safe; path-like refs are SENSITIVE-denied):
        "claimed_task", "route", "focus_node_ids", "focus_edge_ids", "selection_group", "selection_id",
        # Reins slice 1: four counts, a directory condition token, two ages. `basename` and `inbox`
        # are SENSITIVE (deny on air).
        "files_present", "files_acked", "unread_count", "unknown_inboxes", "read_dir",
        "oldest_unread_age_s", "age_s", "ack", "unread_rows_truncated", "unmeasured_inboxes",
        # Reins #50 + honest-dark: operational grant metadata (8-hex id prefix, presence bool,
        # minting lane role, lease timestamps, honor count, dark bool). `minting_session` is
        # SENSITIVE (denies on air), `error` is a BODY (denies on air), and `grants`/`binding`/
        # `lease`/`honors` are ENVELOPES (do not enter air_allowlist).
        "dark", "grant_id", "grant_record_present", "minting_lane", "issued_at", "expires_at",
        "honor_count",
        # Reins #51: operational seat-succession posture/identity fields. `paths` and `command`
        # are SENSITIVE (deny on air), and `panes` is an ENVELOPE.
        "pane_state", "present", "charter_state", "pid",
        # Reins #56 read-surface structural/operational fields (counts, booleans, state tokens,
        # enum-like ids/labels; path-PII fields `rel_path`, `vault_root`, `folder` are SENSITIVE):
        "affordance_kind", "audience", "blocked", "bundle_state", "confidence_word",
        "depth_count", "fact_count", "fact_id", "false_gates", "fixture_only",
        "freshness_state", "key", "lanes", "map_edges", "map_nodes",
        "missing_evidence", "missing_sources", "p0_incidents", "p0_ledger_tail",
        "package_rows", "partial", "planning_attention", "planning_requests",
        "preview", "prior_stage_state", "receipts", "request_attention",
        "request_malformed", "security_signals", "shape", "stale", "text_domain",
        "value_state",
        # Reins #53 quota-phase-history operational/structural fields (timestamps, durations,
        # bounded counts, posture states, and retention token; no PII or financial spend):
        "after_observation_gap", "capacity", "capture_coverage", "captured_at",
        "evicted_rows", "fresh_until", "fresh_until_delta_s", "history_complete",
        "last_attempt_at", "last_gap_ended_at", "last_gap_started_at", "last_success_at",
        "observation_gap", "observation_gap_count", "prior_fresh_until", "prior_state",
        "prior_subscription_quota_state", "retention", "sample_interval_s", "sampled_at",
        "snapshot_captured_at", "source_age_s", "source_captured_at", "started_at",
        "subscription_quota_state",
        # Reins #52 claim-custody operational fields (path-PII fields `marker_dir` and
        # `marker_paths` are SENSITIVE -> deny on air):
        "claim_state", "created_at", "dropped", "events_truncated", "guard",
        "history_scope", "holders", "last_actor", "last_ts", "return_state",
        "sequence", "started",
    }
    newly_aired = {
        attr for attr in fr.air_allowlist() if attr not in CURRENT_ALLOW
    }
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            if attr not in CURRENT_ALLOW and fr.air_policy(domain, attr) == "air":
                newly_aired.add(attr)
    unexpected = newly_aired - VETTED_NEWLY_AIR
    assert not unexpected, f"registry would newly-air UNVETTED fields vs _DEFAULT_ALLOW: {sorted(unexpected)}"
    unexercised = VETTED_NEWLY_AIR - newly_aired
    assert not unexercised, f"VETTED_NEWLY_AIR contains stale/non-aired entries: {sorted(unexercised)}"


def test_air_allowlist_airs_skeleton_denies_pii_and_bodies():
    al = set(fr.air_allowlist())
    # the skeleton repair airs:
    for ok in ("criticality", "freshness", "prior_stage", "predicted_stage", "rel_count",
               "magnitude", "gate", "meta", "stage", "owner", "platform"):
        assert ok in al, f"{ok} should air"
    # PII + free-text bodies + path-like refs do NOT air:
    for deny in ("path", "session", "subject", "label", "title", "parent_spec", "evidence_ref",
                 "route_evidence_ref", "summary", "detail", "missing", "action", "blockers",
                 "basename", "inbox", "rel_path", "vault_root", "folder", "minting_session"):
        assert deny not in al, f"{deny} must NOT air"


def test_facets_payload_self_describes():
    p = fr.facets_payload()
    assert set(p["facets"]) == set(fr.FACETS) and len(p["facets"]) == 9
    assert p["air_allowlist"] == fr.air_allowlist()
    assert {"domain", "attr", "facet"} <= set(p["overrides"][0])


def test_registry_is_more_conservative_on_free_text():
    """The registry correctly NEWLY-DENIES free-text the current flat allowlist over-airs
    (detail/missing/action/next_evidence/blockers) — a safety improvement, documented here."""
    newly_denied_freetext = set()
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            if attr in CURRENT_ALLOW and fr.classify(domain, attr) == "body":
                newly_denied_freetext.add(attr)
    assert {"detail", "missing", "action", "next_evidence", "blockers"} <= newly_denied_freetext


def test_financial_fields_never_air_operator_bar_20260628():
    """The operator's dated bar, quoted verbatim in internal/config/config.go:

        operator AIR bar 2026-06-28 "air structural, DENY $cost" ... the financial "cost" is
        deliberately OMITTED so $ spend never broadcasts.

    The hand-maintained Go default honours it by omission. air_allowlist() must honour it by
    construction, so that replacing one with the other cannot silently re-admit spend.
    """
    al = set(fr.air_allowlist())
    for f in fr.FINANCIAL:
        assert f not in al, f"{f!r} airs — the 2026-06-28 bar says $ spend never broadcasts"


def test_financial_deny_is_by_category_not_by_one_name():
    """A spend field added to the registry later must be denied on arrival, not air until noticed.
    'cost' is the field that actually leaked; the set exists so the next one cannot."""
    assert "cost" in fr.FINANCIAL
    assert len(fr.FINANCIAL) > 1, "deny the category, not the single instance"
    # and the two deny-reasons stay distinct: a spend field is not PII and must not be filed as such
    assert not (fr.FINANCIAL & fr.SENSITIVE), "FINANCIAL and SENSITIVE model different reasons"


def test_cost_denied_but_non_financial_measures_still_air():
    """The bar is about spend, not about measures. Latency and token magnitudes still air — denying
    them would over-correct and break the skeleton repair the registry exists to deliver."""
    al = set(fr.air_allowlist())
    assert "cost" not in al
    for ok in ("latency_ms", "total_tok", "magnitude"):
        assert ok in al, f"{ok} should still air — it is a measure, not spend"


def test_both_air_decision_points_agree_on_financial():
    """air_policy() and air_allowlist() are two doors to the same decision. 'cost' leaked because the
    derived list was replaced without the per-attribute policy following it, so a fix to one door
    left the other open. Pin that they cannot diverge again."""
    al = set(fr.air_allowlist())
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            if attr in fr.FINANCIAL:
                assert fr.air_policy(domain, attr) == "deny", f"air_policy airs {attr!r}"
                assert attr not in al, f"air_allowlist airs {attr!r}"
