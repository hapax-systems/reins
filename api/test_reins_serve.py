"""U1 contract tests — the composed serving surface (reins_serve)."""

import pathlib

import reins_command
import reins_serve
from reins_serve import VERB_TABLE, build_serve_app


def _endpoint(app, path):
    return next(r.endpoint for r in app.routes if getattr(r, "path", "") == path)


def test_read_meta_identity_handshake():
    # the serving-identity handshake: a port is only trusted when it answers app:"reins"
    # with a serving sha — the 8799 foreign-server class becomes a rendered state.
    app = build_serve_app("", [])
    meta = _endpoint(app, "/read/meta")()
    assert meta["app"] == "reins"
    assert meta["dark"] is False
    assert meta["serving_sha"]  # git sha or the honest "unknown" — never absent
    assert meta["api_tree_sha"]
    assert meta["router"] == "mounted"
    assert set(meta["verbs"]) == set(VERB_TABLE)
    wired = [v for v, s in meta["verbs"].items() if s["wired"]]
    assert wired == ["resume"]  # day-1: ONLY the read-only preview is wired


def test_unwired_verb_refuses_typed_501():
    app = build_serve_app("", [])
    cmd = _endpoint(app, "/command/{verb}")
    req = reins_command.CommandRequest(
        target="t1", authority_packet={"kind": "x"}, preflight_receipt={}, idempotency_key="k1"
    )
    resp = cmd("dispatch", req)
    assert resp.status_code == 501
    body = resp.body.decode()
    assert "not-wired" in body and "no ungated path" in body


def test_unregistered_verb_refuses_typed_501():
    app = build_serve_app("", [])
    cmd = _endpoint(app, "/command/{verb}")
    req = reins_command.CommandRequest(
        target="t1", authority_packet={"kind": "x"}, preflight_receipt={}, idempotency_key="k2"
    )
    resp = cmd("rm-rf", req)
    assert resp.status_code == 501
    assert "unregistered verb" in resp.body.decode()


def test_resume_preview_wired_zero_mint():
    app = build_serve_app("", [])
    cmd = _endpoint(app, "/command/{verb}")
    req = reins_command.CommandRequest(
        target="lane-a", authority_packet={"kind": "escape-grant"},
        preflight_receipt={}, idempotency_key="k3",
    )
    resp = cmd("resume", req)
    assert resp.status_code == 200
    body = resp.body.decode()
    assert "would emit session.resume(lane-a)" in body  # preview, not a write
    assert '"event_seq":null' in body.replace(" ", "")  # NO spine write happened


def test_router_failure_degrades_to_read_only_disclosed(monkeypatch):
    def boom(app):
        raise RuntimeError("router-construction-failure")

    monkeypatch.setattr(reins_serve, "_mount_command_router", boom)
    app = build_serve_app("", [])
    meta = _endpoint(app, "/read/meta")()
    assert meta["router"].startswith("degraded:")  # disclosed, never dark
    assert all(getattr(r, "path", "") != "/command/{verb}" for r in app.routes)  # read-only


def test_import_graph_guard_no_mint_surface():
    # the serve/command modules must never IMPORT a mint/dispatch-authority surface —
    # authority is verify-already-minted envelopes only (design pack §Design 3). AST-level:
    # docstrings may (and do) NAME the forbidden surfaces to declare the incapacity.
    import ast

    forbidden = (
        "coord_event_log", "coord_writer", "coord_projection",
        "hapax_methodology_dispatch", "escape_grant_mint", "cc_claim",
    )
    api_dir = pathlib.Path(__file__).parent
    for mod in ("reins_serve.py", "reins_command.py", "reins_ledger.py"):
        tree = ast.parse((api_dir / mod).read_text())
        imported: list[str] = []
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                imported += [a.name for a in node.names]
            elif isinstance(node, ast.ImportFrom):
                imported.append(node.module or "")
                imported += [a.name for a in node.names]
        for name in imported:
            low = name.lower()
            for token in forbidden:
                assert token not in low, (
                    f"{mod} imports forbidden authority surface: {name}"
                )


def test_read_commands_mounted_and_absent_enforcement():
    app = build_serve_app("", [])
    meta = _endpoint(app, "/read/commands")
    proj = meta()  # empty ledger -> honest empty, enforcement absent (never dark)
    assert proj["dark"] is False
    assert proj["enforcement"] == "absent"
    assert isinstance(proj["commands"], list)
