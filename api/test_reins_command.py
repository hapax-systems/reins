"""TDD for the governed COMMAND surface — Inc 0: envelope + validator-and-router skeleton.

The router is a stateless validator-and-router (doctrine Invariant 4: the cockpit
never mints authority). It VERIFIES an already-minted authority_packet, runs the
same preflight predicate the door stubs compute, honors idempotency, and hands off
to an injected transport that owns the actual write. No CoordWriter, no .append(),
no spool write here — those live in the owning surfaces a later increment wires the
transport to (`python -m shared.coord_event_log append`, cc-claim/cc-close,
hapax-methodology-dispatch).
"""

from fastapi.testclient import TestClient

from reins_command import Envelope, Response, route_command


def _envelope(verb="resume", idempotency_key="k1", authority_packet="pkt", target="cx-crit"):
    return Envelope(
        verb=verb,
        target=target,
        authority_packet=authority_packet,
        preflight_receipt={},
        idempotency_key=idempotency_key,
    )


def test_route_command_rejects_unverified_authority():
    # verify-only: authority the router cannot verify is rejected, never minted
    resp = route_command(
        _envelope(),
        verify_authority=lambda _pkt, _tgt: False,
        preflight=lambda _env: True,
        transport=lambda _env: None,
        already_emitted={},
    )
    assert resp.status == "authority-rejected"
    assert resp.http == 403


def test_route_command_fails_preflight_when_authority_ok():
    # distinct from authority-rejected: the transition is illegal GIVEN valid authority
    resp = route_command(
        _envelope(),
        verify_authority=lambda _pkt, _tgt: True,
        preflight=lambda _env: False,
        transport=lambda _env: None,
        already_emitted={},
    )
    assert resp.status == "preflight-failed"
    assert resp.http == 409


def test_route_command_replays_idempotent_without_calling_transport():
    calls = []

    def transport(env):
        calls.append(env)
        return Response(status="ok", http=200, receipt_id="r1", event_seq=42)

    resp = route_command(
        _envelope(idempotency_key="dup"),
        verify_authority=lambda _pkt, _tgt: True,
        preflight=lambda _env: True,
        transport=transport,
        already_emitted={"dup": "r1"},
    )
    assert resp.status == "idempotent-replay"
    assert resp.http == 200 and resp.duplicate is True
    assert resp.receipt_id == "r1"
    assert calls == []  # transport NOT invoked on replay


def test_route_command_surfaces_transport_failure_never_synthesizes_success():
    resp = route_command(
        _envelope(),
        verify_authority=lambda _pkt, _tgt: True,
        preflight=lambda _env: True,
        transport=lambda _env: None,
        already_emitted={},
    )
    assert resp.status == "transport-failed"
    assert resp.http == 502


def test_route_command_success_returns_the_transport_receipt():
    def transport(_env):
        return Response(status="ok", http=200, receipt_id="rec-1", event_seq=7)

    resp = route_command(
        _envelope(),
        verify_authority=lambda _pkt, _tgt: True,
        preflight=lambda _env: True,
        transport=transport,
        already_emitted={},
    )
    assert resp.status == "ok" and resp.http == 200
    assert resp.receipt_id == "rec-1" and resp.event_seq == 7


def test_route_command_routes_via_verifier_not_trust():
    # the router MUST call verify_authority (route), not assume the packet is valid
    seen = {}

    def verify(_pkt, _tgt):
        seen["called"] = True
        return True

    route_command(
        _envelope(authority_packet="P"),
        verify_authority=verify,
        preflight=lambda _env: True,
        transport=lambda _env: Response(status="ok", http=200, receipt_id="r", event_seq=1),
        already_emitted={},
    )
    assert seen.get("called") is True


# ---- Inc 1: resume-intent preview wedge (HTTP endpoint + no-op transport) ----
# Proves the full envelope -> preflight -> receipt loop end-to-end with ZERO mint
# surface: the transport is a no-op that returns the stub's "would emit" preview as
# a structured receipt. No spine write, no CoordWriter, no authority minted.


def _resume_body(target="cx-crit", authority_packet="grant-xyz", idempotency_key="k1", blocked=False):
    return {
        "target": target,
        "authority_packet": authority_packet,
        "preflight_receipt": {"blocked": blocked},
        "idempotency_key": idempotency_key,
    }


def test_resume_preview_returns_fold_delta_with_no_real_write():
    from reins_command import resume_preview_app

    resp = TestClient(resume_preview_app()).post("/command/resume", json=_resume_body())
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"
    assert "session.resume" in body["fold_delta"]
    assert body["event_seq"] is None  # no real spine write
    assert body["spooled"] is False


def test_resume_preview_rejects_missing_authority():
    from reins_command import resume_preview_app

    resp = TestClient(resume_preview_app()).post(
        "/command/resume", json=_resume_body(authority_packet="")
    )
    assert resp.status_code == 403
    assert resp.json()["status"] == "authority-rejected"


def test_resume_preview_fails_blocked_preflight():
    from reins_command import resume_preview_app

    resp = TestClient(resume_preview_app()).post(
        "/command/resume", json=_resume_body(blocked=True)
    )
    assert resp.status_code == 409
    assert resp.json()["status"] == "preflight-failed"


def test_resume_preview_idempotent_replay_does_not_re_emit():
    from reins_command import resume_preview_app

    client = TestClient(resume_preview_app())
    body = _resume_body(idempotency_key="dup-x")
    first = client.post("/command/resume", json=body)
    second = client.post("/command/resume", json=body)
    assert first.status_code == 200 and second.status_code == 200
    assert first.json()["duplicate"] is False
    assert second.json()["duplicate"] is True


def test_command_endpoint_unwired_verbs_are_not_implemented():
    from reins_command import resume_preview_app

    resp = TestClient(resume_preview_app()).post("/command/dispatch", json=_resume_body())
    assert resp.status_code == 501


# ---- Inc 2: dispatch (first real spine-write genus, async via methodology-dispatch) ----
# The cockpit COMPOSES a DispatchLaunchRequest and SUBMITS it to the methodology-dispatch
# MQ via an injected boundary (route, never mint — the daemon consumes, authorizes via
# validate_task, launches, appends coord_dispatch). The receipt is the pending MQ
# message_id; the spine event lands async on the next fold. The real enqueue + lane
# launch is a confirmed e2e step; here the submit boundary is injected (TDD).


def _dispatch_body(
    task_id="reins-air-confidentiality-interaction-hardening-20260624",
    lane="cx-crit",
    authority_case="CASE-X",
    parent_spec="/spec.md",
    message_id="mq-123",
    idempotency_key="dkey-1",
    blocked=False,
    omit=(),
):
    pkt = {
        "lane": lane,
        "platform": "codex",
        "mode": "headless",
        "profile": "full",
        "authority_case": authority_case,
        "parent_spec": parent_spec,
        "message_id": message_id,
    }
    for k in omit:
        pkt.pop(k, None)
    return {
        "target": task_id,
        "authority_packet": pkt,
        "preflight_receipt": {"blocked": blocked},
        "idempotency_key": idempotency_key,
    }


def test_dispatch_composes_request_and_submits_async():
    from reins_command import dispatch_app

    submitted = []

    def submit(req):
        submitted.append(req)
        return "mq-msg-1"

    resp = TestClient(dispatch_app(submit_dispatch=submit)).post(
        "/command/dispatch", json=_dispatch_body()
    )
    assert resp.status_code == 200
    body = resp.json()
    assert body["receipt_id"] == "mq-msg-1"  # the pending MQ message id
    assert body["event_seq"] is None  # spine event lands async via the daemon
    req = submitted[0]
    assert req.task_id == "reins-air-confidentiality-interaction-hardening-20260624"
    assert req.lane == "cx-crit"
    assert req.platform == "codex" and req.mode == "headless" and req.profile == "full"
    assert req.authority_case == "CASE-X"
    assert req.parent_spec == "/spec.md"
    assert req.message_id == "mq-123"
    assert req.idempotency_key == "dkey-1"


def test_dispatch_requires_authority_case_and_parent_spec():
    from reins_command import dispatch_app

    resp = TestClient(dispatch_app(submit_dispatch=lambda _r: "m")).post(
        "/command/dispatch", json=_dispatch_body(omit=("authority_case", "parent_spec"))
    )
    assert resp.status_code == 403


def test_dispatch_surfaces_submit_failure_never_synthesizes_success():
    from reins_command import dispatch_app

    def submit(_r):
        raise RuntimeError("mq-down")

    resp = TestClient(dispatch_app(submit_dispatch=submit)).post(
        "/command/dispatch", json=_dispatch_body()
    )
    assert resp.status_code == 502
    assert resp.json()["status"] == "transport-failed"
