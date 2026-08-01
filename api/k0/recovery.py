"""Ratifier key rotation and the key-loss ceremony. (R0.11, third clause)

THE PROBLEM. SSHSIG binds a ratification to the sovereign's key. It has no answer for what happens
when that key is lost: existing ratifications stay verifiable, but no new one can be bound, and the
estate cannot ratify anything again. R0.11 named this and left it open.

THE TRAP THIS AVOIDS, measured before designing around it. `ssh-keygen -Y verify` validates against
the CURRENT time. Retiring a key with `valid-before` therefore breaks every ratification it ever
made:

    verify at now       -> "key has expired: verify time ... > valid-before ..."
    verify at 20260501  -> "Good signature ..."          (the time the signature was made)

So rotation is only non-destructive if verification supplies the ORIGINAL time. The receipt chain
already carries it — every BootstrapReceipt has `observed_at` — so the chain verifies itself across
rotations. Verifying a historical ratification at "now" is simply the wrong question.

WHERE ROTATION'S AUTHORITY COMES FROM, stated plainly. You cannot sign the retirement of a lost key
with the lost key. So rotation authority CANNOT come from the chain — it is grounded outside it, in
physical control of the durable root, exactly as the identity seed is. This is the same circularity
K0 already names: the ratification act cannot be ratified.

What the ceremony can honestly offer is not proof of authorisation but PROOF OF RECORD. A rotation
writes an auditable event naming both key fingerprints and the moment. A later auditor sees "at
time T, control of the durable root asserted a new ratifier key", cannot conclude the old sovereign
approved it, and knows to treat post-T ratifications as bound to a different key. That is the true
statement, and it is more useful than a false one.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

from .ratifier import RatifierError

#: OpenSSH allowed_signers timestamp format (UTC-suffixed so it is timezone-unambiguous). The
#: trailing Z requires OpenSSH >= 9.1 -- see host_floor, which declares that floor for this reason.
_TS = "%Y%m%d%H%M%SZ"


def _fmt(when: datetime) -> str:
    return when.astimezone(UTC).strftime(_TS)


@dataclass(frozen=True)
class SignerEntry:
    """One principal in the allowed-signers file, with its validity window."""

    principal: str
    public_key: str
    valid_after: datetime | None = None
    valid_before: datetime | None = None

    def render(self) -> str:
        opts = []
        if self.valid_after is not None:
            opts.append(f'valid-after="{_fmt(self.valid_after)}"')
        if self.valid_before is not None:
            opts.append(f'valid-before="{_fmt(self.valid_before)}"')
        prefix = f"{','.join(opts)} " if opts else ""
        return f"{self.principal} {prefix}{self.public_key.strip()}"


def write_signers(path: Path, entries: list[SignerEntry]) -> None:
    """Write the whole allowed-signers file. Retired keys are KEPT, with their windows closed —
    removing them would silently orphan every ratification they made."""
    if not entries:
        raise RatifierError("refusing to write an empty allowed-signers file: no key could ratify")
    for e in entries:
        if not e.principal.strip() or " " in e.principal.strip():
            raise RatifierError(f"principal {e.principal!r} must be non-empty and space-free")
        if not e.public_key.strip().startswith(("ssh-", "sk-", "ecdsa-")):
            raise RatifierError(f"{e.principal}: not an OpenSSH public key")
        if e.valid_after and e.valid_before and e.valid_after >= e.valid_before:
            raise RatifierError(
                f"{e.principal}: valid-after is not before valid-before — the key would never be "
                f"usable, which is a silently dead ratifier"
            )
    path.write_text("\n".join(e.render() for e in entries) + "\n", encoding="utf-8")


def rotate(
    *,
    retiring: SignerEntry,
    successor_principal: str,
    successor_public_key: str,
    at: datetime | None = None,
) -> list[SignerEntry]:
    """Produce the post-rotation signer set: old key window-closed, new key window-opened.

    Both keys are retained. The retired one keeps verifying its own historical ratifications when
    verification supplies their `observed_at`; it can no longer bind new ones.
    """
    at = (at or datetime.now(UTC)).astimezone(UTC)
    if retiring.valid_after is not None and at <= retiring.valid_after:
        raise RatifierError(
            "rotation instant is not after the retiring key became valid — that would leave it "
            "never-usable and silently erase the ratifications it made"
        )
    if retiring.principal == successor_principal:
        raise RatifierError(
            "successor must use a DIFFERENT principal: the allowed-signers file is keyed by "
            "principal, and reusing it makes the two keys indistinguishable to an auditor"
        )
    return [
        SignerEntry(
            principal=retiring.principal,
            public_key=retiring.public_key,
            valid_after=retiring.valid_after,
            valid_before=at,
        ),
        SignerEntry(
            principal=successor_principal,
            public_key=successor_public_key,
            valid_after=at,
        ),
    ]


def rotation_record(
    *,
    retiring_principal: str,
    successor_principal: str,
    reason: str,
    at: datetime | None = None,
) -> dict[str, str]:
    """The auditable payload for a KEY_ROTATION act.

    `reason` is required and must distinguish a planned rotation from a key LOSS, because the two
    have different trust consequences: a planned rotation can be signed by the outgoing key, a loss
    cannot be signed by anything and rests only on control of the durable root.
    """
    if not reason.strip():
        raise RatifierError(
            "rotation requires a stated reason: an auditor must be able to tell a planned rotation "
            "from a key loss, because only the former can be authorised by the outgoing sovereign"
        )
    at = (at or datetime.now(UTC)).astimezone(UTC)
    return {
        "act": "rotated",
        "retiring_principal": retiring_principal,
        "successor_principal": successor_principal,
        "reason": reason.strip(),
        "rotated_at": at.isoformat(),
        "authority": (
            "control of the durable root. Rotation authority cannot derive from the receipt chain: "
            "a lost key cannot sign its own retirement. This record proves WHEN, not that the "
            "outgoing sovereign approved it."
        ),
    }
