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
import reins_read

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


def api_tree_sha() -> str:
    """Byte-hash of the served api/*.py tree — the staleness witness (a running
    server whose tree hash differs from disk is the 8780 class, rendered)."""
    try:
        api_dir = os.path.dirname(os.path.abspath(__file__))
        h = hashlib.sha256()
        for name in sorted(os.listdir(api_dir)):
            if not name.endswith(".py"):
                continue
            with open(os.path.join(api_dir, name), "rb") as f:
                h.update(name.encode())
                h.update(f.read())
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


def _mount_command_router(app: FastAPI) -> None:
    """The verb-table router. Raises on construction failure — the caller degrades
    to read-only and discloses (never a half-mounted write surface)."""
    verify, preflight, transport = _resume_preview_closures()
    emitted: dict[str, str] = {}

    @app.post("/command/{verb}")
    def command(verb: str, req: reins_command.CommandRequest) -> JSONResponse:
        spec = VERB_TABLE.get(verb)
        if spec is None:
            return JSONResponse(
                {"status": "not-implemented", "http": 501, "wired": False,
                 "reason": f"unregistered verb: {verb}"},
                status_code=501,
            )
        if not spec.get("wired"):
            return JSONResponse(
                {"status": "not-wired", "http": 501, "wired": False,
                 "reason": f"{verb} is declared but not wired — the governed rail "
                           "(demand receipt → verify → preflight → transport → witness) "
                           "lands at U3+; no ungated path exists"},
                status_code=501,
            )
        envelope = reins_command.Envelope(
            verb=verb,
            target=req.target,
            authority_packet=req.authority_packet,
            preflight_receipt=req.preflight_receipt,
            idempotency_key=req.idempotency_key,
        )
        resp = reins_command.route_command(
            envelope,
            verify_authority=verify,
            preflight=preflight,
            transport=transport,
            already_emitted=emitted,
        )
        if resp.status == "ok" and resp.receipt_id:
            emitted[envelope.idempotency_key] = resp.receipt_id
        return JSONResponse(reins_command._resp_to_dict(resp), status_code=resp.http)


def build_serve_app(council_root: str, allowlist: list[str], session_cfg: dict | None = None) -> FastAPI:
    app = reins_read.build_app(council_root, allowlist, session_cfg)

    router_state = "mounted"
    try:
        _mount_command_router(app)
    except Exception as e:  # degrade to read-only, disclosed — never dark
        router_state = f"degraded:{e}"

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
