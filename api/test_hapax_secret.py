"""hapax-secret TTY put goes through reins; GET is FileStore; no argv/ledger values."""

from __future__ import annotations

import base64
import errno
import json
import os
import subprocess
import sys
import time
from io import StringIO
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock

import pytest

import hapax_secret
import reins_command
import reins_serve


@pytest.fixture(autouse=True)
def presence_processes(monkeypatch):
    """No test in this file can reach a live notification command by accident."""
    spawned, commands = [], []

    def spawn(argv, **kwargs):
        spawned.append((argv, kwargs))
        return SimpleNamespace(wait=lambda: pytest.fail("foreground waited for trigger"))

    def run(argv, **kwargs):
        commands.append((argv, kwargs))
        return SimpleNamespace(returncode=0)

    proxy = SimpleNamespace(
        Popen=spawn, run=run, DEVNULL=subprocess.DEVNULL,
        TimeoutExpired=subprocess.TimeoutExpired,
    )
    monkeypatch.setattr(hapax_secret, "subprocess", proxy, raising=False)
    return spawned, commands, proxy


@pytest.fixture(autouse=True)
def _isolated_store_and_ledger(tmp_path, monkeypatch):
    monkeypatch.setenv("REINS_SECRET_STORE", str(tmp_path / "secrets"))
    monkeypatch.setenv("REINS_COMMAND_LEDGER", str(tmp_path / "commands.jsonl"))
    monkeypatch.setenv("REINS_SECRETS_HOST_FILE", str(tmp_path / "secrets-host"))
    monkeypatch.setenv("HAPAX_SECRETS_HOST", "secrets-store.example")
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
        hapax_secret.require_loopback_url("http://secrets-store.tailf9491.ts.net:8799/command/secret")
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
    assert "HAPAX_SECRETS_HOST" in text
    assert "python3 -m hapax_secret" in text
    assert ".local/share/reins/current/api" in text
    assert 'HOST == -*' in text or 'HOST" == -*' in text


def test_ssh_argv_tty_only_for_put():
    put = hapax_secret.ssh_argv(tty=True, rest=[])
    get = hapax_secret.ssh_argv(tty=False, rest=["litellm/master-key"])
    assert put[:4] == ["ssh", "-o", "BatchMode=yes", "-o"]
    assert "-t" in put
    assert "--" in put
    assert put[put.index("--") + 1] == "secrets-store.example"
    assert "hapax-secret" in put[-1]
    assert "-t" not in get
    assert "litellm/master-key" in get[-1]


def test_secrets_host_requires_explicit_config(monkeypatch, tmp_path):
    monkeypatch.delenv("HAPAX_SECRETS_HOST", raising=False)
    monkeypatch.setenv("REINS_SECRETS_HOST_FILE", str(tmp_path / "missing-host"))
    with pytest.raises(ValueError, match="not configured"):
        hapax_secret.secrets_host()
    host_file = tmp_path / "configured-host"
    host_file.write_text("configured-store.example\n", encoding="utf-8")
    monkeypatch.setenv("REINS_SECRETS_HOST_FILE", str(host_file))
    assert hapax_secret.secrets_host() == "configured-store.example"


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


@pytest.fixture
def secret_router():
    app = reins_serve.build_serve_app("", [])
    command = next(r.endpoint for r in app.routes if getattr(r, "path", "") == "/command/{verb}")

    def post(_url, body):
        return bytes(command("secret", reins_command.CommandRequest(**json.loads(body))).body)

    return post


def _put(post, value="synthetic-private-canary", name="provider/key"):
    out, err = StringIO(), StringIO()
    rc = hapax_secret.run_put_dialogue(
        prompt=lambda _: name, get_secret=lambda _: value, post=post, stdout=out, stderr=err,
    )
    return rc, out.getvalue(), err.getvalue()


def _delete(post, name="provider/key", answer="yes"):
    out, err = StringIO(), StringIO()
    rc = hapax_secret.run_delete(
        name, confirm=lambda _: answer, post=post, stdout=out, stderr=err,
    )
    return rc, out.getvalue(), err.getvalue()


def test_presence_put_after_commit_no_payload(
    secret_router, _isolated_store_and_ledger, presence_processes, monkeypatch,
):
    spawned, commands, proxy = presence_processes
    spawn = proxy.Popen

    def after_commit(argv, **kwargs):
        assert _isolated_store_and_ledger.get("provider-key") == b"synthetic-private-canary"
        return spawn(argv, **kwargs)

    proxy.Popen = after_commit
    assert _put(secret_router) == (0, "stored provider-key (file via reins)\n", "")
    assert len(spawned) == 1
    argv, options = spawned[0]
    assert argv[:3] == [sys.executable, "-I", "-c"]
    assert argv[4:] == [str(Path(hapax_secret.__file__).resolve().parent)]
    assert options["start_new_session"] is True
    assert options["close_fds"] is True
    assert options["stdin"] == options["stdout"] == options["stderr"] == subprocess.DEVNULL
    hapax_secret._presence_worker()
    assert [a for a, _ in commands] == [
        ["systemctl", "--user", "start", "--no-block", "hapax-isam-totality-audit.service"]
    ]


def test_presence_delete_after_commit_exact_name_reason_and_order(
    secret_router, _isolated_store_and_ledger, presence_processes,
):
    spawned, commands, proxy = presence_processes
    _isolated_store_and_ledger.put("provider-key", b"synthetic-delete-canary")
    spawn = proxy.Popen

    def after_commit(argv, **kwargs):
        assert not _isolated_store_and_ledger.has("provider-key")
        return spawn(argv, **kwargs)

    proxy.Popen = after_commit
    assert _delete(secret_router) == (0, "deleted provider-key\n", "")
    assert len(spawned) == 1
    assert spawned[0][0][4:] == [str(Path(hapax_secret.__file__).resolve().parent), "provider-key"]
    hapax_secret._presence_worker("provider-key")
    assert [a for a, _ in commands] == [
        ["hapax-entitlement", "unregister", "provider-key", "--reason", "explicit_delete"],
        ["systemctl", "--user", "start", "--no-block", "hapax-isam-totality-audit.service"],
    ]


@pytest.mark.parametrize("deleted", [False, None, "true", 1, {}, []])
def test_presence_delete_requires_literal_true(deleted, presence_processes):
    result = {"status": "ok", "payload": {"deleted": deleted}}
    assert _delete(lambda *_: json.dumps(result).encode())[0] == 1
    assert presence_processes[0] == []


@pytest.mark.parametrize("body", [b'{}', b'[]', b'no-json', b'{"status":"refused"}'])
@pytest.mark.parametrize("operation", [_put, _delete])
def test_presence_refused_mutation_never_notifies(body, operation, presence_processes):
    assert operation(lambda *_: body)[0] == 2
    assert presence_processes[0] == []


def test_presence_aborted_or_missing_delete_never_notifies(secret_router, presence_processes):
    assert _delete(secret_router, answer="no")[0] == 1
    assert _delete(secret_router)[0] == 1
    assert presence_processes[0] == []


@pytest.mark.parametrize("value,name", [("", "key"), ("synthetic", "invalid name")])
def test_presence_invalid_put_never_notifies(value, name, secret_router, presence_processes):
    assert _put(secret_router, value=value, name=name)[0] != 0
    assert presence_processes[0] == []


@pytest.mark.parametrize("operation", [_put, _delete])
def test_presence_launch_failure_preserves_mutation(
    operation, secret_router, _isolated_store_and_ledger, presence_processes,
):
    _isolated_store_and_ledger.put("provider-key", b"synthetic-before")
    attempts = []

    def unavailable(*args, **kwargs):
        attempts.append((args, kwargs))
        raise OSError("synthetic-exception-must-not-be-printed")

    presence_processes[2].Popen = unavailable
    rc, out, err = operation(secret_router)
    assert rc == 0 and err == ""
    assert "synthetic-exception" not in out
    assert len(attempts) == 1
    assert _isolated_store_and_ledger.has("provider-key") is (operation is _put)


@pytest.mark.parametrize("failure", ["missing", "timeout", "nonzero"])
def test_presence_worker_failure_is_bounded_and_never_retried(failure, presence_processes):
    commands = []

    def fail(argv, **kwargs):
        commands.append((argv, kwargs))
        if failure == "missing":
            raise FileNotFoundError("synthetic")
        if failure == "timeout":
            raise subprocess.TimeoutExpired(argv, kwargs["timeout"])
        return SimpleNamespace(returncode=3)

    presence_processes[2].run = fail
    hapax_secret._presence_worker("provider-key")
    assert [a[0] for a, _ in commands] == ["hapax-entitlement", "systemctl"]
    for _, options in commands:
        assert 0 < options["timeout"] <= 5
        assert options["check"] is False
        assert options["stdin"] == options["stdout"] == options["stderr"] == subprocess.DEVNULL


def test_presence_handoff_cannot_serialize_value_or_inherit_credentials(
    secret_router, presence_processes, monkeypatch,
):
    spawned = presence_processes[0]
    monkeypatch.setenv("SYNTHETIC_CREDENTIAL", "synthetic-env-canary")
    monkeypatch.setenv("PYTHONPATH", "synthetic-pythonpath-canary")
    for value in ("synthetic-a", "synthetic-value-of-a-different-length"):
        assert _put(secret_router, value=value)[0] == 0
    assert len(spawned) == 2
    assert spawned[0] == spawned[1]  # Includes argv, environment, stdio and process flags.
    env = spawned[0][1]["env"]
    allowed = {
        "HOME", "PATH", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "XDG_CONFIG_HOME",
        "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "PERSONAL_VAULT_PATH",
        "HAPAX_CC_TASKS_ROOT",
    }
    assert env == {k: v for k, v in os.environ.items() if k in allowed}
    assert "synthetic" not in str(spawned[0][0])
    assert "SYNTHETIC_CREDENTIAL" not in env and "PYTHONPATH" not in env


def test_presence_read_and_forward_paths_never_notify(
    secret_router, _isolated_store_and_ledger, presence_processes, monkeypatch, capsys,
):
    _isolated_store_and_ledger.put("provider-key", b"synthetic-read-canary")
    for args in (["provider-key"], ["--where", "provider-key"], ["--list"]):
        assert hapax_secret.main(args) == 0
    monkeypatch.setenv("HAPAX_SECRETS_FORCE_REMOTE", "1")

    def forwarded(*_):
        raise SystemExit(0)

    monkeypatch.setattr(os, "execvp", forwarded)
    with pytest.raises(SystemExit):
        hapax_secret.main([])
    assert presence_processes[0] == []


@pytest.mark.parametrize("delete", [False, True])
def test_presence_real_detached_worker_with_declared_stub_commands(
    delete, tmp_path, monkeypatch, presence_processes, secret_router, _isolated_store_and_ledger,
):
    """Real exec/worker/argv: only synthetic PATH commands, never live services or ledgers."""
    bindir = tmp_path / "bin"
    bindir.mkdir()
    script = f"#!{sys.executable}\n" + '''import json, os, pathlib, sys, time
home = pathlib.Path(os.environ["HOME"])
name = pathlib.Path(sys.argv[0]).name
if name == "hapax-entitlement":
    (home / "unregister-entered").touch()
    deadline = time.monotonic() + 4
    while not (home / "release").exists() and time.monotonic() < deadline:
        time.sleep(.01)
with (home / "calls").open("a") as f:
    f.write(json.dumps([name, *sys.argv[1:]]) + "\\n")
'''
    for command in ("hapax-entitlement", "systemctl"):
        p = bindir / command
        p.write_text(script)
        p.chmod(0o700)
    monkeypatch.setenv("PATH", str(bindir))
    monkeypatch.setenv("HOME", str(tmp_path))
    children = []

    def spawn(argv, **kwargs):
        child = subprocess.Popen(argv, **kwargs)
        children.append(child)
        return child

    presence_processes[2].Popen = spawn
    _isolated_store_and_ledger.put("provider-key", b"synthetic-before")
    start = time.monotonic()
    assert (_delete if delete else _put)(secret_router)[0] == 0
    assert time.monotonic() - start < 1
    assert len(children) == 1
    child = children[0]
    try:
        if delete:
            deadline = time.monotonic() + 3
            while not (tmp_path / "unregister-entered").exists() and time.monotonic() < deadline:
                time.sleep(.01)
            assert (tmp_path / "unregister-entered").exists()
            assert os.getsid(child.pid) == child.pid
            assert not (tmp_path / "calls").exists()  # No start before unregister finishes.
            (tmp_path / "release").touch()
        assert child.wait(timeout=10) == 0
        calls = [json.loads(line) for line in (tmp_path / "calls").read_text().splitlines()]
        expected = [["systemctl", "--user", "start", "--no-block", "hapax-isam-totality-audit.service"]]
        if delete:
            expected.insert(0, ["hapax-entitlement", "unregister", "provider-key", "--reason", "explicit_delete"])
        assert calls == expected
    finally:
        (tmp_path / "release").touch()
        child.wait(timeout=12)
