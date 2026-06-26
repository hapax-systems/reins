"""TDD for the governed COMMAND surface — Inc 0: envelope + validator-and-router skeleton.

The router is a stateless validator-and-router (doctrine Invariant 4: the cockpit
never mints authority). It VERIFIES an already-minted authority_packet, runs the
same preflight predicate the door stubs compute, honors idempotency, and hands off
to an injected transport that owns the actual write. No CoordWriter, no .append(),
no spool write here — those live in the owning surfaces a later increment wires the
transport to (`python -m shared.coord_event_log append`, cc-claim/cc-close,
hapax-methodology-dispatch).
"""

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
