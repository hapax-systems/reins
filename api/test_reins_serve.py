"""U1/U3 contract tests — the composed serving surface (reins_serve)."""

import pathlib

import pytest

import reins_command
import reins_serve
from reins_serve import VERB_TABLE, build_serve_app


@pytest.fixture(autouse=True)
def _isolated_ledger(tmp_path, monkeypatch):
    # every serve test that POSTs a command writes to an ISOLATED ledger, never the real
    # ~/.cache/hapax/reins/commands.jsonl (U3 wiring: the router now witnesses every attempt).
    monkeypatch.setenv("REINS_COMMAND_LEDGER", str(tmp_path / "commands.jsonl"))


def _endpoint(app, path):
    return next(r.endpoint for r in app.routes if getattr(r, "path", "") == path)


def test_api_tree_sha_framing_matches_go_generation_hash():
    # cross-language parity: reins_serve's framed construction must produce the SAME digest as the Go
    # internal/generation.APITreeHash for the same tree (that test pins the identical constant). If either
    # drifts, GEN-SKEW (U6b) false-positives. This is the shared byte-binding contract.
    import hashlib

    tree = {"reins_read.py": b"READ-BODY", "reins_serve.py": b"SERVE-BODY"}
    h = hashlib.sha256()
    for name in sorted(tree):
        reins_serve._framed(h, name.encode())
        reins_serve._framed(h, tree[name])
    full = h.hexdigest()
    assert full == "1cc45033fa146f2fbbef2a1bdb0d9e0f651e5a9949d63a6ea0bb1d77ebd9e540"
    assert full[:16] == "1cc45033fa146f2f"  # the :16 prefix api_tree_sha reports as the witness


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
    assert wired == ["resume", "stage"]  # resume preview (read-only) + governed generation staging (U6b)


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
    for mod in ("reins_serve.py", "reins_command.py", "reins_ledger.py", "reins_route.py", "reins_generation.py"):
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


def test_router_witnesses_every_attempt_in_the_ledger():
    # U3 wiring (design pack A3.2): a gated/unregistered command attempt STILL leaves a durable
    # demand+verdict pair, so /read/commands projects the running server's activity — the
    # frontdoor is witnessed in the live server, not just the module.
    app = build_serve_app("", ["verb", "status"])
    cmd = _endpoint(app, "/command/{verb}")
    req = reins_command.CommandRequest(
        target="lane-a", authority_packet={"k": 1}, preflight_receipt={}, idempotency_key="w1"
    )
    resp = cmd("dispatch", req)  # unwired -> 501, but witnessed
    assert resp.status_code == 501
    assert b"event_id" in resp.body

    proj = _endpoint(app, "/read/commands")()
    assert len(proj["commands"]) == 1
    row = proj["commands"][0]
    assert row["verb"] == "dispatch" and row["status"] == "not-wired"
    assert row["witness"] == "pending"  # spine echo arms at U7/SA-3


def test_router_durable_idempotency_replay_is_duplicate():
    app = build_serve_app("", [])
    cmd = _endpoint(app, "/command/{verb}")
    req = reins_command.CommandRequest(
        target="lane-b", authority_packet={"k": 1}, preflight_receipt={}, idempotency_key="dup-key"
    )
    first = cmd("dispatch", req)
    assert first.status_code == 501  # not-wired, but demand recorded
    replay = cmd("dispatch", req)  # same idempotency_key
    assert replay.status_code == 200
    assert b"idempotent-replay" in replay.body

    # exactly ONE command datom despite the replay (durable dedup, not a second demand).
    proj = _endpoint(app, "/read/commands")()
    assert len(proj["commands"]) == 1


def test_stage_verified_generation_is_witnessed_ok(tmp_path, monkeypatch):
    # U6b-stage: POST /command/stage with a VERIFIED target generation -> 200 ok + a stage receipt,
    # witnessed in the ledger (demand+verdict). NO current pointer is flipped (staging != swapping).
    import hashlib
    import json

    import reins_generation

    root = str(tmp_path / "genstore")
    monkeypatch.setenv("REINS_GENERATION_ROOT", root)
    sha = "genABC"
    d = pathlib.Path(root) / "generations" / sha
    (d / "api").mkdir(parents=True)
    (d / "reins").write_bytes(b"BIN")
    api = {"reins_serve.py": b"s", "reins_read.py": b"r"}
    for n, c in api.items():
        (d / "api" / n).write_bytes(c)
    h = hashlib.sha256()
    for n in sorted(api):
        reins_generation._framed(h, n.encode())
        reins_generation._framed(h, api[n])
    (d / "manifest.json").write_text(json.dumps({
        "sha": sha, "binary_sha256": hashlib.sha256(b"BIN").hexdigest(),
        "api_tree_sha256": h.hexdigest(), "created": "t", "prev": "",
    }))

    app = build_serve_app("", ["verb", "status"])
    cmd = _endpoint(app, "/command/{verb}")
    resp = cmd("stage", reins_command.CommandRequest(
        target=sha, authority_packet={"kind": "stage-authority"}, preflight_receipt={}, idempotency_key="s1"))
    assert resp.status_code == 200
    body = resp.body.decode()
    assert "staged + verified" in body and "no pointer flip" in body

    proj = _endpoint(app, "/read/commands")()
    row = next(c for c in proj["commands"] if c["verb"] == "stage")
    assert row["status"] == "ok"  # witnessed
    # no swap happened: the store's current pointer was never written.
    assert not (pathlib.Path(root) / "current").exists()


def test_stage_missing_generation_typed_refusal_witnessed(tmp_path, monkeypatch):
    monkeypatch.setenv("REINS_GENERATION_ROOT", str(tmp_path / "empty"))
    app = build_serve_app("", ["verb", "status"])
    cmd = _endpoint(app, "/command/{verb}")
    resp = cmd("stage", reins_command.CommandRequest(
        target="does-not-exist", authority_packet={"kind": "x"}, preflight_receipt={}, idempotency_key="s2"))
    assert resp.status_code == 422  # typed stage-rejected, not a blind ok, not a generic 502
    assert "absent" in resp.body.decode()
    proj = _endpoint(app, "/read/commands")()
    row = next(c for c in proj["commands"] if c["verb"] == "stage")
    assert row["status"] == "stage-rejected"  # the refusal is witnessed too


def test_resume_preview_is_witnessed_ok():
    app = build_serve_app("", ["verb", "status"])
    cmd = _endpoint(app, "/command/{verb}")
    req = reins_command.CommandRequest(
        target="lane-c", authority_packet={"kind": "escape-grant"},
        preflight_receipt={}, idempotency_key="rp1",
    )
    resp = cmd("resume", req)
    assert resp.status_code == 200 and b"would emit session.resume" in resp.body
    proj = _endpoint(app, "/read/commands")()
    assert proj["commands"][0]["verb"] == "resume" and proj["commands"][0]["status"] == "ok"
