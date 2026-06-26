"""Governed COMMAND surface — stateless validator-and-router (Inc 0 skeleton).

Doctrine Invariant 4: the cockpit never mints authority. This router VERIFIES an
already-minted ``authority_packet`` (minted upstream by the operator via
``coord-grant-mint`` -> EscapeGrant, or by the methodology-dispatch ledger as an
AuthorityCase + parent_spec), runs the same preflight predicate the door stubs
compute, honors idempotency, and hands off to an injected ``transport`` that owns
the actual write. A later increment wires the transport to the sanctioned surfaces
(``python -m shared.coord_event_log append``, ``cc-claim``/``cc-close``,
``hapax-methodology-dispatch``).

This module NEVER imports CoordWriter, NEVER calls ``.append()``, NEVER writes the
spool dir — route, never mint. The daemon-writer wall is a forwarder convention
(``CoordWriter.daemon`` is a free-form public constructor with no process-identity
check), so the discipline is enforced HERE, at the only place it can be: this file
composes + verifies + forwards, and physically cannot append.
"""

from dataclasses import dataclass
from typing import Any, Callable


@dataclass
class Envelope:
    """A governed command. The closed verb set, target, verify-only authority,
    the dry-run preflight receipt, and an idempotency key (hashed into the
    substrate ``event_id`` by the transport in a later increment)."""

    verb: str
    target: str
    authority_packet: Any
    preflight_receipt: dict
    idempotency_key: str


@dataclass
class Response:
    """The router's verdict. ``status`` is one of: ok, authority-rejected,
    preflight-failed, idempotent-replay, transport-failed. ``receipt_id`` /
    ``event_seq`` carry the substrate ``AppendReceipt`` on success; ``event_seq``
    is null for the filesystem-as-bus genus until the reactive daemon projects it.
    """

    status: str
    http: int
    receipt_id: str | None = None
    event_seq: int | None = None
    duplicate: bool = False
    reason: str | None = None


def route_command(
    envelope: Envelope,
    *,
    verify_authority: Callable[[Any, str], bool],
    preflight: Callable[[Envelope], bool],
    transport: Callable[[Envelope], Response | None],
    already_emitted: dict[str, str],
) -> Response:
    """Validate + route a governed command. Pure: all side-effecting surfaces are
    injected (``verify_authority`` / ``preflight`` / ``transport``), so this is a
    stateless Elm-style fold over the envelope. Never mints authority."""
    # 1. Verify-only authority — the packet must be checkable; never trusted blind.
    if not verify_authority(envelope.authority_packet, envelope.target):
        return Response(status="authority-rejected", http=403)
    # 2. Preflight — the transition must be legal GIVEN valid authority (distinct
    #    from authority-rejected: the stubs' doorVerbLegal / intentStatusFor gate).
    if not preflight(envelope):
        return Response(status="preflight-failed", http=409)
    # 3. Idempotency — a replayed key never re-invokes the transport (the substrate
    #    UNIQUE on event_id makes retries free; this mirrors it at the router).
    if envelope.idempotency_key in already_emitted:
        return Response(
            status="idempotent-replay",
            http=200,
            duplicate=True,
            receipt_id=already_emitted[envelope.idempotency_key],
        )
    # 4. Hand off to the owning surface; never synthesize a success on failure.
    receipt = transport(envelope)
    if receipt is None:
        return Response(status="transport-failed", http=502)
    return receipt
