"""hapax-secret-migrate — move pass/gopass material into the reins FileStore.

Coordinator-held migration tool (row secrets-authority-and-replicas-filestore-migration-20260916).

Contract:
- Reads entries from a ``pass``-shaped store: a local ``~/.password-store`` (``pass show``) or a
  remote host's store over ``ssh -o BatchMode=yes`` (``pass show`` there). ``gopass`` stores are read
  with ``gopass show -n`` when ``--tool gopass`` is given.
- Maps ``a/b/c`` -> ``a-b-c`` exactly as ``hapax_secret.name_of`` does (aliases included).
- Compares each value with the FileStore IN PROCESS. Absent -> put via reins (the only write path).
  Identical -> skip. Differs -> the FileStore value is kept and the name is listed as a conflict.
  Nothing is ever overwritten by this tool.
- Verifies every put by reading the store back and comparing bytes.
- Never prints, logs, or persists a value or a digest of a value. The manifest carries names,
  byte lengths, dispositions, the source, and timestamps only (low-entropy secrets must not be
  hashable offline).

Usage:
  python3 -m hapax_secret_migrate --source local [--dry-run]
  python3 -m hapax_secret_migrate --source ssh:hapax-podium.local [--dry-run]
  python3 -m hapax_secret_migrate --source local --tool gopass
Exit 0 = every entry is now in the FileStore or on the conflict list; 1 = some entry failed.
"""

from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
import time
from dataclasses import asdict, dataclass
from pathlib import Path

import hapax_secret as hs
from k0.key_capture import default_store

_MANIFEST_DIR = Path.home() / ".local" / "share" / "hapax" / "secrets-migration"
_BOM = b"\xef\xbb\xbf"
_LIST_PASS = 'cd "${PASSWORD_STORE_DIR:-$HOME/.password-store}" && find . -name "*.gpg" -type f | sed "s#^\\./##; s#\\.gpg\\$##" | sort'


@dataclass
class Entry:
    source_path: str
    name: str
    length: int
    disposition: str  # put | identical | conflict | failed | dry-run-put | skipped-empty
    detail: str = ""


def _run(argv: list[str], *, timeout: int = 60) -> bytes:
    proc = subprocess.run(argv, capture_output=True, timeout=timeout)
    if proc.returncode != 0:
        err = proc.stderr.decode("utf-8", "replace").strip().splitlines()
        raise RuntimeError(f"{argv[0]} rc={proc.returncode}: {(err[-1] if err else '')[:160]}")
    return proc.stdout


def _remote_prefix(source: str) -> list[str]:
    if source == "local":
        return []
    if source.startswith("ssh:"):
        host = source[4:]
        if not host or host.startswith("-"):
            raise ValueError("ssh source must be ssh:<host>")
        return ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "--", host]
    raise ValueError("source must be 'local' or 'ssh:<host>'")


def list_entries(source: str, tool: str) -> list[str]:
    prefix = _remote_prefix(source)
    if tool == "pass":
        out = _run(prefix + [_LIST_PASS]) if prefix else _run(["sh", "-c", _LIST_PASS])
    else:
        out = _run(prefix + ["gopass ls -f"]) if prefix else _run(["gopass", "ls", "-f"])
    return [line.strip() for line in out.decode("utf-8", "replace").splitlines() if line.strip()]


def read_entry(source: str, tool: str, path: str) -> bytes:
    prefix = _remote_prefix(source)
    cmd = ["pass", "show", path] if tool == "pass" else ["gopass", "show", "-n", path]
    if prefix:
        return _run(prefix + [" ".join(shlex.quote(c) for c in cmd)])
    return _run(cmd)


def normalize(raw: bytes) -> bytes:
    """pass prints the stored file whole; a single trailing newline is the file convention, not data."""
    return raw[:-1] if raw.endswith(b"\n") else raw


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog="hapax-secret-migrate")
    ap.add_argument("--source", required=True, help="local | ssh:<host>")
    ap.add_argument("--tool", choices=("pass", "gopass"), default="pass")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--only", action="append", default=[], help="restrict to these store paths")
    ap.add_argument("--map", action="append", default=[], metavar="PATH=NAME",
                    help="store a path under an explicit FileStore name (for paths that do not map)")
    ap.add_argument("--keep-bom", action="store_true",
                    help="keep a leading UTF-8 BOM instead of stripping it (default: strip and flag)")
    args = ap.parse_args(argv)

    store = default_store()
    if store.backend_id != "file" or not (Path(store.root) / ".key").is_file():
        print("hapax-secret-migrate: this host is not the FileStore authority (no .key). Next action: run on hapax-appendix.", file=sys.stderr)
        return 2

    explicit = {}
    for item in args.map:
        if "=" not in item:
            print(f"hapax-secret-migrate: --map expects PATH=NAME, got {item!r}", file=sys.stderr)
            return 2
        src, dst = item.rsplit("=", 1)
        explicit[src] = dst
    paths = list_entries(args.source, args.tool)
    if args.only:
        wanted = set(args.only)
        paths = [p for p in paths if p in wanted]
    entries: list[Entry] = []
    failures = 0
    for path in paths:
        try:
            name = hs.name_of(explicit[path]) if path in explicit else hs.name_of(path)
        except ValueError as exc:
            entries.append(Entry(path, "", 0, "failed", f"name: {exc}"[:120]))
            failures += 1
            continue
        try:
            value = normalize(read_entry(args.source, args.tool, path))
        except Exception as exc:  # noqa: BLE001 - message is value-free by construction
            entries.append(Entry(path, name, 0, "failed", f"read: {str(exc)[:120]}"))
            failures += 1
            continue
        if not value:
            entries.append(Entry(path, name, 0, "skipped-empty"))
            continue
        flags = []
        if value.startswith(_BOM):
            if args.keep_bom:
                flags.append("bom-kept")
            else:
                value = value[len(_BOM):]
                flags.append("bom-stripped")
        if b"\r" in value:
            flags.append("cr")
        if b"\n" in value:
            flags.append("multiline")
        current = store.get(name)
        if current is not None:
            if current == value:
                entries.append(Entry(path, name, len(value), "identical", ",".join(flags)))
            else:
                entries.append(Entry(path, name, len(value), "conflict", "filestore kept;" + ",".join(flags)))
            continue
        if args.dry_run:
            entries.append(Entry(path, name, len(value), "dry-run-put", ",".join(flags)))
            continue
        try:
            hs.put_via_reins(name, value)
            if store.get(name) != value:
                raise RuntimeError("readback mismatch")
            entries.append(Entry(path, name, len(value), "put", ",".join(flags)))
        except Exception as exc:  # noqa: BLE001
            entries.append(Entry(path, name, len(value), "failed", f"put: {str(exc)[:120]}"))
            failures += 1

    _MANIFEST_DIR.mkdir(parents=True, exist_ok=True, mode=0o700)
    stamp = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    manifest = _MANIFEST_DIR / f"migrate-{args.tool}-{args.source.replace(':', '-')}-{stamp}.json"
    counts: dict[str, int] = {}
    for e in entries:
        counts[e.disposition] = counts.get(e.disposition, 0) + 1
    manifest.write_text(json.dumps({
        "tool": args.tool, "source": args.source, "dry_run": args.dry_run, "at": stamp,
        "counts": counts, "entries": [asdict(e) for e in entries],
    }, indent=1))
    os.chmod(manifest, 0o600)
    print(f"manifest: {manifest}")
    print("counts: " + ", ".join(f"{k}={v}" for k, v in sorted(counts.items())))
    for e in entries:
        if e.disposition in ("conflict", "failed") or e.detail:
            print(f"  {e.disposition:12} {e.name:60} len={e.length:<6} {e.detail}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
