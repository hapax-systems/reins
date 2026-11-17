"""hapax-secret --from-file: a file-shaped credential stored in one act, never echoed.

The getpass dialogue reads one line, so a multi-line service-account key would store its
first line and spill the rest into the shell (operator-now 20261117T192321Z, R-204).
"""

from __future__ import annotations

import json
from io import StringIO

import pytest

import hapax_secret
import reins_command
import reins_serve

# A non-secret, multi-line, service-account-shaped value. The canary must never surface.
CANARY = "canary-private-key-line-never-echoed"
VALUE = (
    "{\n"
    '  "type": "service_account",\n'
    '  "project_id": "example-project",\n'
    f'  "private_key": "-----BEGIN PRIVATE KEY-----\\n{CANARY}\\n-----END PRIVATE KEY-----\\n",\n'
    '  "client_email": "reviewer@example-project.iam.gserviceaccount.com"\n'
    "}\n"
).encode()


@pytest.fixture(autouse=True)
def store(tmp_path, monkeypatch):
    monkeypatch.setenv("REINS_SECRET_STORE", str(tmp_path / "secrets"))
    monkeypatch.setenv("REINS_COMMAND_LEDGER", str(tmp_path / "commands.jsonl"))
    monkeypatch.setenv("REINS_SECRETS_HOST_FILE", str(tmp_path / "secrets-host"))
    monkeypatch.setenv("HAPAX_SECRETS_HOST", "secrets-store.example")
    monkeypatch.delenv("HAPAX_SECRETS_FORCE_REMOTE", raising=False)
    monkeypatch.setattr(hapax_secret, "_notify_presence", lambda *_a: None)
    from k0.key_capture import FileStore

    filestore = FileStore(root=tmp_path / "secrets")
    filestore._key()
    return filestore


def _live_router_post():
    app = reins_serve.build_serve_app("", [])
    cmd = next(r.endpoint for r in app.routes if getattr(r, "path", "") == "/command/{verb}")

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

    return post


def test_a_multi_line_key_round_trips_byte_identical_and_is_never_echoed(tmp_path, store):
    key_file = tmp_path / "downloaded-key.json"
    key_file.write_bytes(VALUE)
    out, err = StringIO(), StringIO()

    rc = hapax_secret.run_put_from_file(
        "vertex/test-account-json", str(key_file), post=_live_router_post(), stdout=out, stderr=err
    )

    assert rc == 0, err.getvalue()
    assert store.get("vertex-test-account-json") == VALUE  # every line, byte for byte
    said = out.getvalue() + err.getvalue()
    assert CANARY not in said and "service_account" not in said
    assert f"stored vertex-test-account-json ({len(VALUE)} bytes, file via reins)" in said
    assert "delete it now" in said
    assert key_file.read_bytes() == VALUE  # the source is left for the operator
    assert CANARY not in (tmp_path / "commands.jsonl").read_text(encoding="utf-8")


@pytest.mark.parametrize(
    ("make", "rc", "reason"),
    [
        (lambda p: p, 2, "is not a regular file"),
        (lambda p: (p.write_bytes(b""), p)[1], 1, "empty secret refused"),
        (lambda p: (p.mkdir(), p)[1], 2, "is not a regular file"),
        (lambda p: (p.write_bytes(b"x" * 65537), p)[1], 2, "exceeds 65536 bytes"),
    ],
    ids=["missing", "empty", "directory", "oversize"],
)
def test_an_unusable_source_stores_nothing(tmp_path, store, make, rc, reason):
    source = make(tmp_path / "key.json")
    posted: list[bytes] = []
    out, err = StringIO(), StringIO()

    got = hapax_secret.run_put_from_file(
        "vertex/test", str(source), post=lambda _u, b: posted.append(b) or b"{}", stdout=out, stderr=err
    )

    assert got == rc and reason in err.getvalue()
    assert posted == [] and store.get("vertex-test") is None


def test_main_routes_from_file_and_refuses_off_the_store_host(tmp_path, monkeypatch, capsys):
    key_file = tmp_path / "key.json"
    key_file.write_bytes(VALUE)
    seen: list[tuple[str, str]] = []
    monkeypatch.setattr(hapax_secret, "run_put_from_file", lambda n, p: seen.append((n, p)) or 0)

    assert hapax_secret.main(["--from-file", "vertex/test", str(key_file)]) == 0
    assert seen == [("vertex/test", str(key_file))]

    def refuse_forward(*_args):
        pytest.fail("a local path must never be forwarded to another host")

    monkeypatch.setattr(hapax_secret.os, "execvp", refuse_forward)
    monkeypatch.setenv("HAPAX_SECRETS_FORCE_REMOTE", "1")
    assert hapax_secret.main(["--from-file", "vertex/test", str(key_file)]) == 2
    err = capsys.readouterr().err
    assert "runs only on the FileStore host (secrets-store.example)" in err
    assert seen == [("vertex/test", str(key_file))]  # no second put
    assert hapax_secret.main(["--from-file", "vertex/test"]) == 2
