from reins_read import score_event, classify_air, to_event, to_task


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


def test_to_task_shape_and_air():
    t = {"task_id": "x-1", "stage": "S6", "authority_case": "CASE-1", "no_go": {"blocked": True, "ok": False}}
    out = to_task("x-1", t, allowlist=["task_id", "stage", "no_go"])
    assert out["task_id"] == "x-1" and out["stage"] == "S6" and out["no_go"] == "blocked"
    assert out["air"]["task_id"] == "ok" and out["air"]["authority_case"] == "deny"
