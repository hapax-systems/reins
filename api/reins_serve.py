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
    "focus": {"wired": False},
    "breakglass": {"wired": False},
    "resume": {"wired": True, "mode": "preview"},  # read-only preview wedge (no spine write)
}


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


def _mount_command_router(app: FastAPI, ledger: reins_ledger.CommandLedger) -> None:
    """The verb-table router. Raises on construction failure — the caller degrades
    to read-only and discloses (never a half-mounted write surface).

    Every attempt is WITNESSED through the durable ledger (design pack A3.2): a DEMAND row
    is recorded BEFORE the wired-gate (so even a gated/unregistered attempt leaves a
    demand+verdict pair), and a VERDICT row records the outcome. Idempotency is the ledger's
    durable seen-set — the in-memory map is retired (a replayed key across a restart is a
    duplicate, not a re-invoke). /read/commands projects these rows, so the frontdoor is
    witnessed in the RUNNING server, not just the module."""
    verify, preflight, transport = _resume_preview_closures()

    @app.post("/command/{verb}")
    def command(verb: str, req: reins_command.CommandRequest) -> JSONResponse:
        # 1. WITNESS the demand first — durable + idempotent, for every verb (A3.2).
        demand = ledger.record_demand(
            verb, req.target, req.idempotency_key,
            reins_ledger.CommandRefs(command_id="demand-" + verb),
        )
        eid = demand["event_id"]
        if demand.get("duplicate"):
            ledger.record_verdict(eid, "idempotent-replay", 200)
            return JSONResponse(
                {"status": "idempotent-replay", "http": 200, "duplicate": True,
                 "event_id": eid, "reason": "replayed idempotency_key — durable dedup"},
                status_code=200,
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

        envelope = reins_command.Envelope(
            verb=verb,
            target=req.target,
            authority_packet=req.authority_packet,
            preflight_receipt=req.preflight_receipt,
            idempotency_key=req.idempotency_key,
        )
        # the ledger already deduped the demand, so route_command sees a fresh emitted map.
        resp = reins_command.route_command(
            envelope,
            verify_authority=verify,
            preflight=preflight,
            transport=transport,
            already_emitted={},
        )
        ledger.record_verdict(eid, resp.status, resp.http, resp.reason or "")
        out = reins_command._resp_to_dict(resp)
        out["event_id"] = eid
        return JSONResponse(out, status_code=resp.http)


def build_serve_app(council_root: str, allowlist: list[str], session_cfg: dict | None = None) -> FastAPI:
    app = reins_read.build_app(council_root, allowlist, session_cfg)

    # the durable command ledger IS the frontdoor's externalized state (A3.9): one instance shared
    # by the command router (writes demand/verdict) and /read/commands (reads the same file).
    ledger = reins_ledger.CommandLedger(reins_ledger.ledger_path(), clock=reins_ledger.iso_utc_now)

    router_state = "mounted"
    try:
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

    @app.get("/read/meta")
    def read_meta() -> dict:
        return {
            "dark": False,
            "app": "reins",
            "serving_sha": sha,
            "api_tree_sha": tree,
            "router": router_state,
            "verbs": {v: {"wired": bool(s.get("wired"))} for v, s in VERB_TABLE.items()},
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
