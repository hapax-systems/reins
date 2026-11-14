"""Offline gate observations: counts are evidence, never release authority."""
import json
import socket
import subprocess
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

import reins_read as rr


ENV = {
    "cc_task_gate": "REINS_CC_TASK_GATE_LEDGER",
    "release_auto_arm": "REINS_AUTHORITY_CASE_LEDGER",
    "policy_shadow": "REINS_POLICY_DECIDE_SHADOW_LEDGER",
    "branch_ruleset": "REINS_BRANCH_RULESET_SNAPSHOT",
}
CFG = {
    "cc_task_gate": "cc_task_gate_ledger",
    "release_auto_arm": "authority_case_ledger",
    "policy_shadow": "policy_decide_shadow_ledger",
}


def write(path, records):
    path.write_text("\n".join(json.dumps(r) for r in records) + "\n")


def release(armed=True):
    return {
        "kind": "release_auto_arm",
        "release_auto_arm_assessment": {
            "armed": armed, "eligible": armed, "needs_arming": not armed,
            "blockers": [] if armed else ["private blocker"], "subject": "private subject",
        },
        "release_auto_arm_result": {"armed": armed, "armed_at": "private time", "note_mutated": True},
        "verified_checks": ["private check"] * 26,
        "verified_checks_head_sha": "private head",
    }


@pytest.fixture
def offline(monkeypatch, tmp_path):
    paths = {key: tmp_path / (key + ".jsonl") for key in ENV}
    for key, path in paths.items():
        monkeypatch.setenv(ENV[key], str(path))

    def absent(*args, **kwargs):
        raise OSError("private source location")

    def forbidden(*args, **kwargs):
        pytest.fail("gate observations must not launch commands or contact a service")

    for name in ("_projection", "_session_snapshot", "_raw_tail"):
        monkeypatch.setattr(rr, name, absent)
    monkeypatch.setattr(subprocess, "Popen", forbidden)
    monkeypatch.setattr(socket, "create_connection", forbidden)
    return paths


def summary():
    return rr.read_gate_summary("unused", [])


def source(result, key):
    return next(s for s in result["sources"] if s["id"] == key)


def assert_dark(result, key):
    s = source(result, key)
    assert (s["status"], s["count"], s["age_bucket"], s["path"], s["detail"]) == (
        "dark", 0, "dark", "", "ledger absent or unreadable",
    )
    assert not [r for r in result["rows"] if r["source"] == key]


def test_all_ledgers_offline_counts_and_no_private_values(offline):
    # Synthetic home/vault/command/UUID values; never a real estate record.
    home = str(Path("/home") / "synthetic-fixture")
    forbidden = [home, home + "/Documents/Private", "run private-command --body private-text",
                 "10000000-0000-4000-8000-000000000001", "private-task", "private-authority"]
    private = dict(zip(("file_path", "parent_spec", "command", "session_id", "task_id", "authority_case"), forbidden))
    private.update(note=forbidden[1], mutation_scope_refs=forbidden, pr=987654)
    write(offline["cc_task_gate"], [dict(private, legacy_exit=code, stage=stage) for code, stage in
          [(0, "S6_IMPLEMENTATION"), (0, "S6"), (2, "S7_RELEASE"), (2, forbidden[0])]])
    write(offline["release_auto_arm"], [dict(private, **release()), dict(private, **release(False))])
    write(offline["policy_shadow"], [dict(private, diverged=True, new_verdict=v) for v in ("allow", "block", "block")])
    result = summary()
    expected = {
        "cc_task_gate": (4, "allow=2; refuse=2; allow_stages=S6:2; refuse_stages=S7:1 unknown:1"),
        "release_auto_arm": (2, "armed=1; blocked=1; recorded_checks=52; historical observations, not quorum"),
        "policy_shadow": (3, "allow=1; block=2; recorded divergences only"),
    }
    for key, (count, detail) in expected.items():
        s = source(result, key)
        assert (s["status"], s["count"], s["detail"], s["path"]) == ("observed", count, detail, "")
        row, = [r for r in result["rows"] if r["source"] == key]
        assert (row["state"], row["action"], row["evidence"]) == ("observed", "observe", detail)
    assert_dark(result, "task_projection")
    assert_dark(result, "session_state")
    assert_dark(result, "event_log")
    for token in forbidden + ["987654", "private source location", "private blocker", "private check", "private subject", "private head"]:
        assert token not in json.dumps(result)
    assert result["totals"]["blocked"] == 0
    assert result["totals"]["preview"] == 4


@pytest.mark.parametrize("key", ENV)
@pytest.mark.parametrize("content", [None, "", "not json\n", "{}\n", "[]\n", '"text"\n'])
def test_unobserved_inputs_stay_dark(offline, key, content):
    if content is not None:
        offline[key].write_text(content)
    assert_dark(summary(), key)


@pytest.mark.parametrize("key", ENV)
def test_unreadable_source_is_dark_without_its_exception(offline, monkeypatch, key):
    original = Path.read_text
    offline[key].touch()

    def unreadable(path, *args, **kwargs):
        if path == offline[key]:
            raise OSError("private failure detail")
        return original(path, *args, **kwargs)

    monkeypatch.setattr(Path, "read_text", unreadable)
    result = summary()
    assert_dark(result, key)
    assert "private failure detail" not in json.dumps(result)


@pytest.mark.parametrize("record", [
    {"legacy_exit": True, "stage": "S6"}, {"legacy_exit": "0", "stage": "S6"},
    {"legacy_exit": False, "stage": "S6"},
    {"legacy_exit": 1, "stage": "S6"}, {"stage": "S6"},
])
def test_task_exit_requires_an_observed_integer(offline, record):
    write(offline["cc_task_gate"], [record])
    assert_dark(summary(), "cc_task_gate")


@pytest.mark.parametrize("stage, label", [("S3_5", "S3.5"), ("S11", "S11"), ("BLOCKED", "BLOCKED"),
                                          (None, "unknown"), ({"private": 1}, "unknown"), ("S99", "unknown")])
def test_stage_values_are_closed_labels(offline, stage, label):
    write(offline["cc_task_gate"], [{"legacy_exit": 0, "stage": stage}])
    assert source(summary(), "cc_task_gate")["detail"].endswith(f"allow_stages={label}:1; refuse_stages=")


@pytest.mark.parametrize("change", [
    {"kind": "other"}, {"release_auto_arm_assessment": None},
    {"release_auto_arm_assessment": {"armed": "true"}},
    {"release_auto_arm_result": {"armed": False}},
    {"verified_checks": "private checks"}, {"verified_checks": [7]},
    {"verified_checks_head_sha": None},
    {"release_auto_arm_assessment": {"armed": False, "eligible": True, "needs_arming": True, "blockers": []}},
])
def test_unclassified_release_records_are_not_armed_or_blocked(offline, change):
    write(offline["release_auto_arm"], [dict(release(), **change)])
    assert_dark(summary(), "release_auto_arm")


def test_bare_release_schema_never_substitutes_for_prefixed_fields(offline):
    row = release()
    row["assessment"] = row.pop("release_auto_arm_assessment")
    row["result"] = row.pop("release_auto_arm_result")
    write(offline["release_auto_arm"], [row])
    assert_dark(summary(), "release_auto_arm")


@pytest.mark.parametrize("record", [
    {"new_verdict": "allow"}, {"new_verdict": "allow", "diverged": False},
    {"new_verdict": "allow", "diverged": "true"}, {"new_verdict": "private", "diverged": True},
    {"new_verdict": [], "diverged": True},
])
def test_shadow_is_divergence_only_and_closed_verdicts(offline, record):
    write(offline["policy_shadow"], [record])
    assert_dark(summary(), "policy_shadow")


def test_ruleset_snapshot_counts_active_branch_rules_only(offline):
    active = {"target": "branch", "enforcement": "active", "name": "private branch",
              "rules": [{"type": t} for t in ("merge_queue", "pull_request", "required_status_checks", "non_fast_forward")]}
    offline["branch_ruleset"].write_text(json.dumps([
        active, dict(active, enforcement="disabled"), dict(active, enforcement="evaluate"), dict(active, target="tag"),
    ]))
    result = summary()
    s = source(result, "branch_ruleset")
    assert (s["status"], s["count"], s["detail"], s["path"]) == (
        "observed", 3, "merge=1; review=1; ci=1; active branch rule counts, snapshot only", "",
    )
    assert "private branch" not in json.dumps(result)


@pytest.mark.parametrize("snapshot", [
    {"target": "branch", "enforcement": "active"},
    {"target": "branch", "enforcement": "active", "rules": [7]},
    {"target": "branch", "enforcement": "active", "rules": [{"type": []}]},
    {"target": "branch", "enforcement": "active", "rules": [{"type": "future_rule"}]},
])
def test_schema_missing_snapshot_is_dark(offline, snapshot):
    offline["branch_ruleset"].write_text(json.dumps(snapshot))
    assert_dark(summary(), "branch_ruleset")


def test_snapshot_is_env_only_and_unproduced_gates_stay_dark(offline, monkeypatch):
    offline["branch_ruleset"].write_text(json.dumps({"target": "branch", "enforcement": "active",
                                                  "rules": [{"type": "pull_request"}]}))
    monkeypatch.delenv(ENV["branch_ruleset"])
    result = rr.read_gate_summary("unused", [], {"branch_ruleset_snapshot": str(offline["branch_ruleset"])})
    for key in ("branch_ruleset", "consent_egress", "allowlist_gate"):
        assert_dark(result, key)


@pytest.mark.parametrize("key", CFG)
def test_ledger_config_is_used_when_env_absent_and_env_wins(offline, monkeypatch, tmp_path, key):
    configured = tmp_path / "configured.jsonl"
    record = {"cc_task_gate": {"legacy_exit": 0, "stage": "S6"}, "release_auto_arm": release(),
              "policy_shadow": {"new_verdict": "block", "diverged": True}}[key]
    write(configured, [record])
    cfg = {CFG[key]: str(configured)}
    assert_dark(rr.read_gate_summary("unused", [], cfg), key)
    monkeypatch.delenv(ENV[key])
    assert source(rr.read_gate_summary("unused", [], cfg), key)["count"] == 1


def test_ledger_defaults_are_local_and_tail_counts_are_bounded(offline, monkeypatch, tmp_path):
    monkeypatch.setattr(Path, "home", lambda: tmp_path)
    cache = tmp_path / ".cache" / "hapax"
    cache.mkdir(parents=True)
    for key in CFG:
        monkeypatch.delenv(ENV[key])
    write(cache / "cc-task-gate-decisions.jsonl", [{"legacy_exit": 2, "stage": "S7"}] +
          [{"legacy_exit": 0, "stage": "S6"}] * 1000)
    write(cache / "authority-case-ledger.jsonl", [release()])
    write(cache / "policy-decide-shadow.jsonl", [{"diverged": True, "new_verdict": "block"}])
    result = summary()
    assert source(result, "cc_task_gate")["count"] == 1000
    assert source(result, "cc_task_gate")["detail"].startswith("allow=1000; refuse=0;")
    assert source(result, "release_auto_arm")["count"] == 1
    assert source(result, "policy_shadow")["count"] == 1


def test_only_the_existing_read_route_is_used(offline):
    app = rr.build_app("unused", [])
    paths = {r.path for r in app.routes}
    assert "/read/gates" in paths
    assert not any(p.startswith("/govern") for p in paths)
    response = TestClient(app).get("/read/gates")
    assert response.status_code == 200
    assert_dark(response.json()["gates"], "cc_task_gate")
