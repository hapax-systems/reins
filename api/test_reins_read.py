from reins_read import score_event, classify_air, to_event, to_task, to_node, to_edge, instance_config


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


def test_to_node_shape_and_air():
    n = {"id": "rdf-owl-kg", "label": "RDF / OWL KG", "kind": "backbone",
         "layer": "semantic-backbone", "status": "asserted", "resolution": 1}
    out = to_node(n, allowlist=["id", "label", "layer", "status"])
    assert out["id"] == "rdf-owl-kg" and out["layer"] == "semantic-backbone" and out["status"] == "asserted"
    assert out["air"]["id"] == "ok" and out["air"]["kind"] == "deny"  # kind not allowlisted -> deny


def test_to_edge_shape_and_air():
    e = {"source": "dmn", "target": "drd", "relation": "defines", "status": "asserted", "confidence": 1.0}
    out = to_edge(e, allowlist=["source", "target", "relation"])
    assert out["source"] == "dmn" and out["target"] == "drd" and out["relation"] == "defines"
    assert out["air"]["relation"] == "ok" and out["air"]["status"] == "deny"


def test_instance_config_neutral_defaults_no_baked_path(monkeypatch):
    monkeypatch.setenv("REINS_CONFIG", "/no/such/reins.toml")  # force the no-file path
    for k in ("REINS_COUNCIL_ROOT", "REINS_AIR_ALLOWLIST", "REINS_PORT"):
        monkeypatch.delenv(k, raising=False)
    cfg = instance_config()
    assert cfg["council_root"] == ""  # NO baked operator path
    assert cfg["port"] == 8799
    assert "kind" in cfg["allowlist"] and "subject" not in cfg["allowlist"]  # conservative on-air default


def test_instance_config_env_overrides(monkeypatch):
    monkeypatch.setenv("REINS_CONFIG", "/no/such/reins.toml")
    monkeypatch.setenv("REINS_COUNCIL_ROOT", "/env/root")
    monkeypatch.setenv("REINS_AIR_ALLOWLIST", "kind,subject")
    cfg = instance_config()
    assert cfg["council_root"] == "/env/root"
    assert cfg["allowlist"] == ["kind", "subject"]
