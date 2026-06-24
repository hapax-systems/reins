from reins_read import score_event, classify_air, to_event


def test_score_recency_and_kind():
    a = score_event({"kind": "pr.merged"}, age_s=5)
    b = score_event({"kind": "review.fail"}, age_s=5)  # escalation outranks
    assert 0.0 <= a <= 1.0 and b > a


def test_air_is_default_deny():
    air = classify_air({"subject": "4284", "summary": "secret name"}, allowlist=["subject"])
    assert air["subject"] == "ok" and air["summary"] == "deny"


def test_to_event_shape():
    raw = {"type": "coord.pr.merged", "subject": "4284", "actor": "alpha",
           "payload": {"summary": "PR#4284 merged"}, "ts": "2026-06-24T14:22:00Z"}
    ev = to_event(raw, allowlist=["kind", "subject"], age_s=2)
    assert ev["kind"] == "pr.merged" and ev["subject"] == "4284"
    assert ev["air"]["actor"] == "deny" and ev["air"]["subject"] == "ok"
