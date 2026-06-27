"""Reins facet registry — the SINGLE SOURCE OF TRUTH for the representational framework.

The keystone (framework §1 Layer-1 + the ratified facet cut, reins-design-ref):
every binnable attribute across every read domain decomposes into ONE of 9 orthogonal FACETS
(the common vocabulary), plus two layers kept OUTSIDE the facet set — EDGES (relations: a property
of a *pair*, the graph layer) and BODIES (free-text payload: default-deny on air) — and the
structural ENVELOPES (wrappers, not row attributes).

This registry drives, downstream: the cell-grammar ENCODER (facet.role -> cell channel), the DOI
fold (which facets carry the "what-matters" signal), typed-join COORDINATION (which facets are
cross-domain keys), and per-facet AIR (skeleton airs / Provenance gates / bodies deny — fixing the
flat _DEFAULT_ALLOW gap). Per A6 it is also the in-band DECODER: one source drives renderer + legend.

NOTE (the field-level homonym): some attribute NAMES carry different facets in different domains
(`status` = provenance-ladder on Node/Edge but posture elsewhere). So classification is keyed by
(domain, attr) with a by-name default + explicit per-domain OVERRIDES. The contract test
(test_facet_registry.py) fails if any live attribute is unresolved or if a name's facet conflicts
across domains without an explicit override.
"""

from __future__ import annotations

# ── THE 9 FACETS ─────────────────────────────────────────────────────────────
# key: gloss (the cold-read legible key — A6) · characteristic (the single question) ·
# role (-> cell channel) · channel · air (skeleton airs / Provenance gates / Measure airs).
# citation order = decreasing concreteness (presentation-only / reorderable, per ratification).
FACETS: dict[str, dict[str, str]] = {
    "identity":   {"gloss": "what it is",          "question": "what is this thing?",
                   "role": "identity",    "channel": "text + selection-shape", "air": "air"},
    "posture":    {"gloss": "what state it's in",  "question": "what condition is it in?",
                   "role": "ordinal",     "channel": "criticality-hue",        "air": "air"},
    "action":     {"gloss": "what it's doing/next","question": "where is it in its lifecycle?",
                   "role": "categorical", "channel": "text / view-axis",       "air": "air"},
    "ownership":  {"gloss": "whose it is",         "question": "who owns/acts on it?",
                   "role": "categorical", "channel": "ownership family-hue",   "air": "air"},
    "place":      {"gloss": "where it sits",       "question": "where does it live?",
                   "role": "categorical", "channel": "family-hue / view-axis", "air": "air"},
    "time":       {"gloss": "how fresh",           "question": "when / how recent?",
                   "role": "temporal",    "channel": "freshness-dim",          "air": "air"},
    "provenance": {"gloss": "how we know / may it air", "question": "what is its evidence/authority/egress class?",
                   "role": "ordinal-confidence", "channel": "secondary pip + AIR-class", "air": "gate"},
    "measure":    {"gloss": "how much",            "question": "what scalar quantity?",
                   "role": "quantitative","channel": "eighth-block bar",       "air": "air"},
    "qualifier":  {"gloss": "which variant",       "question": "which capability/mode variant?",
                   "role": "categorical-subfacet","channel": "text / view-axis","air": "air"},
}

CITATION_ORDER = ["identity", "ownership", "place", "action", "posture",
                  "qualifier", "measure", "time", "provenance"]

# ── THE TWO LAYERS KEPT OUTSIDE THE FACET SET ────────────────────────────────
# EDGES — a relation is a property of a PAIR (an entity-REFERENCE pointing at another unit), not a
# scalar of one unit. The graph/relation layer (framework §3 / §5b). NOT a facet.
EDGES: set[str] = {
    "claimed_task", "parent_spec", "route", "route_id", "route_evidence_ref",
    "source", "target", "relation", "map_source", "map_target", "map_relation",
    "focus_node_ids", "focus_edge_ids", "selection_id", "selection_group", "adapter_id",
    "task_link_state", "evidence_ref", "subject_ref", "windows", "surfaces", "commands",
    "source_refs", "source_ref_labels",
}
# BODIES — free-text payload; default-DENY on air (never a facet).
BODIES: set[str] = {
    "summary", "detail", "prompt", "takeaway", "caveat", "emphasis", "context", "thesis",
    "missing", "action", "next_evidence", "blocked_reasons", "hardening_notes", "must_include",
    "answer_shape", "blockers", "docs", "aliases", "tags",
}
# ENVELOPES — structural wrappers / nesting / mirrors; not binnable row attributes.
ENVELOPES: set[str] = {
    "air", "schema_version", "generated_at", "package_hash", "sources", "rows", "relations",
    "routes", "tools", "validation", "lenses", "claims", "observations", "totals",
    "workbench_contract", "layers", "nodes", "edges", "package", "health", "tmux", "task",
    "evidence_refs", "evidence_summary", "resume", "defaults", "inquiry_modes", "audience_modes",
    "explanation_paths", "follow_on_tranches", "scenes", "by_kind", "scene_count",
    "transcript_roots_observed", "transcript_roots_missing",
    "lifecycle_sources", "lifecycles", "lifecycle_totals", "lifecycle_authority",
    "lifecycle_generated_at", "lifecycle_package_hash", "lifecycle_default_lens",
}

# ── BY-NAME DEFAULT ASSIGNMENT (the unambiguous bulk) ────────────────────────
FACET_BY_NAME: dict[str, str] = {
    # identity (id-family + type/kind/label + subject)
    "id": "identity", "task_id": "identity", "trace_id": "identity", "gate_id": "identity",
    "domain_id": "identity", "lifecycle_id": "identity", "row_id": "identity",
    "capability_id": "identity", "tool_id": "identity", "map_id": "identity",
    "label": "identity", "kind": "identity", "subject_kind": "identity", "map_kind": "identity",
    "family": "identity", "subject": "identity", "title": "identity",
    # posture (current condition / health / admission)
    "state": "posture", "no_go": "posture", "readiness": "posture", "criticality": "posture",
    "severity": "posture", "blocker": "posture", "route_state": "posture", "quota_state": "posture",
    "route_binding_state": "posture", "alive": "posture", "idle": "posture", "stalled": "posture",
    "exists": "posture", "attached": "posture", "available": "posture", "ready": "posture",
    "resume_ready": "posture", "truncated": "posture", "freshness_ok": "posture",
    "parity": "posture", "posture": "posture", "maturity": "posture", "coverage": "posture",
    "status": "posture",  # DEFAULT; overridden -> provenance on graph/epistemic domains
    # action (lifecycle motion)
    "stage": "action", "prior_stage": "action", "predicted_stage": "action", "intent": "action",
    # ownership (the "whose" labels)
    "owner": "ownership", "role": "ownership", "actor": "ownership", "assigned_to": "ownership",
    # place (locus)
    "platform": "place", "layer": "place", "domain": "place", "scope": "place", "depth": "place",
    "plant": "place", "capacity_pool": "place", "terrain": "place", "session": "place", "path": "place",
    # posture (gate result is a state)
    "gate": "posture",
    # time
    "ts": "time", "freshness": "time", "mtime": "time", "updated_at": "time", "observed_at": "time",
    "stale_after": "time", "generated_at": "time", "output_age_s": "time", "relay_age_s": "time",
    "activity_age_s": "time", "age_bucket": "time",
    # provenance (evidence / authority / egress / permission)
    "authority": "provenance", "authority_case": "provenance", "authority_ceiling": "provenance",
    "claim_ceiling": "provenance", "claim_surface": "provenance", "mutation_surface": "provenance",
    "hkp_posture": "provenance", "privacy": "provenance", "raw_access": "provenance",
    "air_class": "provenance", "confidence": "provenance", "evidence": "provenance",
    "dark_policy": "provenance", "freshness_policy": "provenance", "egress_class": "provenance",
    "receipt_requirement": "provenance", "receipt_contracts": "provenance", "authority_use": "provenance",
    # measure (scalar quantities)
    "score": "measure", "magnitude": "measure", "cost": "measure", "latency_ms": "measure",
    "prompt_tok": "measure", "completion_tok": "measure", "total_tok": "measure", "size": "measure",
    "attention": "measure", "route_count": "measure", "ok_count": "measure", "blocked_count": "measure",
    "evidence_count": "measure", "rel_count": "measure", "receipt_count": "measure", "count": "measure",
    "total": "measure", "depth_count": "measure",
    # qualifier (capability/mode variant — the sub-faceted dimensional cross-product)
    "effort": "qualifier", "context_mode": "qualifier", "fast_mode": "qualifier",
    "quantization": "qualifier", "demand_vector": "qualifier", "hardening": "qualifier",
    "eval_plane": "qualifier", "review_obligation": "qualifier", "learning_eligibility": "qualifier",
    "benchmark_coverage": "qualifier", "fixed_overhead": "qualifier", "spend_model": "qualifier",
    "capability_class": "qualifier", "surface_family": "qualifier", "model": "qualifier",
    "model_id": "qualifier", "mode": "qualifier", "profile": "qualifier", "res": "qualifier",
    "lens": "qualifier", "inquiry_mode": "qualifier", "audience_mode": "qualifier",
    "explanation_path": "qualifier", "default_lens": "qualifier", "meta": "qualifier",
}

# ── PER-(domain, attr) OVERRIDES (the field-level homonyms) ───────────────────
# `status` is the provenance confidence-ladder (asserted/observed/inferred/...) on the graph +
# epistemic domains, but a posture state elsewhere. `lifecycle` is a tenant ref (edge) on DomainRow.
OVERRIDES: dict[tuple[str, str], str] = {
    ("Node", "status"): "provenance",
    ("Edge", "status"): "provenance",
    ("EpistemicReadRow", "status"): "provenance",
    ("EpistemicReadRow", "posture"): "provenance",
    ("DomainRow", "lifecycle"): "__edge__",   # ref to a Lifecycle tenant
}


def classify(domain: str, attr: str) -> str:
    """Resolve one (domain, attr) to: a facet key, or 'edge' / 'body' / 'envelope'.
    Returns 'UNRESOLVED:<attr>' if neither the overrides, the layer sets, nor the by-name map cover
    it — the contract test fails on any UNRESOLVED, which is how the registry stays exhaustive."""
    ov = OVERRIDES.get((domain, attr))
    if ov == "__edge__":
        return "edge"
    if ov:
        return ov
    if attr in EDGES:
        return "edge"
    if attr in BODIES:
        return "body"
    if attr in ENVELOPES:
        return "envelope"
    f = FACET_BY_NAME.get(attr)
    if f:
        return f
    return f"UNRESOLVED:{attr}"


# Per-attribute AIR overrides: facet says how a KIND of fact airs, but specific attributes carry
# PII regardless of facet (filesystem paths, tmux session names) and DENY on air. (framework: AIR is
# per-attribute, not only per-facet.)
SENSITIVE: set[str] = {"path", "session"}


def air_policy(domain: str, attr: str) -> str:
    """The full per-attribute AIR decision: SENSITIVE attrs deny regardless of facet; otherwise the
    per-facet default. This is what a renderer/the read API consults to build the on-air allowlist."""
    if attr in SENSITIVE:
        return "deny"
    return air_default(classify(domain, attr))


def air_default(resolution: str) -> str:
    """Per-facet AIR default (bimodal-AIR by construction): skeleton facets AIR; Provenance GATES;
    Measure airs (scalars are operational); bodies DENY; edges air their existence (the value is a
    ref id, gated by the endpoints' own AIR); envelopes n/a."""
    if resolution == "body":
        return "deny"
    if resolution == "provenance":
        return "gate"
    if resolution in FACETS:
        return "air"
    if resolution == "edge":
        return "air"   # the edge's existence airs; endpoint values gate by their own facet
    return "n/a"        # envelope


def legible_key(facet: str) -> str:
    """The cold-read decoder key for a facet (A6 — ships in-band with every view)."""
    return FACETS.get(facet, {}).get("gloss", facet)
