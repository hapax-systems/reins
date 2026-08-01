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

from k0.fail_closed import Evaluation
from k0.refusal import Refusal


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
    # applied = a REAL estate write occurred (NOT a preview). A preview/witness-only transport returns a
    # receipt but leaves this False, so `applied` never false-greens a preview (the mode is the writer's
    # to assert, not inferable from receipt-presence). Only a governed real-write transport sets True.
    applied: bool = False
    # K0 refusal-as-data (INV-3, "BLOCKED always escapes"): every refusing verdict states the LEGAL NEXT
    # move. A refusal that only says "no" is a dead end, and a dead end is a trap, not a gate. Populated
    # from a k0.Refusal, whose constructor REFUSES to build one with an empty legal_next.
    legal_next: str | None = None
    teaches: str | None = None


def _evaluate(predicate: Callable[..., Any], *args: Any) -> tuple[Evaluation, str]:
    """Run a governed predicate under the K0 fail-closed law. Returns (verdict, why).

    THE LAW HAS THREE OUTCOMES, NOT TWO. A ``bool`` predicate can only say yes or no; it has no way
    to say "I could not tell". That third case is the dangerous one, and the naive implementation
    turns it into whichever of the two arms it happens to fall through to.

    Measured on the pre-K0 router: an ``authority_packet`` of unexpected shape made ``packet.get()``
    raise, the exception escaped ``route_command`` entirely, and the surface answered HTTP 500 with a
    non-JSON body. That is not a governed denial — no status, no reason, no legal next move. It
    denies in effect, but it denies INCOHERENTLY, which is exactly the dead end INV-3 forbids.

    So: a predicate that RAISES is UNEVALUABLE, and a predicate that returns None has stated no
    verdict and is UNEVALUABLE too. Both DENY. Predicates may return an ``Evaluation`` directly to
    say so themselves; plain ``bool`` remains legal and keeps its meaning.
    """
    try:
        verdict = predicate(*args)
    except Exception as exc:  # noqa: BLE001 — ANY failure to evaluate is UNEVALUABLE, and denies
        return Evaluation.UNEVALUABLE, f"the predicate could not be evaluated ({type(exc).__name__}: {exc})"
    if isinstance(verdict, Evaluation):
        return verdict, ""
    if verdict is None:
        return Evaluation.UNEVALUABLE, "the predicate returned None — it stated no verdict"
    return (Evaluation.SATISFIED if verdict else Evaluation.VIOLATED), ""


def _refuse(*, gate: str, why: str, legal_next: str, status: str, http: int, teaches: str) -> Response:
    """Build a refusing verdict THROUGH k0.Refusal, so the dead-end law is enforced by construction.

    Refusal's constructor raises DeadEndRefusalError on an empty ``legal_next``. Routing every denial
    through it means this module cannot emit a refusal that fails to teach the caller what to do —
    the guarantee is structural, not a convention reviewers must remember to check.
    """
    refusal = Refusal(gate=gate, why=why, legal_next=legal_next, teaches=teaches)
    return Response(
        status=status,
        http=http,
        reason=refusal.why,
        legal_next=refusal.legal_next,
        teaches=refusal.teaches or None,
    )


def route_command(
    envelope: Envelope,
    *,
    verify_authority: Callable[[Any, str], bool | Evaluation],
    preflight: Callable[[Envelope], bool | Evaluation],
    transport: Callable[[Envelope], Response | None],
    already_emitted: dict[str, str],
) -> Response:
    """Validate + route a governed command. Pure: all side-effecting surfaces are
    injected (``verify_authority`` / ``preflight`` / ``transport``), so this is a
    stateless Elm-style fold over the envelope. Never mints authority.

    K0-GATED. Each predicate runs under the fail-closed law (three outcomes, UNEVALUABLE denies) and
    every refusing arm returns refusal-as-data carrying ``legal_next``.

    WHY AN UNEVALUABLE ARM REUSES ITS SIBLING'S HTTP CODE. An unevaluable authority check answers 403
    and an unevaluable preflight answers 409 — the same codes as their VIOLATED siblings — while the
    ``status`` string keeps the two distinguishable (``authority-unevaluable`` vs
    ``authority-rejected``). A NEW code would be the more expressive choice and the less safe one:
    every client already treats 403/409 as a denial, whereas an unrecognised code is exactly the kind
    of thing a client's ``if resp.ok`` fallthrough turns into a pass. The distinction an auditor needs
    lives in the body, where a new field breaks nothing.
    """
    # 1. Verify-only authority — the packet must be checkable; never trusted blind.
    verdict, why = _evaluate(verify_authority, envelope.authority_packet, envelope.target)
    if verdict is not Evaluation.SATISFIED:
        unevaluable = verdict is Evaluation.UNEVALUABLE
        return _refuse(
            gate=f"command:{envelope.verb}:authority",
            why=why or f"the authority packet is not valid for target {envelope.target!r}",
            legal_next=(
                "mint authority upstream (coord-grant-mint -> EscapeGrant, or a methodology-dispatch "
                "AuthorityCase + parent_spec) and resubmit; this surface verifies, it never mints"
            ),
            status="authority-unevaluable" if unevaluable else "authority-rejected",
            http=403,
            teaches="doctrine/route-never-mint",
        )
    # 2. Preflight — the transition must be legal GIVEN valid authority (distinct
    #    from authority-rejected: the stubs' doorVerbLegal / intentStatusFor gate).
    verdict, why = _evaluate(preflight, envelope)
    if verdict is not Evaluation.SATISFIED:
        unevaluable = verdict is Evaluation.UNEVALUABLE
        return _refuse(
            gate=f"command:{envelope.verb}:preflight",
            why=why or f"{envelope.verb} is not legal from the target's current state",
            legal_next=(
                "re-run the cockpit preview to obtain a fresh, unblocked preflight receipt, then "
                "resubmit; if it is still blocked the transition is illegal from this state"
            ),
            status="preflight-unevaluable" if unevaluable else "preflight-failed",
            http=409,
            teaches="doctrine/preflight",
        )
    # 3. Idempotency — a replayed key never re-invokes the transport (the substrate
    #    UNIQUE on event_id makes retries free; this mirrors it at the router).
    if envelope.idempotency_key in already_emitted:
        return Response(
            status="idempotent-replay",
            http=200,
            duplicate=True,
            receipt_id=already_emitted[envelope.idempotency_key],
        )
    # 4. Hand off to the owning surface; never synthesize a success on failure. A transport that
    #    RAISES is a failure like any other: letting it escape would answer 500-with-no-body, losing
    #    the one thing the caller needs — whether the write happened. It did not; say so, governed.
    try:
        receipt = transport(envelope)
    except Exception as exc:  # noqa: BLE001 — the owning surface's failure is ours to report, not to leak
        receipt = None
        why = f"the owning surface failed ({type(exc).__name__}: {exc})"
    else:
        why = "the owning surface returned no receipt, so nothing is known to have been written"
    if receipt is None:
        return _refuse(
            gate=f"command:{envelope.verb}:transport",
            why=why,
            legal_next=(
                "nothing was written — retry with the SAME idempotency_key once the owning surface is "
                "reachable; the key makes the retry free if the write did in fact land"
            ),
            status="transport-failed",
            http=502,
            teaches="doctrine/route-never-mint",
        )
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
        "applied": resp.applied,
        "legal_next": resp.legal_next,
        "teaches": resp.teaches,
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
            # Also a refusal, and so it must also escape: a bare "not wired" leaves the caller with
            # no move. Routed through _refuse for the same structural guarantee as every other arm.
            return JSONResponse(
                _resp_to_dict(
                    _refuse(
                        gate=f"command:{v}:wiring",
                        why=f"{v} is not wired on this router (it serves {verb!r})",
                        legal_next=(
                            f"use the cockpit's never-mint preview for {v}, or POST to the router that "
                            f"serves it; /read/meta.verbs lists the live wired set"
                        ),
                        status="not-implemented",
                        http=501,
                        teaches="doctrine/one-command-surface",
                    )
                ),
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


def _default_submit_dispatch(req: Any) -> str:
    # The REAL producer: a pure sqlite INSERT into the relay MQ (api/reins_dispatch_mq.send_dispatch_message).
    # NO SPAWN — the lane-launch is downstream (hapax-methodology-dispatch --launch, via the coordinator's
    # tick on a matching cc-task). Tests inject a temp-db submit_dispatch; production writes the live MQ
    # (~/.cache/hapax/relay/messages.db) the dispatcher reads. The enqueue is a real governed write; the
    # verdict's applied=True is armed later by the witness-echo (U7) on coord_dispatch.launch_succeeded.
    from reins_dispatch_mq import send_dispatch_message  # local import: keep the command layer import-light

    return send_dispatch_message(req)
