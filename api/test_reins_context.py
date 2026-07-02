"""Tests for the tri-audience projection engine — AIR-seal-per-audience-before-derivation + never-injector."""
import reins_context as rc

# a contract-shaped fixture (reins-text-context-producer-contract). The session fact is operator-private
# (denied to public); the classification is public-redacted + provider-prompt-eligible.
FIXTURE = {
    "schema_version": 1,
    "evaluation": {"bundle_state": "partial"},
    "facts": {
        "session_extractions": [{
            "fact_id": "fx-s1", "subject_ref": "session:x",
            "freshness_state": "fresh", "confidence_word": "high",
            "air": {"operator_private": "allow", "yard_context": "redact",
                    "hapax_substrate": "redact", "public_or_air": "deny"},
            "extracted": {"active_task_ref": "cc-task-secret"},
            "affordance_inputs": {"can_explain": True, "can_yank_operator_private": True},
        }],
        "text_classifications": [{
            "fact_id": "fx-c1", "subject_ref": "chunk:y", "text_domain": ["design"],
            "freshness_state": "fresh", "confidence_word": "medium",
            "air": {"operator_private": "allow", "yard_context": "allow",
                    "hapax_substrate": "allow", "public_or_air": "redact"},
            "labels": {"primary": "producer_contract"},
            "affordance_inputs": {"can_explain": True, "can_enter_provider_prompt": True, "requires_hold": True},
        }],
    },
}


def test_tri_audience_seal_denied_never_derives():
    op = rc.project(FIXTURE, "operator_private")
    pub = rc.project(FIXTURE, "public_or_air")
    assert op["fact_count"] == 2                      # operator sees both
    subs = {f["subject_ref"] for f in pub["facts"]}
    assert "session:x" not in subs                    # public-DENIED fact never enters public's derivation
    assert "chunk:y" in subs                          # public-redacted classification survives structurally
    assert pub["fact_count"] == 1                      # the count is taken AFTER sealing (no leak-by-count)


def test_redact_seals_body_keeps_envelope():
    yard = rc.project(FIXTURE, "yard_context")
    sess = next(f for f in yard["facts"] if f["subject_ref"] == "session:x")
    assert sess["_air_redacted"] is True
    assert sess["extracted"] == rc.REDACTION_TOKEN    # body redacted…
    assert sess["fact_id"] == "fx-s1"                 # …envelope intact


def test_affordance_why_from_classification():
    op = rc.project(FIXTURE, "operator_private")
    aff = {a["subject_ref"]: {e["affordance_kind"]: e["state"] for e in a["affordances"]}
           for a in op["affordances"]}
    assert aff["chunk:y"]["explain_why"] == "present"
    assert aff["chunk:y"]["refocus"] == "present"
    assert aff["chunk:y"]["stage_injection_preview"] == "hold"   # never-injector: provider-eligible -> HOLD
    assert aff["session:x"].get("yank_operator_private") == "present"  # operator + un-redacted body


def test_yank_denied_outside_operator_audience():
    yard = rc.project(FIXTURE, "yard_context")
    aff = {a["subject_ref"]: {e["affordance_kind"]: e["state"] for e in a["affordances"]}
           for a in yard["affordances"]}
    assert "yank_operator_private" not in aff.get("session:x", {})  # redacted body + non-operator -> no yank


def test_default_deny_on_absent_air():
    bundle = {"facts": {"salience_facts": [{"fact_id": "fx-x", "subject_ref": "s", "air": {}}]}}
    assert rc.project(bundle, "yard_context")["fact_count"] == 0    # absent decision defaults to DENY


def test_project_all_is_tri_audience():
    allp = rc.project_all(FIXTURE)
    assert set(allp) == set(rc.AUDIENCES)


def test_readout_only_never_injector():
    # the module exposes NO egress/inject path — action routes through the governed apply seam elsewhere.
    for banned in ("send", "dispatch", "inject", "publish", "spawn", "spend", "provider_call"):
        assert not hasattr(rc, banned), f"reins_context must expose no {banned} path (never-injector)"
