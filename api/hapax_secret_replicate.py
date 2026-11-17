"""hapax-secret-replicate — push the authority FileStore to a host's read replica.

Coordinator-held (row secrets-authority-and-replicas-filestore-migration-20260916).

Topology: ONE write authority (the FileStore on hapax-appendix). Other hosts hold read replicas
materialized by this push, re-wrapped under the replica's own ``.key`` by the replica's own reins
put path. A replica never receives values on argv or in env: they travel as JSON lines on the
ssh stdin, base64-encoded, and are put through ``hapax_secret.put_via_reins`` on the far side.

  sender  : python3 -m hapax_secret_replicate --to hapax-podium.local [--names a,b,c] [--dry-run]
  receiver: python3 -m hapax_secret_replicate --receive      (invoked over ssh by the sender)

The receiver compares each value with its local store in process, puts only when absent or
different, verifies by readback, and reports ``name status length`` lines — never values.
The sender writes a manifest (names, lengths, statuses, host, time; no digests).
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import subprocess
import sys
import time
from pathlib import Path

import hapax_secret as hs
from k0.key_capture import default_store

_MANIFEST_DIR = Path.home() / ".local" / "share" / "hapax" / "secrets-migration"


def _probe_secret_verb() -> bool:
    """True when this host's reins command surface registers the secret verb (loopback probe, op=has)."""
    import urllib.request
    body = json.dumps({"target": "probe", "authority_packet": {"kind": "secret", "op": "has"},
                       "preflight_receipt": {}, "idempotency_key": "replicate-probe"}).encode("ascii")
    req = urllib.request.Request(hs.command_url(), data=body, method="POST",
                                 headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            parsed = json.loads(resp.read().decode("utf-8"))
    except Exception:  # noqa: BLE001 - unreachable or non-JSON both mean "no verb here"
        return False
    return parsed.get("status") == "ok"


def _receive(direct: bool) -> int:
    store = default_store()
    if store.backend_id != "file" or not (Path(store.root) / ".key").is_file():
        print("replica: no local FileStore .key on this host", file=sys.stderr)
        return 2
    rc = 0
    if direct:
        mode = "direct"
    else:
        mode = "reins" if _probe_secret_verb() else "direct"
    print(f"# mode={mode}" + ("" if mode == "reins" else " (reins secret verb unregistered on this replica; FileStore.put)"))
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            item = json.loads(line)
            name = hs.name_of(str(item["name"]))
            value = base64.b64decode(str(item["value_b64"]), validate=True)
        except Exception as exc:  # noqa: BLE001
            print(f"? invalid-line {str(exc)[:80]}")
            rc = 1
            continue
        current = store.get(name)
        if current == value:
            print(f"{name} identical {len(value)}")
            continue
        try:
            if mode == "reins":
                hs.put_via_reins(name, value)
            else:
                store.put(name, value)
            if store.get(name) != value:
                raise RuntimeError("readback mismatch")
            print(f"{name} {'updated' if current is not None else 'put'} {len(value)}")
        except Exception as exc:  # noqa: BLE001
            print(f"{name} failed {len(value)} {str(exc)[:100]}")
            rc = 1
    return rc


def _send(host: str, names: list[str] | None, dry_run: bool, remote_module_dir: str) -> int:
    store = default_store()
    if store.backend_id != "file" or not (Path(store.root) / ".key").is_file():
        print("replicate: this host is not the authority (no .key)", file=sys.stderr)
        return 2
    root = Path(store.root)
    all_names = sorted(p.stem for p in root.glob("*.bin"))
    wanted = [n for n in all_names if not names or n in set(names)]
    missing = sorted(set(names or []) - set(all_names))
    if missing:
        print(f"replicate: not in authority: {' '.join(missing)}", file=sys.stderr)
        return 2
    payload = []
    for n in wanted:
        v = store.get(n)
        if v is None:
            print(f"replicate: unreadable in authority: {n}", file=sys.stderr)
            return 2
        payload.append(json.dumps({"name": n, "value_b64": base64.b64encode(v).decode("ascii")}))
    if dry_run:
        print(f"dry-run: would push {len(payload)} names to {host}")
        return 0
    remote = (
        f"PYTHONPATH=$HOME/.local/share/reins/current/api:{remote_module_dir} "
        f"python3 -m hapax_secret_replicate --receive"
    )
    proc = subprocess.run(
        ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "--", host, remote],
        input=("\n".join(payload) + "\n").encode("ascii"), capture_output=True, timeout=900,
    )
    lines = proc.stdout.decode("utf-8", "replace").splitlines()
    counts: dict[str, int] = {}
    entries = []
    mode_line = next((ln for ln in lines if ln.startswith("# mode=")), "")
    for ln in lines:
        if ln.startswith("#"):
            continue
        parts = ln.split(" ", 3)
        if len(parts) >= 3:
            entries.append({"name": parts[0], "status": parts[1], "length": parts[2], "detail": parts[3] if len(parts) > 3 else ""})
            counts[parts[1]] = counts.get(parts[1], 0) + 1
    _MANIFEST_DIR.mkdir(parents=True, exist_ok=True, mode=0o700)
    stamp = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    manifest = _MANIFEST_DIR / f"replicate-{host}-{stamp}.json"
    manifest.write_text(json.dumps({"host": host, "at": stamp, "rc": proc.returncode, "mode": mode_line, "counts": counts, "entries": entries,
                                    "stderr": proc.stderr.decode("utf-8", "replace")[-400:]}, indent=1))
    os.chmod(manifest, 0o600)
    print(f"manifest: {manifest}")
    print(mode_line)
    print(f"remote rc={proc.returncode} counts: " + ", ".join(f"{k}={v}" for k, v in sorted(counts.items())))
    for e in entries:
        if e["status"] == "failed":
            print(f"  failed {e['name']} {e['detail']}")
    if proc.returncode != 0 and not entries:
        print(proc.stderr.decode("utf-8", "replace")[-300:], file=sys.stderr)
    return 0 if proc.returncode == 0 else 1


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog="hapax-secret-replicate")
    ap.add_argument("--to", help="replica host (ssh)")
    ap.add_argument("--names", help="comma-separated subset (default: every name in the authority)")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--receive", action="store_true", help="replica side; reads JSON lines on stdin")
    ap.add_argument("--direct", action="store_true", help="receiver: write with FileStore.put (no reins verb on this host)")
    ap.add_argument("--remote-module-dir", default="$HOME/.local/share/hapax/reins-tools",
                    help="where hapax_secret_replicate.py lives on the replica")
    args = ap.parse_args(argv)
    if args.receive:
        return _receive(args.direct)
    if not args.to:
        ap.error("--to <host> or --receive")
    names = [n.strip() for n in args.names.split(",")] if args.names else None
    return _send(args.to, names, args.dry_run, args.remote_module_dir)


if __name__ == "__main__":
    raise SystemExit(main())
