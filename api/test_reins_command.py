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
