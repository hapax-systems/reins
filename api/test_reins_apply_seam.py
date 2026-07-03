"""Keystone functional half — the witnessed REAL-apply cycle (mock-daemon, NO spawn).

Proves the full cycle: dispatch (a REAL sqlite enqueue via reins_dispatch_mq) -> signed verdict
(applied=False, witness=pending — the launch is async/downstream) -> witness-echo (U7) on a
coord_dispatch.launch_succeeded event -> applied=True, witness=echoed. This is "witnessed real apply,"
mock-daemon-proven: a temp MQ + an injected coord event. NO process spawn, NO live daemon — the lane
launch is hapax-methodology-dispatch's correctness on the operator fleet (downstream of reins's enqueue).
"""
import inspect

import reins_dispatch_mq
import reins_ledger


def _intent(message_id="msg-1", lane="cx_red"):
    return reins_dispatch_mq.DispatchIntent(
        task_id="t1",
        lane=lane,
        platform="codex",
        mode="headless",
        profile="full",
        authority_case="CASE-X",
        parent_spec="/spec.md",
        message_id=message_id,
        idempotency_key="k1",
    )


def test_producer_enqueues_a_real_dispatch_row(tmp_path):
    db = str(tmp_path / "relay.db")
    msg_id = reins_dispatch_mq.send_dispatch_message(_intent(), db_path=db)
    assert msg_id == "msg-1"  # caller-minted message_id is preserved (the receipt_id)
    row = reins_dispatch_mq.read_dispatch_message("msg-1", db_path=db)
    assert row is not None
    assert row["message_type"] == "dispatch"
    assert row["sender"] == "reins"
    assert row["authority_case"] == "CASE-X"  # required for dispatch
    assert row["subject"] == "t1"
    assert row["recipients_spec"] == "cx-red"  # lane normalized (cx_red -> cx-red)
    assert row["recipients"] == [{"recipient": "cx-red", "state": "offered"}]
    assert row["payload_hash"]  # sha256 of the payload — the integrity binding


def test_producer_is_pure_sqlite_no_spawn():
    # send_dispatch_message MUST NOT spawn/subprocess/HTTP — it's a sqlite INSERT. The spawn boundary is
    # downstream (hapax-methodology-dispatch --launch). The docstring MENTIONS "subprocess" explanatorily,
    # so check the module imports/calls no spawn surface (globals + the call forms), not a bare substring.
    assert not hasattr(reins_dispatch_mq, "subprocess")
    assert not hasattr(reins_dispatch_mq, "Popen")
    src = inspect.getsource(reins_dispatch_mq)
    for forbidden in (
        "import subprocess",
        "subprocess.run",
        "subprocess.Popen",
        "subprocess.call",
        "subprocess.check",
        "os.system(",
        "Popen(",
    ):
        assert forbidden not in src, f"producer must not call {forbidden!r} (spawn boundary)"


def test_producer_mints_uuid7_when_no_message_id(tmp_path):
    db = str(tmp_path / "relay.db")
    msg_id = reins_dispatch_mq.send_dispatch_message(_intent(message_id=""), db_path=db)
    assert msg_id and msg_id != "msg-1"  # minted (UUIDv7-ish: 36 chars, dashes)
    assert reins_dispatch_mq.read_dispatch_message(msg_id, db_path=db) is not None


def test_witnessed_real_apply_cycle_mock_daemon(tmp_path, monkeypatch):
    # the FULL keystone cycle, mock-daemon (no live spawn):
    led_path = str(tmp_path / "commands.jsonl")
    monkeypatch.setenv("REINS_COMMAND_LEDGER", led_path)
    monkeypatch.setenv("REINS_COMMAND_LEDGER_KEY", str(tmp_path / "ledger-key"))
    db = str(tmp_path / "relay.db")
    ledger = reins_ledger.CommandLedger(led_path, clock=lambda: "2026-07-03T00:00:00Z")

    # 1. demand + REAL enqueue (sqlite, no spawn) + the dispatch verdict (applied=False: launch is async)
    demand = ledger.record_demand("dispatch", "t1", "k1")
    eid = demand["event_id"]
    msg_id = reins_dispatch_mq.send_dispatch_message(_intent(), db_path=db)
    assert msg_id == "msg-1"
    ledger.record_verdict(eid, "ok", 200, effect={"receipt_id": msg_id, "applied": False})

    # the enqueue wrote a real MQ row the dispatcher would consume
    assert reins_dispatch_mq.read_dispatch_message("msg-1", db_path=db)["sender"] == "reins"
    # at enqueue: NOT yet applied (honest — the lane launch is downstream, not the enqueue)
    verdicts = [r for r in ledger.rows() if r.get("kind") == "verdict"]
    assert verdicts[-1]["effect"]["applied"] is False
    assert verdicts[-1]["witness"] == "pending"

    # 2. witness-echo (U7): a coord_dispatch.launch_succeeded for msg-1 (mock daemon -> spine echo)
    echoed = ledger.apply_witness_echo(msg_id, launch_succeeded=True, reason="lane cx-red launched")
    assert echoed is not None and echoed["witness"] == "echoed"
    # the last verdict is now APPLIED + WITNESSED (the real launch, echoed by the spine)
    verdicts = [r for r in ledger.rows() if r.get("kind") == "verdict"]
    assert verdicts[-1]["effect"]["applied"] is True
    assert verdicts[-1]["witness"] == "echoed"
    # the ledger's signed hash-chain is intact across the whole cycle
    ok, _i, reason = ledger.verify_chain()
    assert ok and reason == "ok"

    # an UNMATCHED echo (no matching dispatch verdict) is an honest no-op, never a fabricated echo
    assert ledger.apply_witness_echo("no-such-message", True) is None


def test_witness_echo_launch_failure_records_not_applied(tmp_path, monkeypatch):
    # honest-dark: a coord_dispatch.launch_FAILED echo records applied=False + witness=echoed (the spine
    # echoed a FAILURE — never a false-green applied)
    led_path = str(tmp_path / "commands.jsonl")
    monkeypatch.setenv("REINS_COMMAND_LEDGER", led_path)
    monkeypatch.setenv("REINS_COMMAND_LEDGER_KEY", str(tmp_path / "ledger-key"))
    ledger = reins_ledger.CommandLedger(led_path, clock=lambda: "2026-07-03T00:00:00Z")
    demand = ledger.record_demand("dispatch", "t2", "k2")
    eid = demand["event_id"]
    ledger.record_verdict(eid, "ok", 200, effect={"receipt_id": "msg-2", "applied": False})
    echoed = ledger.apply_witness_echo("msg-2", launch_succeeded=False, reason="preflight rejected")
    assert echoed["witness"] == "echoed"
    verdicts = [r for r in ledger.rows() if r.get("kind") == "verdict"]
    assert verdicts[-1]["effect"]["applied"] is False  # the launch FAILED -> not applied (honest)
    assert verdicts[-1]["status"] == "launch-failed"
