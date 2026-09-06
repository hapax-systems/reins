"""hapax-secret TTY put goes through reins; GET is FileStore; no argv/ledger values."""

from __future__ import annotations

import base64
import errno
import json
import os
import subprocess
from io import StringIO
from pathlib import Path
from unittest.mock import Mock

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

    store = FileStore(root=tmp_path / "secrets")
    store._key()
    return store


def test_name_of_maps_slash_and_rejects_illegal():
    assert hapax_secret.name_of("hapax-reviewer/org") == "hapax-reviewer-org"
    assert hapax_secret.name_of("litellm/master-key") == "litellm-master-key"
    with pytest.raises(ValueError, match="empty"):
        hapax_secret.name_of("  ")
    with pytest.raises(ValueError, match=r"\[A-Za-z0-9"):
        hapax_secret.name_of("has space")


def test_require_loopback_refuses_tailnet():
    with pytest.raises(ValueError, match="loopback"):
        hapax_secret.require_loopback_url("http://secrets.example.ts.net:8799/command/secret")
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
    assert "secrets-store" in text
    assert "HAPAX_SECRETS_HOST" in text
    assert "python3 -m hapax_secret" in text
    assert ".local/share/reins/current/api" in text
    assert 'HOST == -*' in text or 'HOST" == -*' in text


def test_ssh_argv_tty_for_put_not_get(monkeypatch):
    monkeypatch.delenv("HAPAX_SECRETS_HOST", raising=False)
    put = hapax_secret.ssh_argv(tty=True, rest=[])
    get = hapax_secret.ssh_argv(tty=False, rest=["litellm/master-key"])
    assert put[:4] == ["ssh", "-o", "BatchMode=yes", "-o"]
    assert "-t" in put
    assert "--" in put
    assert put[put.index("--") + 1] == "secrets-store"
    assert "hapax-secret" in put[-1]
    assert "-t" not in get
    assert get[get.index("--") + 1] == "secrets-store"
    assert "litellm/master-key" in get[-1]


def test_module_docstring_names_ssh_alias_and_override():
    assert "secrets-store" in hapax_secret.__doc__
    assert "HAPAX_SECRETS_HOST" in hapax_secret.__doc__


def _launcher_text():
    return Path(__file__).resolve().parent.parent.joinpath("scripts/hapax-secret").read_text()


def _launcher_setup():
    # Evaluate only host selection, its refusal, and the remote builder. Never
    # source the wrapper: that would inspect the store or execute ssh/Python.
    text = _launcher_text()
    start = text.index("\nHOST=") + 1
    end = text.index('\nif [[ "${HAPAX_SECRETS_FORCE_REMOTE:-}"')
    return text[start:end]


def _run_bash(script, rest, home, **env):
    return subprocess.run(
        ["/bin/bash", "--noprofile", "--norc", "-c", "set -euo pipefail\n" + script, "--", *rest],
        env={"HOME": str(home), "PATH": os.defpath, "LC_ALL": "C", **env},
        cwd=home,
        capture_output=True,
        text=True,
        timeout=5,
    )


@pytest.mark.parametrize(
    ("override", "expected"),
    [
        (None, "secrets-store"),
        ("", "secrets-store"),
        ("   ", "secrets-store"),
        (" \t\n\r\v\f ", "secrets-store"),
        ("secrets.example.internal", "secrets.example.internal"),
        (" \tsecrets.example.internal\n ", "secrets.example.internal"),
    ],
    ids=["unset", "empty", "whitespace", "mixed-whitespace", "override", "padded-override"],
)
def test_launcher_host_matches_python(monkeypatch, tmp_path, override, expected):
    env = {}
    if override is None:
        monkeypatch.delenv("HAPAX_SECRETS_HOST", raising=False)
    else:
        monkeypatch.setenv("HAPAX_SECRETS_HOST", override)
        env["HAPAX_SECRETS_HOST"] = override
    host_lines = "\n".join(line for line in _launcher_text().splitlines() if line.startswith("HOST="))
    result = _run_bash(host_lines + '\nprintf %s "$HOST"', [], tmp_path, **env)
    assert result.returncode == 0
    assert result.stdout == expected
    assert hapax_secret.secrets_host() == expected


_FORWARDING_CASES = [
    pytest.param(False, ["--where", "name"], "remote", id="where"),
    pytest.param(
        False,
        [
            "--where",
            "name; touch NOT_ALLOWED",
            "'\"$HOME $(touch NOT_ALLOWED) `touch NOT_ALLOWED` \\ * ? [x] & | < > ( ) # ~",
            "",
            "tab\tvalue\nnext line",
        ],
        "remote",
        id="metacharacters",
    ),
    pytest.param(False, ["--where", "name"], "remote home with spaces", id="home-whitespace"),
    pytest.param(True, [], "remote", id="put-no-args"),
    pytest.param(True, ["--delete", "name"], "remote", id="delete"),
]


def _assert_remote_execution(remote, rest, home):
    helper = home / ".local" / "bin" / "hapax-secret"
    helper.parent.mkdir(parents=True)
    helper.write_text('#!/bin/sh\nfor arg do\n  printf \'%s\\n\' "$arg"\ndone\n')
    helper.chmod(0o700)
    result = subprocess.run(
        ["/bin/sh", "-c", remote],
        env={"HOME": str(home), "PATH": os.defpath},
        cwd=home,
        capture_output=True,
        text=True,
        timeout=5,
    )
    assert not (home / "NOT_ALLOWED").exists(), "forwarded argument executed a command"
    assert result.returncode == 0, f"remote helper exited {result.returncode}"
    assert result.stdout == "".join(arg + "\n" for arg in rest)
    assert result.stderr == ""
    if not rest:
        assert remote == '"$HOME/.local/bin/hapax-secret"'


@pytest.mark.parametrize(("tty", "rest", "home_name"), _FORWARDING_CASES)
def test_ssh_argv_remote_execution(monkeypatch, tmp_path, tty, rest, home_name):
    monkeypatch.delenv("HAPAX_SECRETS_HOST", raising=False)
    argv = hapax_secret.ssh_argv(tty=tty, rest=rest)
    expected = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8"]
    if tty:
        expected.append("-t")
    assert argv[:-1] == expected + ["--", "secrets-store"]
    _assert_remote_execution(argv[-1], rest, tmp_path / home_name)


@pytest.mark.parametrize(("tty", "rest", "home_name"), _FORWARDING_CASES)
def test_launcher_remote_execution(tmp_path, tty, rest, home_name):
    text = _launcher_text()
    construction = next(
        line.strip() for line in text.splitlines() if line.startswith("  quoted=")
    )
    # Evaluate the actual TTY selection with a builtin argv recorder in place of
    # both exec statements. No ssh executable or store-host detection is run.
    start = text.index("  if [[ $#")
    branch = text[start:text.index("\nfi", start)]
    assert branch.count("exec ssh ") == 2
    branch = branch.replace("exec ssh ", "capture_argv ")
    script = "\n".join(
        [
            _launcher_setup(),
            construction,
            "capture_argv() { printf '%s\\0' \"$@\"; exit; }",
            branch,
        ]
    )
    result = _run_bash(script, rest, tmp_path)
    assert result.returncode == 0
    assert result.stdout.endswith("\0")
    argv = result.stdout[:-1].split("\0")
    expected = ["-o", "BatchMode=yes", "-o", "ConnectTimeout=8"]
    if tty:
        expected.append("-t")
    assert argv[:-1] == expected + ["--", "secrets-store"]
    _assert_remote_execution(argv[-1], rest, tmp_path / home_name)


@pytest.mark.parametrize(
    ("rest", "tty"),
    [([], True), (["--delete", "name"], True), (["--where", "name"], False)],
    ids=["put-no-args", "delete", "where"],
)
def test_main_remote_tty_selection(monkeypatch, rest, tty):
    monkeypatch.delenv("HAPAX_SECRETS_HOST", raising=False)
    monkeypatch.setattr(hapax_secret, "is_store_host", lambda: False)
    forward = Mock(side_effect=SystemExit(0))
    monkeypatch.setattr(hapax_secret.os, "execvp", forward)
    with pytest.raises(SystemExit):
        hapax_secret.main(rest)
    forward.assert_called_once_with("ssh", hapax_secret.ssh_argv(tty=tty, rest=rest))


@pytest.mark.parametrize("tty", [False, True], ids=["get", "put"])
def test_ssh_argv_uses_host_override(monkeypatch, tty):
    monkeypatch.setenv("HAPAX_SECRETS_HOST", "secrets.example.internal")
    argv = hapax_secret.ssh_argv(tty=tty, rest=[] if tty else ["litellm/master-key"])
    assert argv[argv.index("--") + 1] == "secrets.example.internal"
    assert ("-t" in argv) == tty


@pytest.mark.parametrize("host", ["", "  "], ids=["empty", "whitespace"])
def test_secrets_host_empty_override_uses_alias(monkeypatch, host):
    monkeypatch.setenv("HAPAX_SECRETS_HOST", host)
    assert hapax_secret.secrets_host() == "secrets-store"


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


_ENTRY_NAME = "synthetic-entry"
_ENTRY_VALUE = b"synthetic-value-canary"
_ERROR_DETAIL = "synthetic-error-detail"
_GET_MISSING = (
    b"not found in FileStore: synthetic-entry. "
    b"legal_next: run hapax-secret (TTY put) via reins.\n"
)


@pytest.fixture(params=[["synthetic/entry"], ["--where", "synthetic/entry"]], ids=["get", "where"])
def read_argv(request):
    return request.param


@pytest.fixture
def read_store(_isolated_store_and_ledger, monkeypatch):
    store = _isolated_store_and_ledger
    store.put(_ENTRY_NAME, _ENTRY_VALUE)
    monkeypatch.setattr(hapax_secret, "default_store", lambda: store)
    monkeypatch.setattr(store, "get", Mock(wraps=store.get))
    monkeypatch.setattr(store, "has", Mock(wraps=store.has))

    def refuse_remote(*_args):
        pytest.fail("entry observations must stay on the selected FileStore")

    monkeypatch.setattr(hapax_secret.os, "execvp", refuse_remote)
    return store


def _assert_no_read_leak(captured, store):
    for stream in (captured.out, captured.err):
        assert _ENTRY_VALUE not in stream
        assert str(store.root).encode() not in stream
        assert _ERROR_DETAIL.encode() not in stream


def _assert_missing(rc, captured, argv, store):
    assert rc == 1
    if argv[0] == "--where":
        assert captured.out == b"not found: synthetic-entry\n"
        assert captured.err == b""
    else:
        assert captured.out == b""
        assert captured.err == _GET_MISSING
    _assert_no_read_leak(captured, store)


def _assert_unreadable(rc, captured, store, exception_class="OSError"):
    assert rc == 2
    assert captured.out == b""
    assert captured.err == f"unreadable: synthetic-entry ({exception_class})\n".encode()
    _assert_no_read_leak(captured, store)


def test_main_read_missing(read_store, read_argv, capsysbinary):
    read_store._blob_path(_ENTRY_NAME).unlink()
    rc = hapax_secret.main(read_argv)
    _assert_missing(rc, capsysbinary.readouterr(), read_argv, read_store)
    read_store.get.assert_not_called()
    read_store.has.assert_not_called()


@pytest.mark.parametrize(
    ("error_number", "exception_class"),
    [(errno.EIO, "OSError"), (errno.EACCES, "PermissionError"), (errno.ELOOP, "OSError")],
    ids=["EIO", "EACCES", "ELOOP"],
)
def test_main_read_entry_stat_failure(
    read_store, read_argv, monkeypatch, capsysbinary, error_number, exception_class
):
    entry = read_store._blob_path(_ENTRY_NAME)
    original_stat = Path.stat
    observations = []

    def stat(path, *args, **kwargs):
        if path == entry:
            observations.append(path)
            raise OSError(error_number, _ERROR_DETAIL + _ENTRY_VALUE.decode(), str(entry))
        return original_stat(path, *args, **kwargs)

    monkeypatch.setattr(Path, "stat", stat)
    rc = hapax_secret.main(read_argv)
    _assert_unreadable(rc, capsysbinary.readouterr(), read_store, exception_class)
    assert observations == [entry]
    read_store.get.assert_not_called()
    read_store.has.assert_not_called()


def test_main_read_present_but_get_returns_none(read_store, read_argv, capsysbinary):
    read_store.get.return_value = None
    rc = hapax_secret.main(read_argv)
    _assert_unreadable(rc, capsysbinary.readouterr(), read_store)
    read_store.get.assert_called_once_with(_ENTRY_NAME)
    read_store.has.assert_not_called()


@pytest.mark.parametrize(
    ("error_number", "exception_class"),
    [
        (errno.EIO, "OSError"),
        (errno.EACCES, "PermissionError"),
        (errno.ELOOP, "OSError"),
        (errno.ENOENT, "FileNotFoundError"),
    ],
    ids=["EIO", "EACCES", "ELOOP", "ENOENT-after-stat"],
)
def test_main_read_entry_read_failure(
    read_store, read_argv, monkeypatch, capsysbinary, error_number, exception_class
):
    entry = read_store._blob_path(_ENTRY_NAME)
    original_read = Path.read_bytes
    reads = []

    def read_bytes(path):
        if path == entry:
            reads.append(path)
            raise OSError(error_number, _ERROR_DETAIL + _ENTRY_VALUE.decode(), str(entry))
        return original_read(path)

    monkeypatch.setattr(Path, "read_bytes", read_bytes)
    rc = hapax_secret.main(read_argv)
    _assert_unreadable(rc, capsysbinary.readouterr(), read_store, exception_class)
    assert reads == [entry]
    read_store.get.assert_called_once_with(_ENTRY_NAME)
    read_store.has.assert_not_called()


@pytest.mark.parametrize("corruption", ["truncated", "bad-mac", "directory"])
def test_main_read_corrupt_entry(read_store, read_argv, capsysbinary, corruption):
    entry = read_store._blob_path(_ENTRY_NAME)
    if corruption == "truncated":
        entry.write_bytes(b"synthetic-truncated-blob")
    elif corruption == "bad-mac":
        blob = entry.read_bytes()
        entry.write_bytes(blob[:-1] + bytes([blob[-1] ^ 1]))
    else:
        entry.unlink()
        entry.mkdir()
    rc = hapax_secret.main(read_argv)
    _assert_unreadable(rc, capsysbinary.readouterr(), read_store)
    read_store.get.assert_called_once_with(_ENTRY_NAME)
    read_store.has.assert_not_called()


@pytest.mark.parametrize("value", [b"", _ENTRY_VALUE, _ENTRY_VALUE + b"\n", b"\x00\xff"])
def test_main_read_success_bytes(read_store, read_argv, capsysbinary, value):
    read_store.put(_ENTRY_NAME, value)
    rc = hapax_secret.main(read_argv)
    captured = capsysbinary.readouterr()
    assert rc == 0
    assert captured.err == b""
    if read_argv[0] == "--where":
        assert captured.out == b"filestore\n"
        _assert_no_read_leak(captured, read_store)
    else:
        assert captured.out == value + (b"" if value.endswith(b"\n") else b"\n")
    read_store.get.assert_called_once_with(_ENTRY_NAME)
    read_store.has.assert_not_called()


def test_main_read_observes_presence_once(read_store, read_argv, monkeypatch, capsysbinary):
    # Isolate the CLI's observation from the unchanged get implementation's own stat.
    read_store.get.return_value = _ENTRY_VALUE
    entry = read_store._blob_path(_ENTRY_NAME)
    original_stat = Path.stat
    observations = []

    def stat(path, *args, **kwargs):
        if path == entry:
            observations.append(path)
        return original_stat(path, *args, **kwargs)

    monkeypatch.setattr(Path, "stat", stat)
    rc = hapax_secret.main(read_argv)
    captured = capsysbinary.readouterr()
    assert rc == 0
    assert captured.err == b""
    assert captured.out == (b"filestore\n" if read_argv[0] == "--where" else _ENTRY_VALUE + b"\n")
    assert observations == [entry]
    read_store.get.assert_called_once_with(_ENTRY_NAME)
    read_store.has.assert_not_called()


def test_main_where_then_get_entry_disappears(read_store, capsysbinary):
    rc = hapax_secret.main(["--where", "synthetic/entry"])
    captured = capsysbinary.readouterr()
    assert rc == 0
    assert captured.out == b"filestore\n"
    assert captured.err == b""
    _assert_no_read_leak(captured, read_store)
    read_store.get.assert_called_once_with(_ENTRY_NAME)

    read_store._blob_path(_ENTRY_NAME).unlink()
    read_store.get.reset_mock()
    argv = ["synthetic/entry"]
    rc = hapax_secret.main(argv)
    _assert_missing(rc, capsysbinary.readouterr(), argv, read_store)
    read_store.get.assert_not_called()
    read_store.has.assert_not_called()


def test_main_read_entry_disappears_after_stat(read_store, read_argv, capsysbinary):
    original_get = read_store.get._mock_wraps

    def get(name):
        read_store._blob_path(name).unlink()
        return original_get(name)

    read_store.get.side_effect = get
    rc = hapax_secret.main(read_argv)
    _assert_unreadable(rc, capsysbinary.readouterr(), read_store)
    read_store.get.assert_called_once_with(_ENTRY_NAME)
    read_store.has.assert_not_called()


def test_delete_aborts_without_yes():
    posts: list[tuple[str, bytes]] = []

    def post(url: str, body: bytes) -> bytes:
        posts.append((url, body))
        return b"{}"

    rc = hapax_secret.run_delete(
        "glmcp",
        confirm=lambda _: "n",
        post=post,
        stdout=StringIO(),
        stderr=StringIO(),
    )
    assert rc == 1
    assert posts == []


def test_delete_posts_reins_and_maps_slash_name():
    captured: dict[str, object] = {}

    def post(url: str, body: bytes) -> bytes:
        captured["url"] = url
        captured["body"] = json.loads(body)
        return json.dumps(
            {"status": "ok", "payload": {"deleted": True, "present": False, "name": "glmcp"}}
        ).encode()

    out, err = StringIO(), StringIO()
    rc = hapax_secret.run_delete(
        "glmcp",
        confirm=lambda _: "y",
        post=post,
        stdout=out,
        stderr=err,
    )
    assert rc == 0, err.getvalue()
    body = captured["body"]
    assert body["target"] == "glmcp"
    assert body["authority_packet"] == {"kind": "secret", "op": "delete"}
    assert "deleted glmcp" in out.getvalue()


def test_ssh_argv_tty_for_delete():
    argv = hapax_secret.ssh_argv(tty=True, rest=["--delete", "glmcp"])
    assert "-t" in argv
    assert "--" in argv
