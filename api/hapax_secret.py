"""hapax-secret — operator ritual for estate secrets via reins.

GET (watchdogs/scripts): ``hapax-secret <name>``
WHERE / LIST: ``--where <name>`` / ``--list`` (``--list --json`` for metadata)
AUDIT: ``--audit`` (``--audit --json``) — byte shapes of every stored value.
HISTORY: ``--history <name>`` — timestamps of superseded versions.
PUT (TTY only): ``hapax-secret`` with no args — name, echo-off secret, confirm.
DELETE (TTY confirm): ``hapax-secret --delete <name>``.

EXIT CODES. 0 success. 1 the name is absent (GET, WHERE, DELETE) or the
dialogue was abandoned. 2 the invocation or the store is unusable — bad name,
unreadable entry, no TTY where one is required, a refused value. 3 and only 3:
``integrity_failed: <name>`` — the blob is present and does not authenticate.
3 exists because 1 used to cover it: FileStore.get returned None for a
tampered blob and every caller printed "not found" and offered to re-enter a
secret that was, in fact, sitting right there.

GET / WHERE exit 1 only when the entry's initial stat raises FileNotFoundError.
WHERE reports ``filestore`` only after a successful read. Stat/read failures
exit 2 with empty stdout and ``unreadable: <mapped-name> (<exception-class>)``
on stderr, never error text. Disappearance after the initial stat is a read
failure; each invocation observes afresh, so a prior WHERE success does not
bind GET.

GET writes the value followed by exactly one newline, adding one only if the
value does not already end in one. A consumer that must have the exact bytes
should strip a single trailing newline, not ``tr -d``: ``--audit`` now refuses
to let a value carry CR or LF in the first place, so a trailing newline on
stdout is this program's framing and never part of the secret.

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
from datetime import UTC, datetime
from pathlib import Path
from typing import TextIO

from k0.key_capture import (
    INFORMATIONAL_VALUE_FLAGS,
    SECRET_COMMAND_TOKEN_HEADER,
    FileStore,
    SecretCommandTokenError,
    SecretIntegrityError,
    default_store,
    mint_secret_command_token,
    secret_value_flags,
    validate_secret_value,
)

#: Flags --audit prints but does not fail on: the store-level informational
#: set, plus the migration state, which is a fact about the blob rather than
#: about the value.
_AUDIT_INFORMATIONAL = INFORMATIONAL_VALUE_FLAGS | {"legacy-format-v1"}

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
    """This host holds the store when the store's KEY is here — which is not
    necessarily ``root/.key`` any more, since --key-file / REINS_SECRET_KEY_FILE
    can move it off the backed-up home. Asking the store where its key lives is
    the difference between honouring the override and silently forwarding every
    command to another machine."""
    if os.environ.get("HAPAX_SECRETS_FORCE_REMOTE") == "1":
        return False
    store = _store()
    return store.key_path.is_file()


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
    # A MIRROR, not the gate. The command surface validates the same bytes with
    # the same function and is what a value actually has to get past; this call
    # only saves a round trip and gives the operator the sentence immediately.
    validate_secret_value(value)
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


def _command_headers() -> dict[str, str]:
    """The per-boot capability token that the writing ops require.

    A token the CLI cannot mint is not a reason to post anyway and let the
    server decide: the surface would answer a governed refusal, but the
    operator would read it as "the server rejected me" rather than "this
    session has no runtime directory". Refuse here, naming the real cause.
    """
    headers = {"Content-Type": "application/json", "Accept": "application/json"}
    try:
        headers[SECRET_COMMAND_TOKEN_HEADER] = mint_secret_command_token()
    except SecretCommandTokenError as exc:
        raise RuntimeError(str(exc)) from None
    return headers


def _http_post(url: str, body: bytes) -> bytes:
    req = urllib.request.Request(
        url,
        data=body,
        method="POST",
        headers=_command_headers(),
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


def _store() -> FileStore:
    """The store, honouring --key-file / REINS_SECRET_KEY_FILE.

    The `default_store is not file` branches that used to head every read path
    are gone with PassStore: `default_store()` is unconditional now, so there
    is no backend for them to catch and a branch that cannot fire is a branch
    that will be maintained forever for nothing.
    """
    store = default_store()
    override = os.environ.get("HAPAX_SECRET_KEY_FILE", "").strip()
    if override:
        store.key_file = Path(override)
    return store


def _read_entry(store: FileStore, name: str) -> bytes | None:
    """Only the initial, raising stat may establish absence; never infer it from get."""
    try:
        store._blob_path(name).stat()
    except FileNotFoundError:
        return None
    value = store.get(name)
    if value is None:
        # get no longer conflates absence with corruption — corruption raises
        # SecretIntegrityError and passes straight through. Presence was already
        # observed, so a None here is a disappearance between stat and read.
        raise OSError
    return value


def _do_get(name: str) -> int:
    store = _store()
    mapped = name_of(name)
    try:
        val = _read_entry(store, mapped)
    except SecretIntegrityError:
        print(
            f"integrity_failed: {mapped}. legal_next: the stored blob does not authenticate "
            "— restore it from backup or run hapax-secret (TTY put) to replace it.",
            file=sys.stderr,
        )
        return 3
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
    store = _store()
    mapped = name_of(name)
    try:
        val = _read_entry(store, mapped)
    except SecretIntegrityError:
        print(
            f"integrity_failed: {mapped}. legal_next: the stored blob does not authenticate "
            "— restore it from backup or run hapax-secret (TTY put) to replace it.",
            file=sys.stderr,
        )
        return 3
    except OSError as exc:
        print(f"unreadable: {mapped} ({type(exc).__name__})", file=sys.stderr)
        return 2
    if val is None:
        print(f"not found: {mapped}")
        return 1
    print("filestore")
    return 0


def _do_list(as_json: bool = False) -> int:
    store = _store()
    names = store.names()
    if not as_json:
        print("\n".join(names))
        return 0
    rows = []
    for name in names:
        path = store._blob_path(name)
        try:
            stat_result = path.stat()
        except OSError:
            continue
        rows.append(
            {
                "name": name,
                # the BLOB size, deliberately: the plaintext length is a fact
                # about the secret, and this listing must not carry one.
                "blob_bytes": stat_result.st_size,
                "format": store.blob_format(name),
                "mtime": datetime.fromtimestamp(stat_result.st_mtime, UTC).isoformat(),
            }
        )
    print(json.dumps(rows, indent=2, sort_keys=True))
    return 0


def _do_history(name: str) -> int:
    store = _store()
    mapped = name_of(name)
    stamps = store.history(mapped)
    if not stamps:
        print(
            f"no history for {mapped}. legal_next: history begins at the next put; "
            "hapax-secret --list to see stored names.",
            file=sys.stderr,
        )
        return 1
    print("\n".join(stamps))
    return 0


def _do_audit(as_json: bool = False) -> int:
    """Report the byte shapes of every stored value. Names and flags only.

    THIS EXISTS BECAUSE IT HAPPENED. 17 of 112 stored values began with a UTF-8
    byte-order mark, carried in when the store was populated from pass, and
    every consumer of those keys got a 401 that named the provider rather than
    the store. Put-time refusal stops the next one being written; nothing was
    watching the ones already there. Exit 1 when any name carries a flag, so a
    watchdog can run this and notice.
    """
    store = _store()
    rows = []
    for name in store.names():
        row: dict[str, object] = {"name": name, "format": store.blob_format(name)}
        try:
            value = store.get(name)
        except SecretIntegrityError:
            row["flags"] = ["integrity-failed"]
            rows.append(row)
            continue
        except OSError as exc:
            row["flags"] = [f"unreadable-{type(exc).__name__}"]
            rows.append(row)
            continue
        if value is None:
            continue
        flags = list(secret_value_flags(value))
        if row["format"] == 1:
            flags.append("legacy-format-v1")
        row["flags"] = flags
        rows.append(row)
    # A DEFECT is a byte shape measured to break a consumer, or a blob that
    # cannot be read. `multiline`, `non-utf8` and `legacy-format-v1` are
    # INFORMATION: a document, a binary secret and an unmigrated blob are each
    # perfectly serviceable, and an audit that exits non-zero for them is an
    # audit a watchdog learns to ignore.
    defective = [r for r in rows if set(r["flags"]) - _AUDIT_INFORMATIONAL]
    legacy = [r for r in rows if "legacy-format-v1" in r["flags"]]
    if as_json:
        print(json.dumps(rows, indent=2, sort_keys=True))
    else:
        for row in rows:
            flags = row["flags"] or ["ok"]
            # CodeQL taints `flags` because it is computed FROM the value. It is
            # not derived from it: secret_value_flags returns a subset of the
            # closed literal vocabulary k0.key_capture.SECRET_VALUE_FLAGS and
            # nothing else, pinned by
            # test_secret_value_flags_only_ever_returns_the_closed_vocabulary,
            # and the audit tests assert the canary appears on neither stream
            # (mutation-covered: making --audit stop flagging turns them red).
            line = f"{row['name']}\tv{row['format']}\t{','.join(flags)}"  # codeql[py/clear-text-logging-sensitive-data]
            print(line)
        if defective:
            print(
                f"{len(defective)} of {len(rows)} stored values carry a byte shape "
                "that breaks consumers. Next action: re-put each one with "
                "hapax-secret (TTY put); the value is refused at put until the "
                "shape is gone.",
                file=sys.stderr,
            )
        if legacy:
            print(
                f"note: {len(legacy)} of {len(rows)} blobs are still format 1. They "
                "read correctly; each migrates to format 2 on its next put. Not a "
                "defect.",
                file=sys.stderr,
            )
    return 1 if defective else 0


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if args and args[0] in ("-h", "--help"):
        print(
            "usage: hapax-secret                  # TTY put: name, secret, confirm (via reins)\n"
            "       hapax-secret <name>           # get\n"
            "       hapax-secret --delete <name>  # TTY confirm, then delete via reins\n"
            "       hapax-secret --where <name>   # presence\n"
            "       hapax-secret --list [--json]  # names, or name/size/format/mtime\n"
            "       hapax-secret --audit [--json] # byte-shape flags per name, never values\n"
            "       hapax-secret --history <name> # timestamps of superseded versions\n"
            "\n"
            "       --key-file <path>  read the store key from <path> instead of\n"
            "                          <store>/.key (also REINS_SECRET_KEY_FILE)\n"
            "\n"
            "exit: 0 ok · 1 absent · 2 unusable invocation or store · 3 integrity_failed"
        )
        return 0

    # --key-file is read BEFORE is_store_host, because it decides which path is
    # probed to answer "is the store here". `rest` drops it for local dispatch;
    # the ssh forward sends the ORIGINAL argv, so the remote gets the flag as
    # well — an env var set here would not cross the connection.
    rest = list(args)
    if "--key-file" in rest:
        i = rest.index("--key-file")
        if i + 1 >= len(rest):
            print("usage: hapax-secret --key-file <path> <verb...>", file=sys.stderr)
            return 2
        os.environ["HAPAX_SECRET_KEY_FILE"] = rest[i + 1]
        del rest[i : i + 2]
    if not is_store_host():
        tty = (not rest) or (rest[0] == "--delete")
        try:
            argv = ssh_argv(tty=tty, rest=args)
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
        return _do_list(as_json="--json" in rest[1:])
    if rest[0] == "--audit":
        return _do_audit(as_json="--json" in rest[1:])
    if rest[0] == "--history":
        if len(rest) < 2:
            print("usage: hapax-secret --history <name>", file=sys.stderr)
            return 2
        try:
            return _do_history(rest[1])
        except ValueError as exc:
            print(f"hapax-secret: {exc}", file=sys.stderr)
            return 2
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
