"""The P2 ceremony spine: every genesis stipulation, invoked, in order.

Until this module, the ceremony was a claim: each narrowing module carried present/accept
primitives and a docstring saying "the ceremony does this", while nothing in the kernel
actually performed the sequence (r3/r4, all seats: "the ceremony never invokes the mints —
the module is library-only"). This is the spine. It performs the stipulation ratifications in
their meaning order — the identity FIRST, because every later consent row is signed by the key
that identity binds; the registry next, because dispatch reads it; then the narrowing
stipulations the estate's first acts run under.

What this spine deliberately is NOT: the full P2 driver. Elicitation (the conversation that
PRODUCES the choices — which roles, which profile, which hosts, which forge) belongs to the
usher/executor layer (R2.1, R2.10); this spine takes the sovereign's already-elicited answers
as parameters and performs the consents. The first transmitting act remains where the ladder
puts it — this spine transmits nothing.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from .boot_profile import BootProfile
from .boot_profile import present as present_profile
from .boot_profile import ratified_profile
from .egress_consent import EgressAllowlist
from .egress_consent import accept as accept_allowlist
from .egress_consent import elicit_allowlist, ratified_allowlist
from .forge_choice import ForgeProfile
from .forge_choice import accept as accept_forge
from .forge_choice import present as present_forge
from .forge_choice import ratified_forge
from .ratification import ratify
from .role_registry import RoleSet, SovereignIdentity, _fingerprint_of, mint_genesis_identity


@dataclass(frozen=True)
class CeremonyResult:
    """What the ceremony consented to, in the order it was consented — ids, never prose."""

    sovereign_identity: str
    role_registry: str
    boot_profile: str
    egress_allowlist: str
    forge_choice: str


def ratify_genesis_stipulations(
    root: Path,
    *,
    principal: str,
    roles: tuple[str, ...],
    boot_profile: BootProfile,
    allowlist: EgressAllowlist,
    forge_profile: ForgeProfile,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> CeremonyResult:
    """Perform the P2 stipulation ceremony. Every act is the target module's own guarded
    entry point — this spine composes, it does not re-implement.

    A refusal anywhere propagates and stops the ceremony: a half-consented genesis is a true
    state (the chain holds exactly the consents given), and pending() names what remains.
    """
    mint_genesis_identity(
        root,
        principal=principal,
        roles=roles,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )

    present_profile(
        root, boot_profile, estate_id=estate_id, kernel_version=kernel_version, observed_at=observed_at
    )
    ratify(
        root,
        boot_profile.stipulation(),
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )

    elicit_allowlist(root, allowlist, estate_id=estate_id, kernel_version=kernel_version, observed_at=observed_at)
    accept_allowlist(
        root, allowlist, key_path=key_path, estate_id=estate_id, kernel_version=kernel_version, observed_at=observed_at
    )

    present_forge(
        root, forge_profile, estate_id=estate_id, kernel_version=kernel_version, observed_at=observed_at
    )
    accept_forge(
        root, forge_profile, key_path=key_path, estate_id=estate_id, kernel_version=kernel_version, observed_at=observed_at
    )

    return CeremonyResult(
        sovereign_identity=SovereignIdentity(principal, _fingerprint_of(key_path)).stipulation_id(),
        role_registry=RoleSet(roles).stipulation_id(),
        boot_profile=boot_profile.stipulation_id(),
        egress_allowlist=allowlist.stipulation_id(),
        forge_choice=forge_profile.stipulation_id(),
    )


def ceremony_complete(root: Path) -> bool:
    """Every genesis narrowing answered? Derived from the readers, never from a cursor.

    The forge/egress readers demand signing materials on enforcement paths; this completeness
    check is a READ, so it uses the explicit unauthenticated opt-in — it answers "is it on the
    chain", never "may I transmit".
    """
    return (
        ratified_profile(root) is not None
        and ratified_allowlist(root, allow_unauthenticated=True) is not None
        and ratified_forge(root, allow_unauthenticated=True) is not None
    )
