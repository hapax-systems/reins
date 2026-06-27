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
}


def test_every_live_attr_resolves():
    """Exhaustiveness: no live (domain, attr) lands in UNRESOLVED limbo — the whole point of the cut."""
    unresolved = []
    for domain, attrs in INVENTORY.items():
        for attr in attrs:
            r = fr.classify(domain, attr)
            if r.startswith("UNRESOLVED"):
                unresolved.append(f"{domain}.{attr}")
    assert not unresolved, f"{len(unresolved)} attrs do not resolve to a facet/edge/body/envelope: {unresolved}"


def test_exactly_nine_facets_in_citation_order():
    assert len(fr.FACETS) == 9
    assert set(fr.CITATION_ORDER) == set(fr.FACETS), "citation order must cover exactly the 9 facets"


def test_each_facet_fully_specified():
    for k, f in fr.FACETS.items():
        for field in ("gloss", "question", "role", "channel", "air"):
            assert f.get(field), f"facet {k} missing {field}"


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
