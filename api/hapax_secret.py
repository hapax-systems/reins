"""hapax-secret — operator ritual for estate secrets via reins.

GET (watchdogs/scripts): ``hapax-secret <name>``
WHERE / LIST: ``--where <name>`` / ``--list``
PUT (TTY only): ``hapax-secret`` with no args — name, echo-off secret, confirm.
DELETE (TTY confirm): ``hapax-secret --delete <name>``.

GET / WHERE exit 1 only when the entry's initial stat raises FileNotFoundError;
their existing not-found responses are unchanged. WHERE reports ``filestore``
only after a successful read. Stat/read failures exit 2 with empty stdout and
``unreadable: <mapped-name> (<exception-class>)`` on stderr, never error text.
A present entry for which FileStore.get returns None (including corruption) is
reported as OSError. Disappearance after the initial stat is a read failure;
each invocation observes afresh, so a prior WHERE success does not bind GET.

Put never calls FileStore.put and never ``pass insert``. It POSTs
``http://127.0.0.1:8799/command/secret`` (kind=secret, op=put). Values
never appear on argv. The command ledger records sha256 only.

On a host that is not the FileStore machine, the same argv is forwarded
with ``ssh`` (``-t`` for put and delete) to the logical SSH alias ``secrets-store``.
Bind the alias in private SSH configuration, or override it with
``HAPAX_SECRETS_HOST`` (e.g. ``secrets.example.internal``). Both launchers trim
surrounding whitespace; an unset, empty, or whitespace-only override uses the
alias. The remote shell expands HOME inside double quotes, and each forwarded
argument is quoted separately. ``:8799`` is not opened on the tailnet.
"""

from __future__ import annotations

import base64
import getpass
import json
import os
import re
import shlex
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid
from collections.abc import Callable
from pathlib import Path
from typing import TextIO

from k0.key_capture import FileStore, default_store

ALIASES = {
    "litellm/master-key": "litellm-master-key",
    "langfuse/public-key": "langfuse-public-key",
    "langfuse/secret-key": "langfuse-secret-key",
}
_NAME_RE = re.compile(r"^[A-Za-z0-9._-]+$")
_MAX_SECRET_BYTES = 65536
_DEFAULT_COMMAND_URL = "http://127.0.0.1:8799/command/secret"
_LOOPBACK_HOSTS = frozenset({"127.0.0.1", "localhost", "::1"})


def name_of(raw: str) -> str:
    text = raw.strip().lstrip("/")
    if not text:
        raise ValueError("secret name is empty. Next action: supply a path like hapax-reviewer/org")
    mapped = ALIASES.get(text, text.replace("/", "-"))
    if mapped in {".", ".."} or "/" in mapped or "\\" in mapped or _NAME_RE.fullmatch(mapped) is None:
        raise ValueError(
            "secret name must map to [A-Za-z0-9._-]+. Next action: use letters, digits, dot, "
            "underscore, dash; slashes become dashes"
        )
    return mapped


def command_url() -> str:
    return os.environ.get("HAPAX_SECRET_COMMAND_URL", _DEFAULT_COMMAND_URL).strip() or _DEFAULT_COMMAND_URL


def require_loopback_url(url: str) -> urllib.parse.ParseResult:
    parsed = urllib.parse.urlparse(url)
    host = (parsed.hostname or "").lower()
    if parsed.scheme != "http" or host not in _LOOPBACK_HOSTS:
        raise ValueError(
            "secret command URL must be http loopback (127.0.0.1). Next action: unset "
            "HAPAX_SECRET_COMMAND_URL or point it at http://127.0.0.1:8799/command/secret; "
            "do not bind :8799 on the tailnet"
        )
    return parsed


def is_store_host() -> bool:
    if os.environ.get("HAPAX_SECRETS_FORCE_REMOTE") == "1":
        return False
    store = default_store()
    if store.backend_id != "file":
        return False
    root = Path(store.root)
    return (root / ".key").is_file()


def secrets_host() -> str:
    return os.environ.get("HAPAX_SECRETS_HOST", "secrets-store").strip() or "secrets-store"


def ssh_argv(*, tty: bool, rest: list[str]) -> list[str]:
    host = secrets_host()
    if host.startswith("-"):
        raise ValueError(
            "HAPAX_SECRETS_HOST must not start with '-'. Next action: unset it to use "
            "the secrets-store SSH alias, or set a hostname like secrets.example.internal."
        )
    remote = " ".join(['"$HOME/.local/bin/hapax-secret"', *(shlex.quote(part) for part in rest)])
    cmd = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8"]
    if tty:
        cmd.append("-t")
    cmd.extend(["--", host, remote])
    return cmd


def put_via_reins(name: str, value: bytes, *, post: Callable[[str, bytes], bytes] | None = None) -> dict:
    if len(value) > _MAX_SECRET_BYTES:
        raise ValueError(
            f"secret exceeds {_MAX_SECRET_BYTES} bytes. Next action: store a smaller secret"
        )
    url = command_url()
    require_loopback_url(url)
    body = json.dumps(
        {
            "target": name,
            "authority_packet": {
                "kind": "secret",
                "op": "put",
                "value_b64": base64.b64encode(value).decode("ascii"),
            },
            "preflight_receipt": {},
            "idempotency_key": f"hapax-secret-put-{name}-{uuid.uuid4().hex}",
        },
        separators=(",", ":"),
    ).encode("ascii")
    poster = post or _http_post
    raw = poster(url, body)
    try:
        parsed = json.loads(raw.decode("utf-8"))
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise RuntimeError(
            "reins secret put returned non-JSON. Next action: check reins-read-api on 127.0.0.1:8799"
        ) from exc
    if not isinstance(parsed, dict):
        raise RuntimeError("reins secret put returned a non-object JSON body")
    if parsed.get("status") != "ok":
        reason = parsed.get("reason") or parsed.get("status") or "secret-refused"
        nxt = parsed.get("legal_next") or "fix the named reason and rerun hapax-secret"
        raise RuntimeError(f"{reason}. Next action: {nxt}")
    payload = parsed.get("payload")
    if isinstance(payload, dict):
        payload.pop("value_b64", None)
        payload.pop("value", None)
    return parsed


def delete_via_reins(name: str, *, post: Callable[[str, bytes], bytes] | None = None) -> dict:
    url = command_url()
    require_loopback_url(url)
    body = json.dumps(
        {
            "target": name,
            "authority_packet": {"kind": "secret", "op": "delete"},
            "preflight_receipt": {},
            "idempotency_key": f"hapax-secret-delete-{name}-{uuid.uuid4().hex}",
        },
        separators=(",", ":"),
    ).encode("ascii")
    poster = post or _http_post
    raw = poster(url, body)
    try:
        parsed = json.loads(raw.decode("utf-8"))
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise RuntimeError(
            "reins secret delete returned non-JSON. Next action: check reins-read-api on 127.0.0.1:8799"
        ) from exc
    if not isinstance(parsed, dict):
        raise RuntimeError("reins secret delete returned a non-object JSON body")
    if parsed.get("status") != "ok":
        reason = parsed.get("reason") or parsed.get("status") or "secret-refused"
        nxt = parsed.get("legal_next") or "fix the named reason and rerun hapax-secret --delete"
        raise RuntimeError(f"{reason}. Next action: {nxt}")
    return parsed


def _http_post(url: str, body: bytes) -> bytes:
    req = urllib.request.Request(
        url,
        data=body,
        method="POST",
        headers={"Content-Type": "application/json", "Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.read()
    except urllib.error.HTTPError as exc:
        err_body = exc.read()
        try:
            parsed = json.loads(err_body.decode("utf-8"))
        except (UnicodeError, json.JSONDecodeError):
            raise RuntimeError(
                f"reins secret put HTTP {exc.code}. Next action: start reins-read-api on 127.0.0.1:8799"
            ) from None
        if not isinstance(parsed, dict):
            raise RuntimeError(
                f"reins secret put HTTP {exc.code} with a non-object JSON body. "
                "Next action: start reins-read-api on 127.0.0.1:8799"
            ) from None
        reason = parsed.get("reason") or f"HTTP {exc.code}"
        nxt = parsed.get("legal_next") or "start reins-read-api on 127.0.0.1:8799"
        raise RuntimeError(f"{reason}. Next action: {nxt}") from None
    except urllib.error.URLError as exc:
        raise RuntimeError(
            "reins secret command unreachable on loopback. Next action: start reins-read-api "
            "bound to 127.0.0.1:8799, then rerun hapax-secret"
        ) from exc


def run_put_dialogue(
    *,
    prompt: Callable[[str], str],
    get_secret: Callable[[str], str],
    post: Callable[[str, bytes], bytes] | None = None,
    stdout: TextIO = sys.stdout,
    stderr: TextIO = sys.stderr,
) -> int:
    try:
        raw_name = prompt("Secret name: ")
        name = name_of(raw_name)
    except ValueError as exc:
        print(f"hapax-secret: {exc}", file=stderr)
        return 2
    first = get_secret("Secret: ")
    again = get_secret("Again: ")
    if first != again:
        print(
            "hapax-secret: entries did not match. Next action: run hapax-secret again.",
            file=stderr,
        )
        return 1
    value = first.encode("utf-8")
    if not value:
        print(
            "hapax-secret: empty secret refused. Next action: run hapax-secret again with a non-empty secret.",
            file=stderr,
        )
        return 1
    try:
        result = put_via_reins(name, value, post=post)
    except (ValueError, RuntimeError) as exc:
        print(f"hapax-secret: {exc}", file=stderr)
        return 2
    backend = ""
    payload = result.get("payload")
    if isinstance(payload, dict):
        backend = str(payload.get("backend_id") or "")
    suffix = f" ({backend} via reins)" if backend else " (via reins)"
    print(f"stored {name}{suffix}", file=stdout)
    return 0


def run_delete(
    raw_name: str,
    *,
    confirm: Callable[[str], str],
    post: Callable[[str, bytes], bytes] | None = None,
    stdout: TextIO = sys.stdout,
    stderr: TextIO = sys.stderr,
) -> int:
    try:
        name = name_of(raw_name)
    except ValueError as exc:
        print(f"hapax-secret: {exc}", file=stderr)
        return 2
    answer = confirm(f"Delete {name}? [y/N] ").strip().lower()
    if answer not in {"y", "yes"}:
        print("hapax-secret: delete aborted", file=stderr)
        return 1
    try:
        result = delete_via_reins(name, post=post)
    except (ValueError, RuntimeError) as exc:
        print(f"hapax-secret: {exc}", file=stderr)
        return 2
    payload = result.get("payload") if isinstance(result.get("payload"), dict) else {}
    if payload.get("deleted"):
        print(f"deleted {name}", file=stdout)
        return 0
    print(
        f"not found in FileStore: {name}. Next action: hapax-secret --list",
        file=stderr,
    )
    return 1


def _read_entry(store: FileStore, name: str) -> bytes | None:
    """Only the initial, raising stat may establish absence; never infer it from get."""
    try:
        store._blob_path(name).stat()
    except FileNotFoundError:
        return None
    value = store.get(name)
    if value is None:
        # get conflates absence and corruption. Presence was already observed, so
        # neither corruption nor a subsequent disappearance can mean absent here.
        raise OSError
    return value


def _do_get(name: str) -> int:
    store = default_store()
    if store.backend_id != "file":
        print(
            f"hapax-secret: default_store is {store.backend_id!r}, not file. "
            "legal_next: do not point this CLI at pass.",
            file=sys.stderr,
        )
        return 2
    mapped = name_of(name)
    try:
        val = _read_entry(store, mapped)
    except OSError as exc:
        print(f"unreadable: {mapped} ({type(exc).__name__})", file=sys.stderr)
        return 2
    if val is None:
        print(
            f"not found in FileStore: {mapped}. legal_next: run hapax-secret (TTY put) via reins.",
            file=sys.stderr,
        )
        return 1
    sys.stdout.buffer.write(val + (b"" if val.endswith(b"\n") else b"\n"))
    return 0


def _do_where(name: str) -> int:
    store = default_store()
    if store.backend_id != "file":
        print(
            f"hapax-secret: default_store is {store.backend_id!r}, not file. "
            "legal_next: do not point this CLI at pass.",
            file=sys.stderr,
        )
        return 2
    mapped = name_of(name)
    try:
        val = _read_entry(store, mapped)
    except OSError as exc:
        print(f"unreadable: {mapped} ({type(exc).__name__})", file=sys.stderr)
        return 2
    if val is None:
        print(f"not found: {mapped}")
        return 1
    print("filestore")
    return 0


def _do_list() -> int:
    store = default_store()
    if store.backend_id != "file":
        print(
            f"hapax-secret: default_store is {store.backend_id!r}, not file. "
            "legal_next: do not point this CLI at pass.",
            file=sys.stderr,
        )
        return 2
    root = Path(store.root)
    names = sorted(p.stem for p in root.glob("*.bin")) if root.is_dir() else []
    print("\n".join(names))
    return 0


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if args and args[0] in ("-h", "--help"):
        print(
            "usage: hapax-secret                 # TTY put: name, secret, confirm (via reins)\n"
            "       hapax-secret <name>          # get\n"
            "       hapax-secret --delete <name> # TTY confirm, then delete via reins\n"
            "       hapax-secret --where <name>  # presence\n"
            "       hapax-secret --list"
        )
        return 0

    rest = args
    if not is_store_host():
        tty = (not rest) or (rest[0] == "--delete")
        try:
            argv = ssh_argv(tty=tty, rest=rest)
        except ValueError as exc:
            print(f"hapax-secret: {exc}", file=sys.stderr)
            return 2
        try:
            os.execvp("ssh", argv)
        except FileNotFoundError:
            print(
                "hapax-secret: ssh not found on PATH. Next action: install OpenSSH and rerun, "
                "or run hapax-secret on the FileStore host.",
                file=sys.stderr,
            )
            return 2

    if not rest:
        if not sys.stdin.isatty() or not sys.stdout.isatty():
            print(
                "hapax-secret: put requires a TTY. Next action: run hapax-secret from a terminal "
                "(no args), then enter the name and the secret.",
                file=sys.stderr,
            )
            return 2
        return run_put_dialogue(prompt=input, get_secret=getpass.getpass)

    if rest[0] == "--delete":
        if len(rest) < 2:
            print("usage: hapax-secret --delete <name>", file=sys.stderr)
            return 2
        if not sys.stdin.isatty() or not sys.stdout.isatty():
            print(
                "hapax-secret: delete requires a TTY confirm. Next action: run "
                "hapax-secret --delete <name> from a terminal.",
                file=sys.stderr,
            )
            return 2
        return run_delete(rest[1], confirm=input)
    if rest[0] == "--list":
        return _do_list()
    if rest[0] == "--where":
        if len(rest) < 2:
            print("usage: hapax-secret --where <name>", file=sys.stderr)
            return 2
        try:
            return _do_where(rest[1])
        except ValueError as exc:
            print(f"hapax-secret: {exc}", file=sys.stderr)
            return 2
    try:
        return _do_get(rest[0])
    except ValueError as exc:
        print(f"hapax-secret: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
