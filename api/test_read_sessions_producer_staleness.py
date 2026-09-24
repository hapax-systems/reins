"""A row from a stale producer never asserts alive.

On 2026-09-24 the coordinator that writes state.json had been dead for about eleven hours. Two
captures of /read/sessions, two minutes apart, said so in their own envelope — `producer.state:
"stale"`, `partial: true` — and in the same response returned 17 of 18 rows with `alive: true`.
The envelope knew; the rows never asked it. PR #22 added the envelope stanza and stopped there.

The fixtures are those two captures, reduced: lane, session and task identifiers replaced by
ordinals (this repository is public), every liveness-bearing field verbatim, the source capture
pinned by sha256 in `_provenance`. The replay rebuilds the producer's state.json from the captured
rows and ages its mtime to the captured producer age, so the endpoint under test sees what the
live one saw.
"""

import json
import os
import time
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

import reins_read
from reins_read import build_app

HERE = Path(__file__).parent
CAPTURES = sorted(HERE.glob("fixture_read_sessions_capture_*.json"))

# The producer's per-lane keys that the captured rows carry verbatim.
_LANE_KEYS = ("role", "session", "claimed_task", "alive", "idle", "stalled", "output_age_s", "relay_age_s")
# The derived row fields a replay must reproduce exactly when the producer is live.
_DERIVED = ("state", "alive", "idle", "stalled", "readiness", "blocker", "attention", "output_age_s", "relay_age_s")
_ALLOW = ["role", "state", "alive", "idle", "stalled", "readiness", "blocker", "attention"]


def _load(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def _replay(capture: dict, tmp_path: Path, monkeypatch, producer_age_s: float) -> TestClient:
    lanes = {row["role"]: {k: row[k] for k in _LANE_KEYS} for row in capture["sessions"]}
    state = tmp_path / "state.json"
    state.write_text(json.dumps({"lanes": lanes}), encoding="utf-8")
    then = time.time() - producer_age_s
    os.utime(state, (then, then))
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(state))
    for var in ("REINS_ORCHESTRATION_LEDGER_DIR", "REINS_SESSION_TRANSCRIPT_ROOTS", "REINS_VAULT_ROOT"):
        monkeypatch.delenv(var, raising=False)
    cfg = {
        "orchestration_ledger_dir": str(tmp_path / "no-ledger"),
        "session_transcript_roots": [],
        "vault_root": str(tmp_path / "no-vault"),
    }
    return TestClient(build_app(str(tmp_path), _ALLOW, cfg))


def test_both_captures_are_present():
    assert [p.name for p in CAPTURES] == [
        "fixture_read_sessions_capture_A_20260924T1859Z.json",
        "fixture_read_sessions_capture_B_20260924T1901Z.json",
    ]


@pytest.mark.parametrize("path", CAPTURES, ids=lambda p: p.name)
def test_the_captures_record_the_contradiction(path):
    """Pins what the fixtures are evidence of: a stale envelope over rows asserting alive."""
    capture = _load(path)
    assert capture["producer"]["state"] == "stale"
    assert capture["producer"]["age_s"] > reins_read._PRODUCER_STALE_S
    assert capture["partial"] is True
    assert sum(1 for row in capture["sessions"] if row["alive"]) == 17


@pytest.mark.parametrize("path", CAPTURES, ids=lambda p: p.name)
def test_a_live_replay_reproduces_every_captured_row(path, tmp_path, monkeypatch):
    """Fidelity of the replay, and the unchanged live path: with a fresh producer, every row comes
    back exactly as captured. Without this, a red below could be an artefact of the rebuild."""
    capture = _load(path)
    body = _replay(capture, tmp_path, monkeypatch, producer_age_s=0).get("/read/sessions").json()
    assert body["producer"]["state"] == "live"
    assert body.get("partial") is not True
    got = {row["role"]: row for row in body["sessions"]}
    assert set(got) == {row["role"] for row in capture["sessions"]}
    for want in capture["sessions"]:
        assert {k: got[want["role"]][k] for k in _DERIVED} == {k: want[k] for k in _DERIVED}, want["role"]


@pytest.mark.parametrize("path", CAPTURES, ids=lambda p: p.name)
def test_a_stale_producer_replay_asserts_no_row_alive(path, tmp_path, monkeypatch):
    capture = _load(path)
    body = _replay(capture, tmp_path, monkeypatch, capture["producer"]["age_s"]).get("/read/sessions").json()
    assert body["producer"]["state"] == "stale"
    assert body["partial"] is True
    alive = [row["role"] for row in body["sessions"] if row["alive"]]
    assert alive == [], f"{len(alive)} rows assert alive under a stale producer"


@pytest.mark.parametrize("path", CAPTURES, ids=lambda p: p.name)
def test_a_stale_producer_row_says_unknown_not_a_last_tick_verdict(path, tmp_path, monkeypatch):
    """alive=false alone would read as offline. The honest row says the producer cannot tell us:
    state and readiness unknown, the blocker names the producer, and no last-tick idle/stalled."""
    capture = _load(path)
    body = _replay(capture, tmp_path, monkeypatch, capture["producer"]["age_s"]).get("/read/sessions").json()
    assert len(body["sessions"]) == len(capture["sessions"])
    for row in body["sessions"]:
        assert (row["state"], row["readiness"], row["blocker"]) == ("unknown", "unknown", "stale_producer"), row["role"]
        # Unknown reads unknown: null, never a measured false or a last-tick age served as current.
        for field in ("alive", "idle", "stalled", "output_age_s", "relay_age_s"):
            assert row[field] is None, (row["role"], field, row[field])


def test_the_row_verdict_follows_the_envelope_stanza_not_a_second_clock(tmp_path, monkeypatch):
    """One freshness verdict per response. If the stanza says live, rows keep the producer's word;
    if it says stale, rows do not."""
    capture = _load(CAPTURES[0])
    client = _replay(capture, tmp_path, monkeypatch, producer_age_s=0)
    real_now = reins_read._now
    monkeypatch.setattr(reins_read, "_now", lambda: real_now() + reins_read._PRODUCER_STALE_S + 1)
    body = client.get("/read/sessions").json()
    assert body["producer"]["state"] == "stale"
    assert not any(row["alive"] for row in body["sessions"])


def test_an_unmeasurable_producer_age_is_stale():
    assert reins_read._producer_verdict(None) == "stale"


# ── threshold: derived from the producer's measured cadence, boundary pinned ─────────────────────
#
# The coordinator's loop sleeps max(1, tick_s - elapsed) with tick_s = 30 (code default and the
# deployed unit's HAPAX_COORDINATOR_TICK_S), so its write period is max(30s, tick duration). Measured
# from its journal over the run that ended 2026-09-24T08:00:52Z (765 consecutive in-run intervals
# between `tick:` lines): p50 47.3s, p99 97.8s, p99.9 128.6s, max 139.0s, none over 150s.
NOMINAL_TICK_S = 30.0
MEASURED_MAX_INTERVAL_S = 139.0


def test_the_stale_threshold_clears_the_measured_cadence_without_hiding_a_death():
    """At least 2x the slowest measured write interval, so a live producer never reads stale; at
    most 10 nominal ticks, so a dead one is named within five minutes."""
    assert 2 * MEASURED_MAX_INTERVAL_S <= reins_read._PRODUCER_STALE_S <= 10 * NOMINAL_TICK_S


def test_the_verdict_boundary_is_inclusive_at_the_threshold():
    t = reins_read._PRODUCER_STALE_S
    assert reins_read._producer_verdict(t) == "live"
    assert reins_read._producer_verdict(t + 0.001) == "stale"
    assert reins_read._producer_verdict(0.0) == "live"


@pytest.mark.parametrize(("offset", "want"), [(0.0, "live"), (0.001, "stale")])
def test_the_snapshot_boundary_is_measured_from_the_file_it_read(offset, want, tmp_path, monkeypatch):
    state = tmp_path / "state.json"
    state.write_text(json.dumps({"lanes": {"lane-01": {"alive": True}}}), encoding="utf-8")
    written = 1_800_000_000.0
    os.utime(state, (written, written))
    monkeypatch.setenv("REINS_COORDINATOR_STATE", str(state))
    monkeypatch.setattr(reins_read, "_now", lambda: written + reins_read._PRODUCER_STALE_S + offset)
    _raw, producer = reins_read._session_snapshot()
    assert producer["state"] == want


# ── fail-closed signature ─────────────────────────────────────────────────────────────────────────


def test_to_session_has_no_default_producer_verdict():
    """A caller that never consulted the producer must not get live semantics by omission."""
    with pytest.raises(TypeError):
        reins_read.to_session("lane-01", {"alive": True}, [])
    with pytest.raises(TypeError):
        reins_read.to_session_detail("lane-01", {"alive": True}, [])


# ── one snapshot: content and verdict from the same file generation ──────────────────────────────


def test_a_producer_write_between_read_and_stat_cannot_launder_stale_rows_as_live(tmp_path, monkeypatch):
    """The coordinator writes by atomic rename. If a fresh generation lands after the stale content
    was read, a stat of the path would call the stale rows live. The verdict must come from the
    file that was read."""
    capture = _load(CAPTURES[0])
    client = _replay(capture, tmp_path, monkeypatch, capture["producer"]["age_s"])
    state = Path(os.environ["REINS_COORDINATOR_STATE"])
    real_load = json.load
    swapped = []

    def load_then_swap(fh, *args, **kwargs):
        data = real_load(fh, *args, **kwargs)
        if not swapped and getattr(fh, "name", "") == str(state):
            fresh = state.with_suffix(".tmp")
            fresh.write_text(json.dumps({"lanes": {}}), encoding="utf-8")
            os.replace(fresh, state)
            swapped.append(True)
        return data

    monkeypatch.setattr(reins_read.json, "load", load_then_swap)
    body = client.get("/read/sessions").json()
    assert swapped, "the swap never ran — the test proves nothing"
    assert len(body["sessions"]) == len(capture["sessions"]), "rows must come from the generation read"
    assert body["producer"]["state"] == "stale"
    assert not any(row["alive"] for row in body["sessions"])


# ── the detail door ───────────────────────────────────────────────────────────────────────────────


def test_the_session_detail_door_reads_unknown_under_a_stale_producer(tmp_path, monkeypatch):
    """/read/session/{role} is the same row one click deeper; it must not contradict the list, and
    what the producer cannot say is null — not a measured absence of the tmux session."""
    capture = _load(CAPTURES[0])
    client = _replay(capture, tmp_path, monkeypatch, capture["producer"]["age_s"])
    body = client.get("/read/session/lane-01").json()
    assert body["dark"] is False
    detail = body["detail"]
    assert detail["state"] == "unknown"
    assert detail["blocker"] == "stale_producer"
    for field in ("alive", "idle", "stalled", "output_age_s", "relay_age_s"):
        assert detail["health"][field] is None, field
    for field in ("exists", "attached", "activity_age_s"):
        assert detail["tmux"][field] is None, field


def test_the_session_detail_door_keeps_the_live_verdict(tmp_path, monkeypatch):
    capture = _load(CAPTURES[0])
    body = _replay(capture, tmp_path, monkeypatch, producer_age_s=0).get("/read/session/lane-01").json()
    assert body["detail"]["health"]["alive"] is True
    assert body["detail"]["state"] == "active"
    assert body["detail"]["tmux"]["exists"] is True


# ── the neighbouring surfaces: gate summary and observe ──────────────────────────────────────────


def _lane_gate_rows(summary: dict) -> dict[str, dict]:
    return {row["gate_id"]: row for row in summary["rows"] if row["gate_id"].startswith("lane.blocker.")}


def test_the_gate_summary_lane_rows_follow_a_live_producer(tmp_path, monkeypatch):
    capture = _load(CAPTURES[0])
    _replay(capture, tmp_path, monkeypatch, producer_age_s=0)
    lanes = _lane_gate_rows(reins_read.read_gate_summary(str(tmp_path), _ALLOW, {"orchestration_ledger_dir": str(tmp_path / "no-ledger")}))
    want: dict[str, int] = {}
    for row in capture["sessions"]:
        if row["blocker"] != "none":
            want[row["blocker"]] = want.get(row["blocker"], 0) + 1
    assert {k.removeprefix("lane.blocker."): v["subject"] for k, v in lanes.items()} == {
        b: f"{n} lanes" for b, n in want.items()
    }


@pytest.mark.parametrize("path", CAPTURES, ids=lambda p: p.name)
def test_the_gate_summary_serves_no_last_tick_lane_blocker_under_a_stale_producer(path, tmp_path, monkeypatch):
    """The gate surface built its lane rows from the same to_session: under a dead producer it said
    7 lanes stale_relay, 6 no_claim, 3 no_session, 1 offline — the last tick, read as now."""
    capture = _load(path)
    _replay(capture, tmp_path, monkeypatch, capture["producer"]["age_s"])
    summary = reins_read.read_gate_summary(str(tmp_path), _ALLOW, {"orchestration_ledger_dir": str(tmp_path / "no-ledger")})
    lanes = _lane_gate_rows(summary)
    assert list(lanes) == ["lane.blocker.stale_producer"]
    assert lanes["lane.blocker.stale_producer"]["subject"] == f"{len(capture['sessions'])} lanes"
    source = next(s for s in summary["sources"] if s["id"] == "session_state")
    assert "stale" in source["detail"]


def test_observe_counts_lanes_from_a_live_producer(tmp_path, monkeypatch):
    capture = _load(CAPTURES[0])
    _replay(capture, tmp_path, monkeypatch, producer_age_s=0)
    health, agents = reins_read._observe_session_dimensions()
    assert (health["status"], agents["status"]) == ("live", "live")
    assert agents["count"] == len(capture["sessions"])


@pytest.mark.parametrize("path", CAPTURES, ids=lambda p: p.name)
def test_observe_does_not_call_a_dead_producers_lane_counts_live(path, tmp_path, monkeypatch):
    """/read/observe labelled the last tick's alive/idle/stalled counts `coordinator_state live`.
    Its dimensions are live|dark; under a stale producer they are dark with the reason named."""
    capture = _load(path)
    _replay(capture, tmp_path, monkeypatch, capture["producer"]["age_s"])
    for dim in reins_read._observe_session_dimensions():
        assert dim["status"] == "dark", dim
        assert dim["count"] is None, dim
        assert "stale" in dim["summary"], dim
