"""K0 member `identity-seed` — the estate identity, minted locally. (R0.11 / R0.3 W3)

RATIFIED AS KERNEL 2026-08-01, tier `core`, lever class state-context. The circularity witness:
receipts are chained under an estate/operator identity, and minting that identity is an act
requiring a receipt, which requires the identity to attribute it to.

WHAT WAS MISSING. `bootstrap_receipt.genesis_self_attest()` already takes `estate_id` as a
parameter and `BootstrapReceipt` already carries it as a field — the receipt spine CONSUMES an
identity but nothing MINTS one. This is that producer.

NON-PII BY CONSTRUCTION, NOT BY POLICY
--------------------------------------
R0.10's never-collect list names **raw host fingerprints** alongside operator PII, credentials and
transcripts. So the estate id must NOT be derived from the host: not hostname, not MAC, not
username, not machine-id, not a hash of any of them. A hash does not launder a fingerprint — it is
still a stable identifier of that machine, and it still correlates across estates.

It is therefore drawn from `secrets.token_hex` — pure CSPRNG, no host input at any point. There is
nothing to leak because nothing about the host ever enters it. `assert_non_pii()` re-checks that at
runtime against the actual host facts, so a future edit that "improves" minting by seeding it from
the machine fails loudly instead of quietly shipping a fingerprint.

DURABILITY IS A PRECONDITION, NOT A DETAIL
------------------------------------------
The seed is minted ONCE and every subsequent receipt chains under it. Losing it does not lose a
convenience — it makes the entire existing chain unattributable, because the identity the chain is
signed against no longer exists. So minting refuses to write to a root that has not passed the
durable-root guard, and load-or-mint never silently re-mints: a missing seed beside an existing
chain is an ERROR, not an invitation to start a new identity.
"""

from __future__ import annotations

import json
import os
import secrets
import socket
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

from .fail_closed import Evaluation, decide

#: Filename under the durable root. Deliberately not dotted: this is a fact of record, not config.
SEED_FILENAME = "estate-identity.json"

#: 32 hex chars = 128 bits. Enough that collision across estates is not a concern, short enough to
#: appear in a receipt id without dominating it.
_ID_BYTES = 16


class IdentitySeedError(RuntimeError):
    """Raised when the identity seed cannot be established or is untrustworthy."""


@dataclass(frozen=True)
class EstateIdentity:
    """The minted seed. `estate_id` is the value the receipt chain attributes acts to."""

    estate_id: str
    minted_at: datetime
    seed_schema: int = 1

    def to_json(self) -> str:
        return json.dumps(
            {
                "seed_schema": self.seed_schema,
                "estate_id": self.estate_id,
                "minted_at": self.minted_at.isoformat(),
            },
            sort_keys=True,
            indent=2,
        )

    @classmethod
    def from_json(cls, text: str) -> EstateIdentity:
        try:
            d = json.loads(text)
        except json.JSONDecodeError as e:
            raise IdentitySeedError(f"identity seed is not readable JSON: {e}") from e
        if d.get("seed_schema") != 1:
            raise IdentitySeedError(f"unknown seed_schema {d.get('seed_schema')!r}")
        for k in ("estate_id", "minted_at"):
            if not str(d.get(k, "")).strip():
                raise IdentitySeedError(f"identity seed missing {k}")
        try:
            minted_at = datetime.fromisoformat(str(d["minted_at"]))
        except ValueError as e:
            # Every other corruption in this loader raises IdentitySeedError. A bare ValueError
            # here escapes the seed-error contract, so a caller catching IdentitySeedError to
            # refuse cleanly would instead crash on this one field.
            raise IdentitySeedError(
                f"identity seed has an unparseable minted_at {d['minted_at']!r}: {e}"
            ) from e
        return cls(
            estate_id=str(d["estate_id"]),
            minted_at=minted_at,
            seed_schema=1,
        )


def _host_facts() -> list[str]:
    """The host identifiers the seed must never encode. Read ONLY to prove absence."""
    facts = [socket.gethostname(), os.environ.get("USER", ""), os.environ.get("LOGNAME", "")]
    try:
        facts.append(Path("/etc/machine-id").read_text().strip())
    except OSError:
        pass
    return [f.strip().lower() for f in facts if f and f.strip()]


def assert_non_pii(estate_id: str) -> None:
    """Fail closed if the id encodes any host fact. Cheap, and it catches the exact regression
    where someone makes minting 'deterministic' by seeding it from the machine."""
    low = estate_id.lower()
    for fact in _host_facts():
        if len(fact) >= 4 and fact in low:
            raise IdentitySeedError(
                f"estate_id contains a host fact ({fact!r}). The identity seed must be non-PII and "
                f"must not fingerprint the host — hashing it would not help, a stable machine "
                f"identifier is still a fingerprint."
            )


def mint_estate_id() -> str:
    """A fresh, non-PII estate id. CSPRNG only — no host input at any point."""
    estate_id = secrets.token_hex(_ID_BYTES)
    assert_non_pii(estate_id)
    return estate_id


def load_or_mint(durable_root: Path, *, chain_exists: bool | None = None) -> EstateIdentity:
    """Load the seed under `durable_root`, or mint it on first run.

    `chain_exists` is the fail-closed guard that matters: if a receipt chain is already present but
    the seed is gone, re-minting would silently orphan every existing receipt. That case refuses.
    Pass None when it genuinely cannot be determined — an unevaluable predicate DENIES.
    """
    if not durable_root.is_dir():
        raise IdentitySeedError(
            f"durable root {durable_root} is not a directory — declare_durable_root() must pass "
            f"before an identity is minted; the seed has nowhere durable to land"
        )

    path = durable_root / SEED_FILENAME
    if path.exists():
        identity = EstateIdentity.from_json(path.read_text(encoding="utf-8"))
        assert_non_pii(identity.estate_id)
        return identity

    # No seed on disk. Minting is only safe if no chain is already attributing acts to one.
    decide(
        "identity-seed",
        Evaluation.SATISFIED if chain_exists is False else (
            Evaluation.VIOLATED if chain_exists else Evaluation.UNEVALUABLE
        ),
        legal_next=(
            "restore the identity seed from backup, or declare this a new estate deliberately"
        ),
        violated_why=(
            "a receipt chain exists but its identity seed is missing. Minting a new one would "
            "orphan every receipt already chained under the old identity."
        ),
        unevaluable_why=(
            "could not determine whether a receipt chain already exists. Minting blind risks "
            "orphaning an existing chain, so it denies."
        ),
        teaches="doctrine/identity-seed",
    )

    identity = EstateIdentity(estate_id=mint_estate_id(), minted_at=datetime.now(UTC))
    # EXCLUSIVE CREATION, NOT CHECK-THEN-WRITE. Two processes can both pass the existence check
    # above before either writes. With os.replace, both would mint, the last write would win, and
    # the loser would return an identity that is NOT the one on disk -- attributing its receipts to
    # an estate that does not exist. os.link fails atomically if the destination is already there,
    # so exactly one minter can win, and the loser ADOPTS the winner's seed rather than its own.
    tmp = path.parent / f".{path.name}.{os.getpid()}.{secrets.token_hex(8)}.tmp"
    # ATOMIC IS NOT DURABLE. os.replace guarantees no reader sees a torn file; it guarantees
    # nothing about surviving power loss. Without the fsyncs, a crash seconds after minting can
    # leave the rename recorded and the CONTENTS empty -- and the estate identity is the one file
    # that cannot be re-derived: a lost seed orphans every receipt in the chain.
    with open(tmp, "w", encoding="utf-8") as handle:
        handle.write(identity.to_json())
        handle.flush()
        os.fsync(handle.fileno())
    try:
        os.link(tmp, path)  # atomic: a torn seed file is an unattributable chain
    except FileExistsError:
        # Another minter won the race. Its seed is the estate's identity; ours never existed.
        os.unlink(tmp)
        return EstateIdentity.from_json(path.read_text(encoding="utf-8"))
    os.unlink(tmp)
    dir_fd = os.open(path.parent, os.O_RDONLY)
    try:
        os.fsync(dir_fd)  # the LINK itself must reach disk, not just the bytes
    finally:
        os.close(dir_fd)
    return identity


# ---------------------------------------------------------------------------------------------
# RATIFIER SIGNING KEY — NOT IMPLEMENTED, DELIBERATELY.
#
# R0.11's second clause asks for "a ratifier signing key binding ratification receipts to the
# sovereign", and BootstrapReceipt.operator_ratification is already the pointer field for it.
#
# It is not implemented here because it requires asymmetric signatures, and the kernel has no
# crypto dependency: this package is stdlib-only and the api project declares only fastapi and
# uvicorn (neither `cryptography` nor `pynacl` is importable). Adding a large native dependency to
# a kernel whose entire purpose is to be minimal and estate-independent is an architectural
# decision, not an implementation detail.
#
# An HMAC would be the tempting stdlib answer and it is the WRONG one: anyone holding the key can
# forge a ratification, so it cannot bind an act to the sovereign — which is the whole requirement.
# A fake signature is worse than an absent one because it reads as binding.
#
# OPERATOR DECISION: adopt `cryptography` (ed25519) in the kernel, or keep the kernel dependency-
# free and site the ratifier key one layer out. Until then this seam stays honestly empty.
# ---------------------------------------------------------------------------------------------
