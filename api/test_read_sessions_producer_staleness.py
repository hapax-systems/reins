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
        assert (row["alive"], row["idle"], row["stalled"]) == (False, False, False), row["role"]


def test_the_row_verdict_follows_the_envelope_stanza_not_a_second_clock(tmp_path, monkeypatch):
    """One freshness verdict per response. If the stanza says live, rows keep the producer's word;
    if it says stale, rows do not — whatever the file's age at the moment each row was built."""
    capture = _load(CAPTURES[0])
    client = _replay(capture, tmp_path, monkeypatch, producer_age_s=0)
    monkeypatch.setattr(reins_read, "_producer_age_s", lambda _path: reins_read._PRODUCER_STALE_S + 1)
    body = client.get("/read/sessions").json()
    assert body["producer"]["state"] == "stale"
    assert not any(row["alive"] for row in body["sessions"])


def test_an_unmeasurable_producer_age_is_stale_for_the_rows_too(tmp_path, monkeypatch):
    """The file can vanish between the read and the stat; age None is already 'stale' in the
    envelope, and the rows must agree rather than default to the lanes' last word."""
    capture = _load(CAPTURES[0])
    client = _replay(capture, tmp_path, monkeypatch, producer_age_s=0)
    monkeypatch.setattr(reins_read, "_producer_age_s", lambda _path: None)
    body = client.get("/read/sessions").json()
    assert body["producer"]["state"] == "stale"
    assert not any(row["alive"] for row in body["sessions"])


def test_the_session_detail_door_does_not_assert_alive_under_a_stale_producer(tmp_path, monkeypatch):
    """/read/session/{role} is the same row one click deeper; it must not contradict the list."""
    capture = _load(CAPTURES[0])
    client = _replay(capture, tmp_path, monkeypatch, capture["producer"]["age_s"])
    body = client.get("/read/session/lane-01").json()
    assert body["dark"] is False
    detail = body["detail"]
    assert detail["health"]["alive"] is False
    assert detail["state"] == "unknown"
    assert detail["blocker"] == "stale_producer"
    assert detail["tmux"]["exists"] is False


def test_the_session_detail_door_keeps_the_live_verdict(tmp_path, monkeypatch):
    capture = _load(CAPTURES[0])
    body = _replay(capture, tmp_path, monkeypatch, producer_age_s=0).get("/read/session/lane-01").json()
    assert body["detail"]["health"]["alive"] is True
    assert body["detail"]["state"] == "active"
