"""Synthetic custody sources; no lifecycle tool or estate configuration is used."""

import copy
import json
import os
import sys
from contextlib import contextmanager

import pytest


def fixture(tmp_path):
    rows = tmp_path / "rows"
    rows.mkdir()
    marker = tmp_path / "claim"
    marker.write_text("work-a\n")
    note = rows / "work-a.md"
    note.write_text(
        "---\ntask_id: work-a\nstatus: claimed\nassigned_to: lane-a\n"
        "branch: feature/a\npr: null\nrelated: [work-b]\n---\n"
        "## Session log\n"
        "- 2026-10-13T01:00:00Z lane-a claimed (cc-claim, session=private-value)\n"
        "- 2026-10-13T01:45:32Z lane-a: return refused exit8 claim_return_started. PRIVATE-BODY\n"
    )
    return (
        note,
        marker,
        {
            "rows_dirs": [str(rows)],
            "task_ids": ["work-a"],
            "claim_markers": [{"role": "lane-a", "path": str(marker)}],
        },
    )


@contextmanager
def no_effects():
    """Intercept effects at Python's syscall audit boundary, including short-lived writes."""
    active = True

    def audit(event, args):
        if not active:
            return
        if event == "open":
            _, mode, flags = args
            assert not (
                flags
                & (os.O_WRONLY | os.O_RDWR | os.O_CREAT | os.O_TRUNC | os.O_APPEND)
            ), "write attempted"
            assert not (mode and any(c in mode for c in "wax+")), "write attempted"
        assert event not in {
            "subprocess.Popen",
            "os.system",
            "os.exec",
            "os.posix_spawn",
            "os.remove",
            "os.rename",
            "os.mkdir",
            "os.rmdir",
            "os.chmod",
            "os.utime",
        }, f"effect attempted: {event}"

    sys.addaudithook(audit)
    try:
        yield
    finally:
        active = False


def test_no_write_or_process_and_only_metadata_egress(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, marker, spec = fixture(tmp_path)
    projector = CustodyProjection(spec)
    before = [
        (p.read_bytes(), p.stat().st_mtime_ns, p.stat().st_ino) for p in (note, marker)
    ]
    with no_effects():
        data = projector.read(now=100)
        again = projector.read(now=101)
    assert data["state"] == "ok"
    assert len(data["transitions"]) == 1
    assert again["transitions"] == data["transitions"]
    assert before == [
        (p.read_bytes(), p.stat().st_mtime_ns, p.stat().st_ino) for p in (note, marker)
    ]
    assert "PRIVATE-BODY" not in json.dumps(data)
    assert "private-value" not in json.dumps(data)
    assert "feature/a" not in json.dumps(data)


def test_missing_rows_source_is_typed_and_not_empty_success(tmp_path):
    from reins_claim_custody import CustodyProjection

    data = CustodyProjection(
        {"rows_dirs": [str(tmp_path / "missing")], "task_ids": ["work-a"]}
    ).read()
    assert data["state"] == "rows-source-unavailable"
    assert data["rows"] == []
    assert data["transitions"] == []


def test_started_inference_and_recorded_refusal_are_distinct(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, _, spec = fixture(tmp_path)
    row = CustodyProjection(spec).read()["rows"][0]
    assert row["started"] is True
    assert row["return_state"] == "started-work-hold"
    assert row["claim_state"] == "present"
    refused = row["recorded_events"][-1]
    assert refused["kind"] == "return-refused"
    assert refused["guard"] == "claim_return_started"
    assert refused["confidence"] == "reported"
    assert "cc-close" in refused["remedy"]
    assert refused["ts"] == "2026-10-13T01:45:32Z"
    note.write_text(note.read_text().split("## Session log")[0])
    row = CustodyProjection(spec).read()["rows"][0]
    assert row["started"] is True
    assert row["recorded_events"] == []


def test_supersession_relations_and_claim_history(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, marker, spec = fixture(tmp_path)
    note.write_text(
        note.read_text().replace("status: claimed", "status: superseded")
        + "- 2026-10-13T02:12:11Z lane-a closed as superseded: demand preserved in work-b under disposition. (cc-close)\n"
    )
    (note.parent / "work-b.md").write_text(
        "---\ntask_id: work-b\nstatus: offered\nassigned_to: unassigned\ndepends_on: [work-c]\n---\n"
    )
    (note.parent / "work-c.md").write_text(
        "---\ntask_id: work-c\nstatus: claimed\nassigned_to: lane-a\nbranch: child\n---\n## Session log\n- 2026-10-13T02:12:39Z lane-a claimed (cc-claim, session=private)\n"
    )
    spec["task_ids"] += ["work-b", "work-c"]
    marker.write_text("work-c\n")
    data = CustodyProjection(spec).read()
    rows = {r["task_id"]: r for r in data["rows"]}
    assert rows["work-a"]["return_state"] == "not-returnable-status"
    assert rows["work-a"]["recorded_events"][-1]["target"] == "work-b"
    assert [(r["relation"], r["target"]) for r in rows["work-b"]["relations"]] == [
        ("depends_on", "work-c")
    ]
    assert rows["work-c"]["claim_state"] == "present"
    assert rows["work-c"]["recorded_events"][0]["kind"] == "claimed"


def test_bounded_flood_immutable_history_and_no_phantom_changes(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, _, spec = fixture(tmp_path)
    projector = CustodyProjection(spec, capacity=3)
    first = projector.read(now=100)
    saved = copy.deepcopy(first)
    for i in range(20):
        note.write_text(
            note.read_text().replace(f"lane-{i - 1}" if i else "lane-a", f"lane-{i}")
        )
        result = projector.read(now=101 + i)
    assert len(result["transitions"]) == 3
    assert result["dropped"] == 18
    assert [e["sequence"] for e in result["transitions"]] == [19, 20, 21]
    assert result["transitions"][-1]["prior"]["assigned_to"] == "lane-18"
    assert result["transitions"][-1]["current"]["assigned_to"] == "lane-19"
    assert first == saved
    result["transitions"][-1]["current"]["assigned_to"] = "caller-tamper"
    assert (
        projector.read(now=200)["transitions"][-1]["current"]["assigned_to"]
        == "lane-19"
    )


@pytest.mark.parametrize(
    "body",
    [
        "---\ntask_id: work-a\nstatus: claimed\nstatus: offered\n---\n",
        "---\ntask_id: work-a\nstatus: [claimed]\n---\n",
        "---\ntask_id: wrong-id\nstatus: claimed\n---\n",
        "---\ntask_id: work-a\nstatus: claimed\n",
    ],
)
def test_bad_rows_do_not_mint_custody(tmp_path, body):
    from reins_claim_custody import CustodyProjection

    note, _, spec = fixture(tmp_path)
    note.write_text(body)
    row = CustodyProjection(spec).read()["rows"][0]
    assert row["state"] == "row-unavailable"
    assert "started" not in row


def test_unavailable_marker_is_not_absent_and_symlink_row_is_not_read(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, marker, spec = fixture(tmp_path)
    marker.unlink()
    marker.mkdir()
    assert (
        CustodyProjection(spec).read()["rows"][0]["claim_state"]
        == "claim-source-unavailable"
    )
    outside = tmp_path / "private"
    note.rename(outside)
    note.symlink_to(outside)
    assert CustodyProjection(spec).read()["rows"][0]["state"] == "row-unavailable"


def test_outage_keeps_history_and_recovery_does_not_invent_transition(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, _, spec = fixture(tmp_path)
    projector = CustodyProjection(spec)
    first = projector.read()
    note.parent.rename(tmp_path / "away")
    outage = projector.read()
    assert outage["state"] == "rows-source-unavailable"
    assert outage["transitions"] == first["transitions"]
    (tmp_path / "away").rename(note.parent)
    assert projector.read()["transitions"] == first["transitions"]


def test_endpoint_uses_one_process_local_projector(tmp_path):
    from fastapi.testclient import TestClient
    from reins_read import build_app

    note, _, spec = fixture(tmp_path)
    client = TestClient(build_app("", [], {"claim_custody": spec}))
    first = client.get("/read/claim-custody").json()
    assert first["state"] == "ok"
    note.write_text(note.read_text().replace("status: claimed", "status: superseded"))
    second = client.get("/read/claim-custody").json()
    assert len(second["transitions"]) == 2
    assert second["history_scope"] == "process-observations"


def test_unconfigured_is_typed_and_never_guesses_paths():
    from reins_claim_custody import CustodyProjection

    with no_effects():
        assert CustodyProjection(None).read()["state"] == "custody-binding-unavailable"


def test_bounds_reject_large_sources_and_cap_reported_events(tmp_path):
    from reins_claim_custody import MAX_BYTES, MAX_EVENTS, MAX_ROWS, CustodyProjection

    note, _, spec = fixture(tmp_path)
    body = note.read_text()
    note.write_text(body + "\n" + "x" * MAX_BYTES)
    assert CustodyProjection(spec).read()["rows"][0]["state"] == "row-unavailable"
    note.write_text(
        body
        + "".join(
            f"- 2026-10-13T03:{i:02}:00Z lane-a claimed (cc-claim, session=private)\n"
            for i in range(50)
        )
    )
    row = CustodyProjection(spec).read()["rows"][0]
    assert len(row["recorded_events"]) == MAX_EVENTS
    assert row["events_truncated"] is True
    spec["task_ids"] = [f"work-{i}" for i in range(MAX_ROWS + 1)]
    assert CustodyProjection(spec).read()["state"] == "custody-binding-unavailable"
    with pytest.raises(ValueError):
        CustodyProjection(None, capacity=1000)


def test_yaml_aliases_and_ambiguous_roots_are_unavailable(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, _, spec = fixture(tmp_path)
    original = note.read_text()
    note.write_text(original.replace("status: claimed", "status: &s claimed\ncopy: *s"))
    assert CustodyProjection(spec).read()["rows"][0]["state"] == "row-unavailable"
    note.write_text(original)
    spec["rows_dirs"] *= 2
    assert CustodyProjection(spec).read()["rows"][0]["state"] == "row-unavailable"


def test_missing_marker_and_conflicting_holder_are_distinct(tmp_path):
    from reins_claim_custody import CustodyProjection

    _, marker, spec = fixture(tmp_path)
    marker.unlink()
    assert CustodyProjection(spec).read()["rows"][0]["claim_state"] == "absent"
    marker.write_text("work-a\n")
    spec["claim_markers"][0]["role"] = "lane-b"
    assert CustodyProjection(spec).read()["rows"][0]["claim_state"] == "conflict"
    spec.pop("claim_markers")
    assert CustodyProjection(spec).read()["rows"][0]["claim_state"] == "unconfigured"


def test_unstarted_is_candidate_only_and_null_spellings_are_respected(tmp_path):
    from reins_claim_custody import CustodyProjection

    note, _, spec = fixture(tmp_path)
    note.write_text(note.read_text().replace("feature/a", "nil"))
    row = CustodyProjection(spec).read()["rows"][0]
    assert row["started"] is False
    assert row["guard"] == ""
    assert row["return_state"] == "unstarted-candidate"
    note.write_text(note.read_text().replace("pr: null", "pr: 7"))
    assert CustodyProjection(spec).read()["rows"][0]["started"] is True


def test_registry_covers_payload_and_air_never_exposes_identity(tmp_path):
    import facet_registry as fr
    from reins_claim_custody import CustodyProjection

    _, _, spec = fixture(tmp_path)
    data = CustodyProjection(spec).read(
        allowlist=list(fr.FACET_BY_NAME) + list(fr.ENVELOPES) + list(fr.BODIES)
    )

    def check(value):
        if isinstance(value, list):
            for item in value:
                check(item)
        elif isinstance(value, dict):
            for key, item in value.items():
                assert not fr.classify("ClaimCustody", key).startswith("UNRESOLVED"), (
                    key
                )
                if key in {
                    "task_id",
                    "assigned_to",
                    "actor",
                    "remedy",
                    "holders",
                    "recorded_events",
                }:
                    assert value["air"][key] == "deny"
                if key != "air":
                    check(item)

    check(data)
    assert data["rows"][0]["air"]["started"] == "ok"


def test_instance_config_accepts_binding_without_default(tmp_path, monkeypatch):
    from reins_read import instance_config

    config = tmp_path / "instance.toml"
    config.write_text(
        '[claim_custody]\nrows_dirs=["/example/rows"]\ntask_ids=["work-a"]\n'
    )
    monkeypatch.setenv("REINS_CONFIG", str(config))
    assert instance_config()["claim_custody"]["task_ids"] == ["work-a"]
    config.write_text("")
    assert instance_config()["claim_custody"] is None
