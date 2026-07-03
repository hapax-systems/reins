"""reins_serve — the ONE composed serving surface (convergence design pack, slice U1).

Composition, not authority: the read app (reins_read.build_app) and the verb-table
command router serve from ONE process on ONE port. Authority lives in the ENVELOPE
(verify-already-minted packets), never in the transport or the port — this module
adds no authority of its own and must never import a mint/dispatch-capable surface
(pinned by the import-graph guard test).

- /read/*        — the read model, unchanged.
- /read/meta     — the serving-identity handshake: {app: "reins", serving_sha, ...}.
                   The cockpit verifies identity on first fold; a port that answers
                   without this identity renders PORT: FOREIGN SERVER (the 8799
                   squatter class becomes a rendered state, impossible to miss).
- /command/{verb}— the verb-table router. Day-1 table ships every verb wired:false
                   except the read-only resume PREVIEW (zero mint surface); an
                   unwired or unregistered verb returns a TYPED 501 refusal. The
                   governed rail (demand receipts, ledger, witnesses) lands at U3+.

Honesty floor: if the command router cannot be built, the app degrades to READ-ONLY
and DISCLOSES it via /read/meta (router: "degraded:<reason>") — never dark, never
half-mounted.
"""

from __future__ import annotations

import hashlib
import os
import subprocess
from typing import Any

from fastapi import FastAPI
from fastapi.responses import JSONResponse

import reins_command
import reins_generation
import reins_ledger
import reins_read
import reins_route

# The day-1 verb table (design pack §Design 3): every verb present and typed, only
# the read-only resume preview is wired. Wiring a verb here without its governed
# transport is forbidden — the rail (U3+) injects transports, this table only
# declares surface shape.
VERB_TABLE: dict[str, dict[str, Any]] = {
    "dispatch": {"wired": False},
    "claim": {"wired": False},
    "close": {"wired": False},
    "approve": {"wired": False},
    "deny": {"wired": False},
    "handoff": {"wired": False},
    "focus": {"wired": True, "mode": "inflection"},  # operator-attested frontdoor primitive (reins-local, not spine dispatch)
    "breakglass": {"wired": True, "mode": "inflection"},  # operator-attested sanctioned-exit witness (reins-local)
    "resume": {"wired": True, "mode": "preview"},  # read-only preview wedge (no spine write)
    # governed generation staging (U6b-stage): verifies a target generation against the store's
    # byte-binding + witnesses the attempt; it does NOT flip the current pointer (staging != swapping —
    # the swap is U6b-swap). Wired-before-swap is safe: staging is inert until a swap mechanism exists.
    "stage": {"wired": True, "mode": "governed"},
}


def serving_version() -> str:
    """The ONE semver source. REINS_VERSION env (a deployed generation can stamp it at stage time, like
    REINS_SERVING_SHA) wins; else the repo VERSION file (api/../VERSION — present on a source/kit install);
    else an honest "dev". Rides /read/meta so the cockpit detects a binary/API SKEW (the two halves ship as
    one versioned pair). "dev" never trips a false GEN-SKEW — the cockpit excludes it."""
    env = os.environ.get("REINS_VERSION", "").strip()
    if env:
        return env
    try:
        here = os.path.dirname(os.path.abspath(__file__))
        with open(os.path.join(here, "..", "VERSION"), encoding="utf-8") as f:
            v = f.read().strip()
        return v or "dev"
    except Exception:
        return "dev"


def serving_sha() -> str:
    """The generation identity: REINS_SERVING_SHA when deployed from a generation
    store (U6), else the live git sha, else an HONEST "unknown" — never fabricated."""
    env = os.environ.get("REINS_SERVING_SHA", "").strip()
    if env:
        return env
    try:
        repo = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        out = subprocess.run(
            ["git", "-C", repo, "rev-parse", "HEAD"],
            capture_output=True, text=True, timeout=5,
        )
        sha = out.stdout.strip()
        return sha if out.returncode == 0 and sha else "unknown"
    except Exception:
        return "unknown"


def _framed(h, b: bytes) -> None:
    # 8-byte big-endian length prefix + bytes — domain separation so adjacent name/content fields
    # cannot collide across a boundary. MUST match internal/generation.writeFramed (Go), so the
    # cockpit's byte-binding and this witness compute the same construction (GEN-SKEW, U6b).
    h.update(len(b).to_bytes(8, "big"))
    h.update(b)


def api_tree_sha() -> str:
    """Byte-hash of the served api/*.py tree — the staleness witness. Canonical set = {top-level *.py}
    (non-recursive; __pycache__/.pyc/lockfiles excluded), length-framed, sorted by name. This is the
    :16 PREFIX of the same full digest internal/generation.APITreeHash stores in a generation manifest,
    so GEN-SKEW can compare server[:16] == manifest.api_tree_sha256[:16]."""
    try:
        api_dir = os.path.dirname(os.path.abspath(__file__))
        h = hashlib.sha256()
        for name in sorted(os.listdir(api_dir)):
            if not name.endswith(".py"):
                continue
            with open(os.path.join(api_dir, name), "rb") as f:
                _framed(h, name.encode())
                _framed(h, f.read())
        return h.hexdigest()[:16]
    except Exception:
        return "unknown"


def _resume_preview_closures():
    """The Inc-1 preview wedge closures (mirrors reins_command.resume_preview_app —
    same zero-mint contract, hosted in the verb table instead of one-app-per-verb)."""

    def verify(packet: Any, target: str) -> bool:
        return bool(packet) and bool(target)

    def preflight(env: reins_command.Envelope) -> bool:
        return not env.preflight_receipt.get("blocked")

    def transport(env: reins_command.Envelope) -> reins_command.Response:
        return reins_command.Response(
            status="ok",
            http=200,
            receipt_id=f"preview-{env.idempotency_key}",
            event_seq=None,  # no real spine write
            fold_delta=(
                f"would emit session.resume({env.target}) via the governed COMMAND "
                "surface — preview (no-op transport; the real rail lands at U3+)"
            ),
            spooled=False,
        )

    return verify, preflight, transport


def _stage_closures():
    """The governed `stage` verb (U6b-stage). verify: the authority packet + target sha must be present.
    preflight: not explicitly blocked. transport: VALIDATE the target generation against the store's
    byte-binding (reins_generation.verify_generation — the Go Verify contract) and record a stage receipt;
    a missing/tampered/quarantined generation returns a TYPED stage-rejected (422), never a blind ok.
    NO pointer flip — verify_generation is read-only; the swap is U6b-swap."""

    def verify(packet: Any, target: str) -> bool:
        return bool(packet) and bool(target)

    def preflight(env: reins_command.Envelope) -> bool:
        return not env.preflight_receipt.get("blocked")

    def transport(env: reins_command.Envelope) -> reins_command.Response:
        ok, reason = reins_generation.verify_generation(env.target)
        if not ok:
            return reins_command.Response(status="stage-rejected", http=422, reason=reason)
        return reins_command.Response(
            status="ok",
            http=200,
            receipt_id=f"stage-{env.target}",
            event_seq=None,  # no spine write; the swap receipt genus (reins.swap.*) completes at U11
            fold_delta=f"generation {env.target} staged + verified (ready to swap; no pointer flip)",
            spooled=False,
        )

    return verify, preflight, transport


def _focus_closures():
    """The operator-attested `focus` verb (convergence rung 2). The operator's focus-inflection — which
    task/session the operator is attending to — recorded as a WITNESSED reins primitive the ROUTE + the
    spine consume for prioritization. verify: an operator_attestation packet + a target. preflight: not
    blocked. transport: the witnessed demand->verdict row IS the durable inflection (no separate store; no
    spine write; reins mints nothing), so the transport just confirms it. This is NOT spine dispatch —
    focus originates in the reins frontdoor (per the convergence cleave); the spine only CONSUMES it."""

    def verify(packet: Any, target: str) -> bool:
        # the attestation must SHAPE-match the ruling it claims — a bare-truthy packet from any same-UID
        # process must not mint a durable 'operator' focus. The recorded attestation then means what it says.
        return _is_operator_attestation(packet) and bool(target)

    def preflight(env: reins_command.Envelope) -> bool:
        return not env.preflight_receipt.get("blocked")

    def transport(env: reins_command.Envelope) -> reins_command.Response:
        return reins_command.Response(
            status="ok",
            http=200,
            receipt_id=f"focus-{env.idempotency_key}",
            event_seq=None,  # no spine write — the ledger witness IS the record
            fold_delta=(
                f"operator focus-inflection on {env.target} — RECORDED for the spine to consume "
                "(consumer pending: the seam contract is open; no dispatch, no spine write)"
            ),
            spooled=False,
        )

    return verify, preflight, transport


def _is_operator_attestation(packet: Any) -> bool:
    """Shape-check an operator_attestation packet (kind + the registered ruling id). This is NOT a
    cryptographic proof — loopback presence remains the trust root (A3.3) — but it ensures the WITNESSED
    attestation is what it claims, not an arbitrary truthy object. A stronger nonce/TTL lands before any
    spine consumer reads focus rows or any WRITE verb (arm/close/dispatch) wires (U7)."""
    return (
        isinstance(packet, dict)
        and packet.get("kind") == "operator_attestation"
        and str(packet.get("ruling", "")).startswith("RULING-REINS-OPERATOR-ATTESTATION")
    )


def _breakglass_closures():
    """The operator-attested `breakglass` verb — the ONE sanctioned exit from the frontdoor ("outside-Reins
    dispatch → breakglass-only"). It WITNESSES that the operator is deliberately engaging the n-DLC outside
    reins, with a reason; it does NOT itself dispatch or grant anything (that is the spine's escape-control
    tree). The witnessed record is the value: it builds the empirical ledger of what-is-done-outside-reins,
    which feeds the sole-frontdoor cutover. authority = operator_attestation; reins mints nothing; no spine
    write. verify: attestation + a non-empty reason (the exit must be justified, never a bare breakglass)."""

    def verify(packet: Any, target: str) -> bool:
        return _is_operator_attestation(packet) and bool(str(target).strip())

    def preflight(env: reins_command.Envelope) -> bool:
        return not env.preflight_receipt.get("blocked")

    def transport(env: reins_command.Envelope) -> reins_command.Response:
        return reins_command.Response(
            status="ok",
            http=200,
            receipt_id=f"breakglass-{env.idempotency_key}",
            event_seq=None,  # no spine write — the ledger witness IS the sanctioned-exit record
            fold_delta=(
                f"operator BREAKGLASS — sanctioned outside-frontdoor engagement: {env.target} "
                "(witnessed; the frontdoor records the exit + reason; reins dispatches nothing)"
            ),
            spooled=False,
        )

    return verify, preflight, transport


def _mount_command_router(app: FastAPI, ledger: reins_ledger.CommandLedger) -> None:
    """The verb-table router. Raises on construction failure — the caller degrades
    to read-only and discloses (never a half-mounted write surface).

    Every attempt is WITNESSED through the durable ledger (design pack A3.2): a DEMAND row
    is recorded BEFORE the wired-gate (so even a gated/unregistered attempt leaves a
    demand+verdict pair), and a VERDICT row records the outcome. Idempotency is the ledger's
    durable seen-set — the in-memory map is retired (a replayed key across a restart is a
    duplicate, not a re-invoke). /read/commands projects these rows, so the frontdoor is
    witnessed in the RUNNING server, not just the module."""
    # per-verb transports: a wired verb without a bound transport is an HONEST 501 (never a blind pass).
    closures_by_verb = {
        "resume": _resume_preview_closures(),
        "stage": _stage_closures(),
        "focus": _focus_closures(),
        "breakglass": _breakglass_closures(),
    }

    @app.post("/command/{verb}")
    def command(verb: str, req: reins_command.CommandRequest) -> JSONResponse:
        # 1. WITNESS the demand first — durable + idempotent, for every verb (A3.2).
        demand = ledger.record_demand(
            verb, req.target, req.idempotency_key,
            reins_ledger.CommandRefs(command_id="demand-" + verb),
        )
        eid = demand["event_id"]
        if demand.get("duplicate"):
            # a duplicate ONLY occurs for a command that already reached terminal SUCCESS (the ledger
            # dedups on success, not on demand). Replay the ORIGINAL success http; do NOT append a new
            # verdict — the original `ok` verdict is the truth, and a fresh 'idempotent-replay' verdict
            # would overwrite it under the projection's last-verdict-wins fold.
            http = int(demand.get("prior_http", 200))
            return JSONResponse(
                {"status": "idempotent-replay", "http": http, "duplicate": True,
                 "event_id": eid, "reason": "replayed idempotency_key — prior success, not re-executed"},
                status_code=http,
            )

        spec = VERB_TABLE.get(verb)
        if spec is None:
            ledger.record_verdict(eid, "unregistered", 501, f"unregistered verb: {verb}")
            return JSONResponse(
                {"status": "not-implemented", "http": 501, "wired": False, "event_id": eid,
                 "reason": f"unregistered verb: {verb}"},
                status_code=501,
            )
        if not spec.get("wired"):
            reason = (f"{verb} is declared but not wired — the governed rail "
                      "(demand receipt → verify → preflight → transport → witness) "
                      "arms at U7; no ungated path exists")
            ledger.record_verdict(eid, "not-wired", 501, reason)
            return JSONResponse(
                {"status": "not-wired", "http": 501, "wired": False, "event_id": eid,
                 "reason": reason},
                status_code=501,
            )

        cl = closures_by_verb.get(verb)
        if cl is None:
            # wired in the table but no transport bound — disclose, never blind-pass a write surface.
            reason = f"{verb} is wired but has no bound transport (router misconfig)"
            ledger.record_verdict(eid, "no-transport", 501, reason)
            return JSONResponse(
                {"status": "no-transport", "http": 501, "wired": True, "event_id": eid, "reason": reason},
                status_code=501,
            )
        verify, preflight, transport = cl

        envelope = reins_command.Envelope(
            verb=verb,
            target=req.target,
            authority_packet=req.authority_packet,
            preflight_receipt=req.preflight_receipt,
            idempotency_key=req.idempotency_key,
        )
        # the ledger already deduped the demand, so route_command sees a fresh emitted map. A RAISING
        # verify/preflight/transport must still leave a VERDICT — otherwise the demand strands as an
        # unresolved 'pending' forever (witness-every-attempt, A3.2). Record transport-error + refuse typed.
        try:
            resp = reins_command.route_command(
                envelope,
                verify_authority=verify,
                preflight=preflight,
                transport=transport,
                already_emitted={},
            )
        except Exception as e:  # the transport blew up — witness the failure, never leave it pending
            ledger.record_verdict(eid, "transport-error", 500, repr(e))
            return JSONResponse(
                {"status": "transport-error", "http": 500, "wired": True, "event_id": eid,
                 "reason": f"{verb} transport raised: {e!r} (witnessed; nothing applied)"},
                status_code=500,
            )
        ledger.record_verdict(
            eid, resp.status, resp.http, resp.reason or "",
            # record the APPLIED effect on success — what the transport produced (receipt/effect), so the
            # signed ledger carries preview→gate→APPLY, not just the gate verdict. Nothing applied on a
            # non-ok verdict (an honest empty effect, never fabricated).
            effect=(
                {
                    "receipt_id": resp.receipt_id,
                    "event_seq": resp.event_seq,
                    "fold_delta": resp.fold_delta,
                    "spooled": resp.spooled,
                }
                if resp.status == "ok"
                else None
            ),
        )
        out = reins_command._resp_to_dict(resp)
        out["event_id"] = eid
        return JSONResponse(out, status_code=resp.http)


def build_serve_app(council_root: str, allowlist: list[str], session_cfg: dict | None = None) -> FastAPI:
    app = reins_read.build_app(council_root, allowlist, session_cfg)

    # the durable command ledger IS the frontdoor's externalized state (A3.9). Its CONSTRUCTION is inside
    # the degrade path: a ledger that cannot load (e.g. a tampered/garbage JSONL line the reload chokes
    # on) must degrade the router to read-only and DISCLOSE it via /read/meta — never brick the whole
    # composed app at startup (which would crash-loop reins-api and dark the entire read model).
    router_state = "mounted"
    try:
        ledger = reins_ledger.CommandLedger(reins_ledger.ledger_path(), clock=reins_ledger.iso_utc_now)
        _mount_command_router(app, ledger)
    except Exception as e:  # degrade to read-only, disclosed — never dark
        router_state = f"degraded:{e}"

    @app.get("/read/commands")
    def read_commands(limit: int = 80) -> dict:
        # the witnessed-frontdoor projection (U3): demand+verdict datoms, honest witness state,
        # `absent` enforcement (the dispatch-gate does not exist until U13/CP-E). AIR default-deny.
        return reins_ledger.read_commands(None, allowlist, limit)

    @app.get("/route/posture")
    def route_posture() -> dict:
        # ROUTE projection (U4): NO SPINE DECISION ON FILE — reins mints no routing decision; honest
        # keyspace coverage + source states + the reqvec 0..5 producer contract.
        return reins_route.read_route_posture(None)

    @app.get("/route/candidates")
    def route_candidates() -> dict:
        # measured DEMAND evidence per routing_class (raw reqvec, no scalar); task_reqvec absent;
        # candidate ranking is a spine decision (dark).
        return reins_route.read_route_candidates(None)

    sha = serving_sha()
    tree = api_tree_sha()
    ver = serving_version()

    @app.get("/read/meta")
    def read_meta() -> dict:
        return {
            "dark": False,
            "app": "reins",
            "version": ver,  # the ONE semver — the cockpit compares its compiled version to detect skew
            "serving_sha": sha,
            "api_tree_sha": tree,
            "router": router_state,
            # mode is projected so the cockpit can render a PREVIEW verb honestly (a preview-mode ok is
            # "would emit …", never "✓ applied") — never-false-green on the frontdoor.
            "verbs": {v: {"wired": bool(s.get("wired")), "mode": s.get("mode", "")}
                      for v, s in VERB_TABLE.items()},
        }

    return app


if __name__ == "__main__":  # pragma: no cover
    import uvicorn

    cfg = reins_read.instance_config()
    uvicorn.run(
        build_serve_app(cfg["council_root"], cfg["allowlist"], cfg),
        host="127.0.0.1",
        port=cfg["port"],
    )
