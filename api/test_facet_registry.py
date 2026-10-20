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


def _populated_read_route_cfg(tmp_path) -> tuple[str, dict]:
    """Build a temporary read-only fixture config so registered /read/* routes emit populated rows."""
    import json

    council_root = tmp_path / "council"
    council_root.mkdir(parents=True, exist_ok=True)

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
    (vault_root / "30-areas" / "note.md").write_text("# Note\n", encoding="utf-8")

    (council_root / "reins-context-bundle.json").write_text(
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

    charter_path = tmp_path / "charter-alpha.md"
    charter_path.write_text("---\nassigned_to: alpha\n---\n", encoding="utf-8")
    marker_path = tmp_path / "cc-active-task-alpha"
    marker_path.write_text("task-1\n", encoding="utf-8")

    cfg = {
        "council_root": str(council_root),
        "grant_dir": str(grants_dir),
        "grant_honor_ledger": str(honor_ledger),
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
        "vault_root": str(vault_root),
    }
    return str(council_root), cfg


def test_every_live_attr_resolves(tmp_path, monkeypatch):
    """Exhaustiveness: no live (domain, attr) or registered /read/* route payload key lands in
    UNRESOLVED limbo — generic over all registered /read/* routes in build_app so no future facet
    can escape."""
    import reins_read as rr
    import reins_seat_succession as ss
    from fastapi.testclient import TestClient

    unresolved = []
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            r = fr.classify(domain, attr)
            if r.startswith("UNRESOLVED"):
                unresolved.append(f"{domain}.{attr}")

    # Generic payload walk over every registered GET /read/* route on build_app (both dark/unconfigured
    # and populated fixtures). Any new /read/* route added to build_app is walked automatically.
    council_root, pop_cfg = _populated_read_route_cfg(tmp_path)
    monkeypatch.setattr(
        ss,
        "_pane_census",
        lambda _spec: {"hapax-claude-alpha": [{"id": "%1", "pid": 101, "command": "bash"}]},
    )
    apps = [
        ("dark", rr.build_app("", fr.air_allowlist(), {})),
        ("populated", rr.build_app(council_root, fr.air_allowlist(), pop_cfg)),
    ]
    seen_routes: set[str] = set()
    for mode, app in apps:
        read_routes = sorted(
            r.path
            for r in app.routes
            if getattr(r, "path", "").startswith("/read/") and r.path != "/read/facets"
        )
        assert read_routes, "build_app must register /read/* routes"
        with TestClient(app) as client:
            for route_path in read_routes:
                seen_routes.add(route_path)
                url = route_path.replace("{role}", "alpha").replace(
                    "{ts}", "2026-06-26T00:00:00Z"
                )
                resp = client.get(url)
                assert resp.status_code == 200, f"{route_path} ({mode}) returned {resp.status_code}"
                payload = resp.json()
                attrs: set[str] = set()
                _collect_payload_attrs(payload, attrs)
                assert attrs, f"{route_path} ({mode}) emitted an empty payload"
                for attr in sorted(attrs):
                    if fr.classify("", attr).startswith("UNRESOLVED"):
                        unresolved.append(f"{route_path}[{mode}].{attr}")

    assert "/read/grant-consumption" in seen_routes
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
    # path (filesystem PII) and session (tmux name) are place-facet but DENY on air (per-attr override).
    assert fr.classify("EvidenceRef", "path") == "place"
    assert fr.air_policy("EvidenceRef", "path") == "deny"
    assert fr.air_policy("Session", "session") == "deny"
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

    Two deny-reasons, not one: PII (who) and FINANCIAL (how much). This test originally modelled only
    PII, which is exactly how "cost" came to be vetted as safe — it is numbers with no PII, and it
    still must not broadcast."""
    # VETTED safe newly-aired set (each eyeballed: structural/operational, NO PII/free-text). A NEW
    # field that newly-airs trips this test for re-vetting (drift guard) — the safety review is pinned.
    VETTED_NEWLY_AIR = {
        # the skeleton repair (were denied, operational metadata):
        "criticality", "freshness", "prior_stage", "predicted_stage", "rel_count",
        "magnitude", "gate", "meta",
        # Trace LLM-metadata repair (numbers, no PII) — the flat list also omitted these.
        # "cost" was vetted here as "numbers, no PII" — true, and the wrong criterion: the operator
        # bar denies spend for broadcast-economics reasons, not privacy ones. It is FINANCIAL-denied.
        "latency_ms", "prompt_tok", "completion_tok", "total_tok", "size", "total",
        # recency timestamps (Time facet):
        "activity_age_s", "mtime", "updated_at",
        # structured identity/ownership/action/variant (ids, lane names, verbs — not free-text):
        "actor", "assigned_to", "model", "trace_id", "intent", "ready", "attached",
        "lens", "inquiry_mode", "audience_mode", "explanation_path",
        # edge ref IDs (task/node/route ids — safe; path-like refs are SENSITIVE-denied):
        "claimed_task", "route", "focus_node_ids", "focus_edge_ids", "selection_group", "selection_id",
        # Reins slice 1: four counts, a directory condition token, two ages. The projection's three
        # NAME-bearing fields are NOT here — `basename` and `inbox` are SENSITIVE (deny on air).
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
    }
    newly_aired = set()
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            if attr not in CURRENT_ALLOW and fr.air_policy(domain, attr) == "air":
                newly_aired.add(attr)
    unexpected = newly_aired - VETTED_NEWLY_AIR
    assert not unexpected, f"registry would newly-air UNVETTED fields vs _DEFAULT_ALLOW: {sorted(unexpected)}"


def test_air_allowlist_airs_skeleton_denies_pii_and_bodies():
    al = set(fr.air_allowlist())
    # the skeleton repair airs:
    for ok in ("criticality", "freshness", "prior_stage", "predicted_stage", "rel_count",
               "magnitude", "gate", "meta", "stage", "owner", "platform"):
        assert ok in al, f"{ok} should air"
    # PII + free-text bodies + path-like refs do NOT air:
    for deny in ("path", "session", "subject", "label", "title", "parent_spec", "evidence_ref",
                 "route_evidence_ref", "summary", "detail", "missing", "action", "blockers",
                 "basename", "inbox"):
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
