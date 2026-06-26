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

from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pydantic import BaseModel


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
    fold_delta: str | None = None
    spooled: bool = False
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


class CommandRequest(BaseModel):
    """The HTTP body for POST /command/{verb}. ``authority_packet`` is verify-only
    data (an EscapeGrant, an AuthorityCase triple, or a verb-bound capability) —
    minted upstream, never here."""

    target: str
    authority_packet: Any
    preflight_receipt: dict
    idempotency_key: str


def _resp_to_dict(resp: Response) -> dict:
    return {
        "status": resp.status,
        "http": resp.http,
        "receipt_id": resp.receipt_id,
        "event_seq": resp.event_seq,
        "fold_delta": resp.fold_delta,
        "spooled": resp.spooled,
        "duplicate": resp.duplicate,
        "reason": resp.reason,
    }


def build_command_app(
    *,
    verb: str,
    verify_authority: Callable[[Any, str], bool],
    preflight: Callable[[Envelope], bool],
    transport: Callable[[Envelope], Response | None],
) -> FastAPI:
    """A thin HTTP wrapper around route_command for one wired verb. All effectful
    surfaces are injected (verify/preflight/transport) — this adds NO authority of
    its own. Idempotency for the preview wedge is an in-memory ``emitted`` map; the
    substrate's UNIQUE on event_id replaces it for real writes (Inc 2+)."""

    app = FastAPI()
    emitted: dict[str, str] = {}

    @app.post("/command/{v}")
    def command(v: str, req: CommandRequest) -> JSONResponse:
        if v != verb:
            return JSONResponse(
                {"status": "not-implemented", "http": 501, "reason": f"{v} not wired"},
                status_code=501,
            )
        envelope = Envelope(
            verb=v,
            target=req.target,
            authority_packet=req.authority_packet,
            preflight_receipt=req.preflight_receipt,
            idempotency_key=req.idempotency_key,
        )
        resp = route_command(
            envelope,
            verify_authority=verify_authority,
            preflight=preflight,
            transport=transport,
            already_emitted=emitted,
        )
        if resp.status == "ok" and resp.receipt_id:
            emitted[envelope.idempotency_key] = resp.receipt_id
        return JSONResponse(_resp_to_dict(resp), status_code=resp.http)

    return app


def resume_preview_app() -> FastAPI:
    """Inc 1 wedge: the resume-intent preview. A no-op transport returns the stub's
    'would emit session.resume(<lane>)' preview as a structured receipt — proving the
    full contract end-to-end with ZERO mint surface (no spine write, no authority
    minted). Inc 2 wires the real transport for the first real write (dispatch)."""

    def verify(packet: Any, target: str) -> bool:
        # Preview wedge: the packet must be present + the lane identity resolvable.
        # Inc 2 replaces this with verify_escape_grant (real, route-not-mint).
        return bool(packet) and bool(target)

    def preflight(env: Envelope) -> bool:
        # The cockpit dry-ran the transition; a blocked receipt forbids the verb.
        return not env.preflight_receipt.get("blocked")

    def transport(env: Envelope) -> Response:
        return Response(
            status="ok",
            http=200,
            receipt_id=f"preview-{env.idempotency_key}",
            event_seq=None,  # no real spine write
            fold_delta=(
                f"would emit session.resume({env.target}) via the governed COMMAND "
                "surface — preview (no-op transport; Inc 2 wires the real write)"
            ),
            spooled=False,
        )

    return build_command_app(
        verb="resume", verify_authority=verify, preflight=preflight, transport=transport
    )


@dataclass
class DispatchIntent:
    """The cockpit-owned dispatch intent — the subset of the daemon's
    ``DispatchLaunchRequest`` that the cockpit composes (task / lane / platform / mode /
    profile / authority). The daemon adds its OWN ``mq_db_path`` + ``event_log`` when it
    builds the full request. The cockpit never touches those daemon-owned resources —
    route, never mint."""

    task_id: str
    lane: str
    platform: str
    mode: str
    profile: str
    authority_case: str
    parent_spec: str | None
    message_id: str
    idempotency_key: str | None = None


def dispatch_app(*, submit_dispatch: Callable[[Any], str] | None = None) -> FastAPI:
    """Inc 2: the dispatch verb — first real spine-write genus. Composes a
    ``DispatchIntent`` (the cockpit-owned subset) and SUBMITS it to the
    methodology-dispatch MQ via an
    injected ``submit_dispatch`` boundary — route, never mint. The cockpit submits an
    intent; the daemon consumes it, authorizes via ``validate_task``, launches, and
    appends the ``coord_dispatch.launch_*`` event. The receipt is the pending MQ
    message_id; the spine event (with event_seq) lands async on the next fold.

    The default submit raises NotImplementedError — Inc 2 proves composition + routing
    with an injected boundary; the real MQ-enqueue + lane-launch is a confirmed e2e
    step (it spawns a process, so it is operator-confirmed, not autonomous)."""

    submit = submit_dispatch or _default_submit_dispatch

    def verify(packet: Any, target: str) -> bool:
        # Authority triple (methodology-dispatch's model): route-not-mint.
        return bool(target) and all(
            packet.get(k) for k in ("authority_case", "parent_spec", "message_id")
        )

    def preflight(env: Envelope) -> bool:
        if env.preflight_receipt.get("blocked"):
            return False
        pkt = env.authority_packet
        return all(pkt.get(k) for k in ("lane", "platform", "mode", "profile"))

    def transport(env: Envelope) -> Response | None:
        pkt = env.authority_packet
        req = DispatchIntent(
            task_id=env.target,
            lane=pkt["lane"],
            platform=pkt["platform"],
            mode=pkt["mode"],
            profile=pkt["profile"],
            authority_case=pkt["authority_case"],
            parent_spec=pkt["parent_spec"],
            message_id=pkt["message_id"],
            idempotency_key=env.idempotency_key,
        )
        try:
            message_id = submit(req)
        except Exception as exc:
            return Response(status="transport-failed", http=502, reason=str(exc))
        return Response(
            status="ok",
            http=200,
            receipt_id=message_id,
            event_seq=None,  # spine event lands async via the daemon
            fold_delta=(
                f"dispatch submitted (message {message_id}); lane launch is async via "
                "hapax-methodology-dispatch — the coord_dispatch event lands on the next fold"
            ),
            spooled=False,
        )

    return build_command_app(
        verb="dispatch", verify_authority=verify, preflight=preflight, transport=transport
    )


def _default_submit_dispatch(_req: Any) -> str:
    raise NotImplementedError(
        "dispatch MQ producer not wired — inject submit_dispatch (Inc 2 proves composition "
        "+ routing; the real enqueue + lane-launch e2e is a confirmed step)"
    )
