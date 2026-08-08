"""K0 — the first-init ceremony driver: the sovereign key act as one command.

Until this module the ceremony spine (ceremony.py) was a library with no invocation
entry: the estate's first ceremony — and with it the R2.2 ceremony-native ratification
receipt — could not be run. This driver is the entry point.

The operator's act, end to end:

    cd ~/projects/reins
    uv run python -m k0.ceremony_driver \
        --key ~/.ssh/hapax-sovereign-ed25519 \
        --principal operator@hapax-sovereign \
        --estate-id estate:<the estate's own name> \
        --with-r22

What it does, in order, each step receipted and each refusal typed:

1. declare the durable root (fail-closed on a volatile filesystem),
2. open the chain with the genesis self-attest when empty (the kernel attesting its
   own manifest — never a caller-supplied digest),
3. perform the genesis stipulations (identity -> registry -> boot profile -> egress
   allowlist -> forge -> support boundary -> the fatigue run receipt) with every
   value an explicit flag carrying the estate's current answer as default,
4. with --with-r22, propose and ratify the R2.2 segment stipulation over the pinned
   canonical bytes (the digest is recomputed and checked against R22_RATIFIED_PIN
   before any signing — a drifted segment is refused, never signed).

The ceremony is IDEMPOTENT-BY-REFUSAL: a completed ceremony refuses a second run
(ceremony_complete), a ratified stipulation refuses re-proposal (ratification.py).
A partial run leaves a true state — pending() names what remains.

This driver transmits nothing, calls no model, and performs no network I/O. The
signing is SSHSIG through the operator's key via ratifier.py (the kernel's only
subprocess boundary).
"""

from __future__ import annotations

import argparse
import sys
from datetime import UTC, datetime
from pathlib import Path

from bootstrap_receipt import (
    append_receipt,
    declare_durable_root,
    genesis_self_attest,
    load_chain,
    verify_chain_at,
)
from k0.boot_profile import PROFILES
from k0.ceremony import ceremony_complete, ratify_genesis_stipulations
from k0.egress_consent import EgressAllowlist
from k0.forge_choice import FORGE_PROFILES, ForgeChoice
from k0.manifest import kernel_identity
from k0.ratification import Stipulation, propose, ratify
from k0.refusal import RefusalError
from k0.support_boundary import SupportBoundary

import deterministic_segment

#: Estate-default answers, each overridable by flag. These are the values the landed
#: R2.x modules already carry as the estate's current state; the driver makes them
#: explicit rather than inventing them.
DEFAULT_ROOT = Path.home() / ".local" / "share" / "hapax" / "k0"
DEFAULT_ESTATE_ID = "estate:local"
DEFAULT_ROLES = ("claude", "codex", "glm", "kimi")
DEFAULT_HOSTS = ("api.anthropic.com", "api.z.ai")

R22_STIPULATION_ID = "r22-deterministic-segment"
R22_SUBJECT = (
    "the deterministic pre-model segment, ratified as a regime: membership, the "
    "half-open boundary (the crow cold-start is the terminal act), and the honesty "
    "laws, over the pinned canonical bytes"
)


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="python -m k0.ceremony_driver",
        description="Run the first-init K0 ceremony: genesis, the genesis stipulations, "
        "and optionally the R2.2 segment ratification — each act receipted, each signed "
        "by the sovereign key.",
    )
    parser.add_argument("--root", type=Path, default=DEFAULT_ROOT,
                        help=f"the durable K0 root (default {DEFAULT_ROOT})")
    parser.add_argument("--key", type=Path, required=True,
                        help="the sovereign SSH private key (ed25519) used for SSHSIG signing")
    parser.add_argument("--principal", required=True,
                        help="the sovereign principal, e.g. operator@hapax-sovereign")
    parser.add_argument("--estate-id", default=DEFAULT_ESTATE_ID)
    parser.add_argument("--roles", default=",".join(DEFAULT_ROLES),
                        help="comma-separated initial role registry")
    parser.add_argument("--hosts", default=",".join(DEFAULT_HOSTS),
                        help="comma-separated egress allowlist hosts")
    parser.add_argument("--with-r22", action="store_true",
                        help="also propose and ratify the R2.2 segment stipulation")
    parser.add_argument("--dry-run", action="store_true",
                        help="print the acts and checks without writing anything")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    identity = kernel_identity()
    roles = tuple(r.strip() for r in args.roles.split(",") if r.strip())
    hosts = tuple(h.strip() for h in args.hosts.split(",") if h.strip())

    if args.dry_run:
        chain_len = len(load_chain(args.root)) if (args.root / "bootstrap-receipts.jsonl").exists() else 0
        print(f"root: {args.root} (chain rows: {chain_len})")
        print(f"estate_id: {args.estate_id}  kernel: {identity['kernel_version']}")
        print(f"principal: {args.principal}  roles: {roles}")
        print(f"hosts: {hosts}  forge: github-only-ceiling  profile: existing-agent-harness")
        print(f"with-r22: {args.with_r22} (segment digest {deterministic_segment.R22_RATIFIED_PIN[:16]}…)")
        print("dry-run: nothing written, nothing signed")
        return 0

    if not args.key.is_file():
        print(f"ceremony driver: sovereign key not found: {args.key}", file=sys.stderr)
        print("next: ssh-keygen -t ed25519 -C '<principal>' -f <path> — the key's provenance "
              "is the operator's own act", file=sys.stderr)
        return 2

    declare_durable_root(args.root)  # raises (fail-closed) on a volatile filesystem

    chain_path = args.root / "bootstrap-receipts.jsonl"
    if not chain_path.exists():
        receipt = genesis_self_attest(
            estate_id=args.estate_id,
            kernel_version=identity["kernel_version"],
            kernel_manifest_sha256=identity["kernel_manifest_sha256"],
            observed_at=datetime.now(UTC),
        )
        append_receipt(args.root, receipt)
        print(f"genesis: {receipt.receipt_id}")

    if ceremony_complete(args.root):
        print("ceremony: already complete — nothing to do (a completed ceremony refuses a second run)")
    else:
        result = ratify_genesis_stipulations(
            args.root,
            principal=args.principal,
            roles=roles,
            boot_profile=PROFILES["existing-agent-harness"],
            allowlist=EgressAllowlist(hosts=hosts),
            forge_profile=FORGE_PROFILES[ForgeChoice.GITHUB_ONLY],
            key_path=args.key,
            support_boundary=SupportBoundary(
                in_scope=("install", "first-init", "reins"),
                out_scope=("custom-consulting", "hosted-service"),
                answer_surface="docs/SUPPORT.md",
            ),
            estate_id=args.estate_id,
            kernel_version=identity["kernel_version"],
            observed_at=datetime.now(UTC),
        )
        print(f"ceremony: complete — identity {result.sovereign_identity}")

    if args.with_r22:
        canonical = deterministic_segment.DETERMINISTIC_SEGMENT.canonical()
        digest = deterministic_segment.DETERMINISTIC_SEGMENT.digest()
        if digest != deterministic_segment.R22_RATIFIED_PIN:
            print("ceremony driver: REFUSED — the segment's digest does not match "
                  "R22_RATIFIED_PIN; a drifted segment is never signed", file=sys.stderr)
            return 3
        stipulation = Stipulation.over(
            R22_STIPULATION_ID, R22_SUBJECT, canonical.encode("utf-8")
        )
        try:
            propose(
                args.root, stipulation,
                estate_id=args.estate_id,
                kernel_version=identity["kernel_version"],
                observed_at=datetime.now(UTC),
            )
        except RefusalError:
            pass  # already pending is a true state; ratify reads it
        ratify(
            args.root, stipulation,
            key_path=args.key,
            estate_id=args.estate_id,
            kernel_version=identity["kernel_version"],
            observed_at=datetime.now(UTC),
        )
        print(f"r22: ratified — {stipulation.stipulation_id} over sha256:{digest[:16]}…")

    verdict = verify_chain_at(args.root)
    if not verdict.ok:
        print(f"ceremony driver: chain does not verify: {verdict.errors}", file=sys.stderr)
        return 4
    print(f"chain: verifies ({len(load_chain(args.root))} receipts at {args.root})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
