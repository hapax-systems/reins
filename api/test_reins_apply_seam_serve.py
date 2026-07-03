"""Keystone functional half slice 2 — the dispatch verb WIRED LIVE in the serve layer.

Proves /command/dispatch is no longer "not-wired": it routes through the real producer (an injectable
submit_dispatch; tests use a temp MQ), returns 200 + the message_id receipt, records the verdict
(applied=False — the launch is async), and the witness-echo (U7) arms applied=True. NO SPAWN, NO live MQ.

These tests import reins_serve -> reins_read -> hapax.spine, so they need hapax-spine on the path (the CI
venv installs the wheel; locally, put the hapax-spine source on PYTHONPATH). The slice-1 cycle tests in
test_reins_apply_seam.py stay hapax-spine-free.
"""
import reins_dispatch_mq
import reins_ledger
import reins_serve
from fastapi.testclient import TestClient


def _dispatch_body(message_id="msg-1", blocked=False):
    return {
        "target": "t1",
        "authority_packet": {
            "lane": "cx_red",
            "platform": "codex",
            "mode": "headless",
            "profile": "full",
            "authority_case": "CASE-X",
            "parent_spec": "/spec.md",
            "message_id": message_id,
        },
        "preflight_receipt": {"blocked": blocked},
        "idempotency_key": "k1",
    }


def test_dispatch_closures_transport_calls_the_producer_and_leaves_applied_false():
    # _dispatch_closures' transport calls submit (the producer) + returns a Response whose applied stays
    # False (the enqueue; the lane launch is async, armed by the witness-echo). verify/preflight mirror dispatch_app.
    verify, preflight, transport = reins_serve._dispatch_closures(submit=lambda req: req.message_id)
    env = reins_serve.reins_command.Envelope(
        verb="dispatch",
        target="t1",
        authority_packet=_dispatch_body()["authority_packet"],
        preflight_receipt={"blocked": False},
        idempotency_key="k1",
    )
    assert verify(env.authority_packet, env.target) is True
    assert preflight(env) is True
    resp = transport(env)
    assert resp.status == "ok" and resp.http == 200
    assert resp.receipt_id == "msg-1"
    assert resp.applied is False  # enqueue alone -> not applied (honest)
    # authority-rejected: missing the authority triple
    assert verify({"lane": "x"}, "t1") is False
    # preflight-failed: blocked
    assert preflight(reins_serve.reins_command.Envelope(
        verb="dispatch", target="t1", authority_packet=_dispatch_body()["authority_packet"],
        preflight_receipt={"blocked": True}, idempotency_key="k1")) is False


def test_serve_dispatch_is_wired_and_enqueues_via_the_producer(tmp_path, monkeypatch):
    # /command/dispatch is WIRED (200, not the 501 not-wired) and calls the injected producer (temp MQ,
    # no spawn, no live daemon). Then the witness-echo (U7) arms applied=True.
    monkeypatch.setenv("REINS_COMMAND_LEDGER", str(tmp_path / "commands.jsonl"))
    monkeypatch.setenv("REINS_COMMAND_LEDGER_KEY", str(tmp_path / "ledger-key"))
    db = str(tmp_path / "relay.db")

    def submit(req):
        # the REAL producer against a temp db — proves the serve wiring reaches a real sqlite enqueue
        return reins_dispatch_mq.send_dispatch_message(req, db_path=db)

    app = reins_serve.build_serve_app("", ["verb", "status", "witness", "applied"], {}, submit_dispatch=submit)
    client = TestClient(app)

    r = client.post("/command/dispatch", json=_dispatch_body())
    assert r.status_code == 200, r.text  # WIRED (the prior state returned 501 not-wired)
    out = r.json()
    assert out["status"] == "ok"
    assert out["receipt_id"] == "msg-1"  # the producer's message_id is the receipt
    # the producer really enqueued a dispatch row (a real sqlite write; no spawn)
    row = reins_dispatch_mq.read_dispatch_message("msg-1", db_path=db)
    assert row is not None and row["sender"] == "reins" and row["message_type"] == "dispatch"

    # at enqueue: applied=False (the lane launch is async/downstream, NOT the enqueue)
    led = reins_ledger.CommandLedger(str(tmp_path / "commands.jsonl"))
    verdicts = [row for row in led.rows() if row.get("kind") == "verdict"]
    assert verdicts[-1]["effect"]["applied"] is False
    assert verdicts[-1]["witness"] == "pending"

    # witness-echo (U7): a coord_dispatch.launch_succeeded for msg-1 -> applied=True, witness=echoed
    led.apply_witness_echo("msg-1", launch_succeeded=True, reason="lane cx-red launched")
    verdicts = [row for row in led.rows() if row.get("kind") == "verdict"]
    assert verdicts[-1]["effect"]["applied"] is True
    assert verdicts[-1]["witness"] == "echoed"
    ok, _i, reason = led.verify_chain()
    assert ok and reason == "ok"


def test_serve_dispatch_rejects_missing_authority(tmp_path, monkeypatch):
    # the wired dispatch verb still enforces verify-only authority (route, never mint)
    monkeypatch.setenv("REINS_COMMAND_LEDGER", str(tmp_path / "commands.jsonl"))
    monkeypatch.setenv("REINS_COMMAND_LEDGER_KEY", str(tmp_path / "ledger-key"))
    app = reins_serve.build_serve_app("", [], {}, submit_dispatch=lambda req: req.message_id)
    client = TestClient(app)
    body = _dispatch_body()
    body["authority_packet"] = {"lane": "cx_red"}  # missing authority_case/parent_spec/message_id
    r = client.post("/command/dispatch", json=body)
    assert r.status_code == 403
    assert r.json()["status"] == "authority-rejected"
