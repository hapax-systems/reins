"""hapax-secret TTY put goes through reins; GET is FileStore; no argv/ledger values."""

from __future__ import annotations

import base64
import json
from io import StringIO
from pathlib import Path

import pytest

import hapax_secret
import reins_command
import reins_serve


@pytest.fixture(autouse=True)
def _isolated_store_and_ledger(tmp_path, monkeypatch):
    monkeypatch.setenv("REINS_SECRET_STORE", str(tmp_path / "secrets"))
    monkeypatch.setenv("REINS_COMMAND_LEDGER", str(tmp_path / "commands.jsonl"))
    monkeypatch.delenv("HAPAX_SECRETS_FORCE_REMOTE", raising=False)
    # Tests run the store-host path; create a FileStore key so is_store_host can be true.
    from k0.key_capture import FileStore

    FileStore(root=tmp_path / "secrets")._key()


def test_name_of_maps_slash_and_rejects_illegal():
    assert hapax_secret.name_of("hapax-reviewer/org") == "hapax-reviewer-org"
    assert hapax_secret.name_of("litellm/master-key") == "litellm-master-key"
    with pytest.raises(ValueError, match="empty"):
        hapax_secret.name_of("  ")
    with pytest.raises(ValueError, match=r"\[A-Za-z0-9"):
        hapax_secret.name_of("has space")


def test_require_loopback_refuses_tailnet():
    with pytest.raises(ValueError, match="loopback"):
        hapax_secret.require_loopback_url("http://hapax-appendix.tailf9491.ts.net:8799/command/secret")
    hapax_secret.require_loopback_url("http://127.0.0.1:8799/command/secret")


def test_put_dialogue_mismatch_does_not_post():
    posts: list[tuple[str, bytes]] = []

    def post(url: str, body: bytes) -> bytes:
        posts.append((url, body))
        return b"{}"

    rc = hapax_secret.run_put_dialogue(
        prompt=lambda _: "hapax-reviewer/org",
        get_secret=lambda p: "alpha" if p.startswith("Secret") else "beta",
        post=post,
        stdout=StringIO(),
        stderr=StringIO(),
    )
    assert rc == 1
    assert posts == []


def test_put_dialogue_empty_secret_does_not_post():
    posts: list[tuple[str, bytes]] = []

    def post(url: str, body: bytes) -> bytes:
        posts.append((url, body))
        return b"{}"

    rc = hapax_secret.run_put_dialogue(
        prompt=lambda _: "k",
        get_secret=lambda _: "",
        post=post,
        stdout=StringIO(),
        stderr=StringIO(),
    )
    assert rc == 1
    assert posts == []


def test_put_dialogue_posts_reins_packet_without_argv_shape(tmp_path, monkeypatch):
    monkeypatch.setenv("HAPAX_SECRET_COMMAND_URL", "http://127.0.0.1:8799/command/secret")
    captured: dict[str, object] = {}

    def post(url: str, body: bytes) -> bytes:
        captured["url"] = url
        captured["raw"] = body
        captured["body"] = json.loads(body)
        assert b"s3cret-canary" not in body  # plaintext never on the wire body; only value_b64
        return json.dumps(
            {
                "status": "ok",
                "http": 200,
                "payload": {"backend_id": "file", "op": "put", "name": "hapax-reviewer-org"},
            }
        ).encode()

    out, err = StringIO(), StringIO()
    rc = hapax_secret.run_put_dialogue(
        prompt=lambda _: "hapax-reviewer/org",
        get_secret=lambda _: "s3cret-canary",
        post=post,
        stdout=out,
        stderr=err,
    )
    assert rc == 0, err.getvalue()
    assert captured["url"] == "http://127.0.0.1:8799/command/secret"
    body = captured["body"]
    assert body["target"] == "hapax-reviewer-org"
    assert body["authority_packet"]["kind"] == "secret"
    assert body["authority_packet"]["op"] == "put"
    assert base64.b64decode(body["authority_packet"]["value_b64"]) == b"s3cret-canary"
    assert "s3cret-canary" not in out.getvalue()
    assert "stored hapax-reviewer-org" in out.getvalue()
    assert "file via reins" in out.getvalue()


def test_put_via_reins_live_router_never_ledgers_value(tmp_path, monkeypatch):
    app = reins_serve.build_serve_app("", [])
    cmd = next(r.endpoint for r in app.routes if getattr(r, "path", "") == "/command/{verb}")
    canary = "sk-dialogue-canary-never"

    def post(_url: str, body: bytes) -> bytes:
        req = json.loads(body)
        resp = cmd(
            "secret",
            reins_command.CommandRequest(
                target=req["target"],
                authority_packet=req["authority_packet"],
                preflight_receipt=req["preflight_receipt"],
                idempotency_key=req["idempotency_key"],
            ),
        )
        return bytes(resp.body)

    result = hapax_secret.put_via_reins("frontier-key", canary.encode(), post=post)
    assert result["status"] == "ok"
    assert result["payload"]["backend_id"] == "file"
    assert "value_b64" not in result["payload"]
    ledger = (tmp_path / "commands.jsonl").read_text(encoding="utf-8")
    assert canary not in ledger


def test_main_no_args_non_tty_refuses(monkeypatch):
    monkeypatch.setattr(hapax_secret.sys.stdin, "isatty", lambda: False)
    monkeypatch.setattr(hapax_secret.sys.stdout, "isatty", lambda: True)
    rc = hapax_secret.main([])
    assert rc == 2


def test_launcher_script_sshes_without_baked_home():
    text = Path(__file__).resolve().parent.parent.joinpath("scripts/hapax-secret").read_text()
    assert "ssh" in text
    assert " -t -- " in text or " -t --" in text
    assert "/home/" not in text
    assert "hapax-appendix" in text
    assert "python3 -m hapax_secret" in text
    assert 'HOST == -*' in text or 'HOST" == -*' in text


def test_ssh_argv_tty_only_for_put():
    put = hapax_secret.ssh_argv(tty=True, rest=[])
    get = hapax_secret.ssh_argv(tty=False, rest=["litellm/master-key"])
    assert put[:4] == ["ssh", "-o", "BatchMode=yes", "-o"]
    assert "-t" in put
    assert "--" in put
    assert put[put.index("--") + 1] == "hapax-appendix"
    assert "hapax-secret" in put[-1]
    assert "-t" not in get
    assert "litellm/master-key" in get[-1]


def test_ssh_argv_rejects_option_like_host(monkeypatch):
    monkeypatch.setenv("HAPAX_SECRETS_HOST", "-oProxyCommand=evil")
    with pytest.raises(ValueError, match="must not start"):
        hapax_secret.ssh_argv(tty=False, rest=["x"])


def test_main_get_roundtrip(tmp_path, monkeypatch, capsys):
    from k0.key_capture import default_store

    store = default_store()
    store.put("hapax-reviewer-org", b"from-store")
    rc = hapax_secret.main(["hapax-reviewer/org"])
    assert rc == 0
    assert capsys.readouterr().out == "from-store\n"
