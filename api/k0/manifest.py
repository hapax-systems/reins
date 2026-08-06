"""K0 — the Stage-0 kernel manifest, as DATA (R0.4). RATIFIED 2026-08-01.

Lives INSIDE the kernel package so K0 can attest itself: before this, genesis_self_attest()
had to be TOLD its own manifest hash by the caller, which means the kernel could be made to
attest a manifest that was not its own. Now the pin is computed from the membership the
package actually carries.

Mirrors the estate's Ladder-as-data precedent (a statechart carried as a frozen dataclass with
runtime invariant checks): the structure
is a frozen dataclass, the canonical value is a module constant, and the invariants are machine
checks rather than prose.

THE FIXED POINT (from first-init-ratification-ceremony-design-2026-07-09.md §2):

    K0 is the LEAST set of mechanisms
      (a) presupposed by every bootstrap act, and
      (b) not installable by any governed bootstrap act.

Both halves are load-bearing. (a) alone would admit anything convenient; (b) alone would admit
anything merely awkward to install. A member must satisfy BOTH, and this module refuses a manifest
where any member fails either -- that is what makes the criterion machine-checkable instead of a
claim in a design document.

WHY NOT INSTALLABLE: each member carries a circularity witness naming the act that would have to
install it and the reason that act presupposes it. You cannot ratify the ratification act, receipt
the receipt primitive's own installation, or fail-closed-guard the fail-closed default. Circularity
is the membership test, not an inconvenience.

DRIFT PIN: the manifest carries a sha256 over its own canonical serialization. K0 changing silently
is the failure this pin exists to prevent -- kernel replacement is legal ONLY under enforce-flip
(P5), never as a side effect of an edit.

NOTE ON A CITED PRECEDENT: the design docs attribute this pin to a "CEILING_DENOTATION pin
precedent". No such symbol exists anywhere in the estate (verified 2026-08-01), and no drift-pin
pattern exists in council/shared or reins. This module therefore ESTABLISHES the pattern rather
than copying it. Recorded so the next reader does not go looking for a precedent that is not there.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from typing import Mapping

#: The GOVERNED bootstrap acts. K0-core membership is quantified over ALL of these.
#:
#: RATIFIED 2026-08-01 (option C). Two corrections to the design's §2 list, both forced:
#:   * `probe` and `reconcile` ADDED -- §7's receipt enum records them and they are genuine acts.
#:   * `install` REMOVED -- it precedes K0 (kit signature verification and durable-root are pre-K0
#:     facts). K0 cannot be quantified over an act that runs before the kernel arms.
BOOTSTRAP_ACTS: tuple[str, ...] = ("elicit", "ratify", "mint", "probe", "reconcile", "flip")

#: How an act ENDS. §7's receipt enum lists these alongside acts, which is what made the act set
#: look like nine. A disposition presupposes nothing on its own; it is a property of an act.
DISPOSITIONS: tuple[str, ...] = ("refused", "held", "escaped")

#: Acts that run BEFORE K0 arms. Not governed by the kernel, so not quantified over.
PRE_K0_ACTS: tuple[str, ...] = ("install",)

#: The typed lever lattice (R0.8). Five are mandatory at boot; the kernel claim is specifically
#: that the NORMATIVE-CONSTRAINT class is wholly contained in K0, while the others contribute only
#: a minimal seed. Recording all seven keeps the containment claim falsifiable.
LEVER_CLASSES: tuple[str, ...] = (
    "normative-constraint",   # wholly in K0
    "state-context",          # seed only: receipt primitive + identity seed
    "elicitation",            # seed only: the ratification act
    "loop-control",           # seed only: FSM phase-legality law
    "verification-gating",    # boots mandatory, but installable by a governed act => NOT K0
    "affordance-grant",       # boots mandatory, installable => NOT K0
    "resource-envelope",      # degenerate-but-typed at boot => NOT K0
)


#: The two tiers, ratified 2026-08-01 (option C).
#:
#: WHY TWO TIERS. The fixed point as originally stated was a CONJUNCTION: presupposed by every act
#: AND not installable. `ratification-act` satisfies the second and fails the first -- the phase
#: order is K0_ACTIVE -> HOST_RECONCILE -> STIPULATION_RATIFY, so a reconcile happens BEFORE any
#: ratification and therefore does not presuppose it. Yet you cannot ratify the ratification act,
#: so nothing can install it either. A mechanism can be non-installable AND not universally
#: presupposed; the conjunction had no room for that. The tiers name the exception instead of
#: hiding it.
TIERS: tuple[str, ...] = ("core", "seed")


@dataclass(frozen=True)
class KernelMember:
    """One K0 member, with the evidence for its tier's admission test."""

    id: str
    lever_class: str
    #: "core" -- presupposed by ALL governed acts. "seed" -- constitutive of the regime instead.
    tier: str
    #: Which governed acts presuppose this member. For tier "core", must be ALL of BOOTSTRAP_ACTS.
    presupposed_by: tuple[str, ...]
    #: The act that would have to install it, and why that is circular. Required for BOTH tiers:
    #: non-installability is what makes something kernel at all.
    circularity_witness: str
    #: SEED ONLY. What regime this member constitutes, and why that regime cannot bootstrap itself.
    #: This is the seed tier's admission test -- without it, "seed" would be a place to dump
    #: anything that failed the core test, which would gut the criterion.
    constitutive_of: str = ""


@dataclass(frozen=True)
class KernelManifest:
    kernel_version: str
    members: tuple[KernelMember, ...]
    #: Facts that must hold BEFORE K0 arms. Deliberately NOT members: they are install-time
    #: preconditions, established by the distribution and the host, not by the kernel.
    pre_k0: tuple[str, ...]

    def canonical(self) -> str:
        """Stable serialization. Sorting makes the pin insensitive to declaration order."""
        return json.dumps(
            {
                "kernel_version": self.kernel_version,
                "members": [
                    {
                        "id": m.id,
                        "lever_class": m.lever_class,
                        "tier": m.tier,
                        "presupposed_by": sorted(m.presupposed_by),
                        "circularity_witness": m.circularity_witness,
                        "constitutive_of": m.constitutive_of,
                    }
                    # sort by id, not by dict: declaration order must not move the pin
                    for m in sorted(self.members, key=lambda x: x.id)
                ],
                "pre_k0": sorted(self.pre_k0),
            },
            sort_keys=True,
            separators=(",", ":"),
        )

    def drift_pin(self) -> str:
        return hashlib.sha256(self.canonical().encode("utf-8")).hexdigest()


K0 = KernelManifest(
    kernel_version="0.1.0",
    members=(
        KernelMember(
            id="fail-closed-default",
            tier="core",
            lever_class="normative-constraint",
            presupposed_by=BOOTSTRAP_ACTS,
            circularity_witness=(
                "A governed act installing the fail-closed default would itself run either "
                "fail-closed (presupposing it) or fail-open (violating it before it exists). "
                "You cannot fail-closed-guard the fail-closed default."
            ),
        ),
        KernelMember(
            id="refusal-as-data",
            tier="core",
            lever_class="normative-constraint",
            presupposed_by=BOOTSTRAP_ACTS,
            circularity_witness=(
                "Every act can refuse, including the act that would install refusal. A refusal "
                "emitted before refusal-as-data exists has no legible form and cannot be received."
            ),
        ),
        KernelMember(
            id="receipt-primitive",
            tier="core",
            lever_class="state-context",
            presupposed_by=BOOTSTRAP_ACTS,
            circularity_witness=(
                "Installing the append-only hash-chained receipt primitive is itself an act that "
                "must be receipted. Its own installation receipt has nowhere to land. The genesis "
                "receipt is a kernel SELF-attest for exactly this reason."
            ),
        ),
        KernelMember(
            id="identity-seed",
            tier="core",
            lever_class="state-context",
            presupposed_by=BOOTSTRAP_ACTS,
            circularity_witness=(
                "Receipts are chained under an estate/operator identity. Minting that identity is "
                "an act requiring a receipt, which requires the identity to attribute it to."
            ),
        ),
        KernelMember(
            id="ratification-act",
            lever_class="elicitation",
            tier="seed",
            # NOT presupposed by probe/reconcile: the phase order is
            # K0_ACTIVE -> HOST_RECONCILE -> STIPULATION_RATIFY, so both occur before any
            # ratification exists. Recorded honestly rather than asserted as universal.
            presupposed_by=("elicit", "ratify", "mint", "flip"),
            circularity_witness=(
                "You cannot ratify the ratification act. Its authority cannot derive from an "
                "application of itself."
            ),
            constitutive_of=(
                "The stipulative regime. Every ratified row, and so every stipulation the estate "
                "later relies on, derives its authority from this act. The regime cannot bootstrap "
                "itself: an act that established it would need authority the regime alone confers. "
                "It is kernel not because every act needs it, but because nothing can install it "
                "and everything stipulative rests on it."
            ),
        ),
        KernelMember(
            id="fsm-phase-legality-law",
            tier="core",
            lever_class="loop-control",
            presupposed_by=BOOTSTRAP_ACTS,
            circularity_witness=(
                "Whether an act is legal in the current phase must be answerable BEFORE that act "
                "runs, including the act that would install the legality law. Carried as data per "
                "the Ladder-as-data precedent (sdlc_invariants.SDLC_LADDER)."
            ),
        ),
    ),
    pre_k0=(
        "kit-signature-verified-before-first-run",
        "durable-root-declared",
    ),
)


class KernelManifestError(Exception):
    """Raised when a manifest violates the fixed point. Fail closed: never warn and proceed."""


def verify(manifest: KernelManifest = K0, *, expect_pin: str | None = None) -> str:
    """Check the fixed point and (optionally) the drift pin. Returns the pin.

    Fail-closed by construction: every failure raises. There is no 'mostly valid' kernel.
    """
    if not manifest.members:
        raise KernelManifestError("K0 is empty: a kernel with no members presupposes nothing")

    seen: set[str] = set()
    for m in manifest.members:
        if m.id in seen:
            raise KernelManifestError(f"duplicate member: {m.id}")
        seen.add(m.id)

        if m.tier not in TIERS:
            raise KernelManifestError(f"{m.id}: unknown tier {m.tier!r}")

        # half (a) — core only: presupposed by EVERY governed act
        missing = set(BOOTSTRAP_ACTS) - set(m.presupposed_by)
        if m.tier == "core" and missing:
            raise KernelManifestError(
                f"{m.id}: core member not presupposed by {sorted(missing)} — presupposed by SOME "
                f"acts is not core, it is merely early. If it is non-installable and constitutive, "
                f"it belongs in tier 'seed' WITH a constitutive_of witness."
            )

        # the seed tier's own admission test — otherwise "seed" becomes a dumping ground for
        # anything that failed the core test, which would gut the criterion.
        if m.tier == "seed":
            if not m.constitutive_of.strip():
                raise KernelManifestError(
                    f"{m.id}: seed member with no constitutive_of witness — the seed tier admits "
                    f"only mechanisms that CONSTITUTE a regime which cannot bootstrap itself"
                )
            if not missing:
                raise KernelManifestError(
                    f"{m.id}: presupposed by every act, so it is core, not seed — the seed tier is "
                    f"the exception, never the default"
                )
        elif m.constitutive_of.strip():
            raise KernelManifestError(
                f"{m.id}: core member carries a constitutive_of witness — that field is the seed "
                f"tier's admission test and must not be used to soften a core claim"
            )
        unknown = set(m.presupposed_by) - set(BOOTSTRAP_ACTS)
        if unknown:
            raise KernelManifestError(f"{m.id}: unknown bootstrap acts {sorted(unknown)}")

        # half (b): a non-installability witness
        if not m.circularity_witness.strip():
            raise KernelManifestError(
                f"{m.id}: no circularity witness — membership requires showing that installing it "
                f"presupposes it"
            )

        if m.lever_class not in LEVER_CLASSES:
            raise KernelManifestError(f"{m.id}: unknown lever class {m.lever_class!r}")

    # the containment claim: normative-constraint is wholly in K0, so it must actually appear
    if not any(m.lever_class == "normative-constraint" for m in manifest.members):
        raise KernelManifestError(
            "no normative-constraint member: the design's containment claim would be vacuous"
        )

    # the seed tier is the exception; if it ever outgrows core the criterion has been abandoned
    n_seed = sum(1 for m in manifest.members if m.tier == "seed")
    n_core = sum(1 for m in manifest.members if m.tier == "core")
    if n_seed >= n_core:
        raise KernelManifestError(
            f"seed tier ({n_seed}) is not smaller than core ({n_core}) — seed is the named "
            f"exception, not a second kernel"
        )

    # pre-K0 facts must not smuggle themselves in as members
    overlap = set(manifest.pre_k0) & seen
    if overlap:
        raise KernelManifestError(
            f"pre-K0 facts declared as members: {sorted(overlap)} — install-time preconditions are "
            f"established by the distribution and host, not by the kernel"
        )

    pin = manifest.drift_pin()
    if expect_pin is not None and pin != expect_pin:
        raise KernelManifestError(
            f"K0 DRIFT: manifest hashes to {pin}, expected {expect_pin}. The kernel may be replaced "
            f"only under enforce-flip (P5), never as a side effect of an edit."
        )
    return pin


#: THE RATIFIED PIN. Operator-of-record, 2026-08-01, option C (5 core + 1 seed).
#:
#: This is a LITERAL, not a computation. An earlier draft wrote `K0_DRIFT_PIN = verify(K0)`, which
#: pinned nothing: the digest was derived from whatever membership happened to be present, so any
#: edit silently moved the pin and every check still passed. A drift pin that recomputes itself is
#: not a pin. The ratified value is written down, and the module refuses to import if the manifest
#: no longer hashes to it.
#:
#: Changing this line is changing what the kernel IS. It moves only under enforce-flip (P5) with a
#: KERNEL_UPGRADE receipt (R6.5) — never as a side effect of editing membership.
RATIFIED_PIN = "b604b52bfdd9e267b7a5b68f42d020f233065f3c6d77eeb9f244de2d78ee6d59"

#: Import-time enforcement: an unratified kernel does not load.
K0_DRIFT_PIN = verify(K0, expect_pin=RATIFIED_PIN)



# --- The exclusion ledger: what makes MINIMALITY falsifiable -------------------------------
#
# The fixed point says K0 is the LEAST such set. A checker can verify each member meets the
# criterion; it cannot, on its own, verify that nothing is MISSING and nothing SUPERFLUOUS.
# Minimality was therefore asserted, not proven -- which is exactly what blocks confident
# ratification.
#
# This ledger closes that gap by CLOSED-WORLD discipline, the same shape as the registry's
# REQUIRED_ROUTE_IDS frozen-set equality check: every kernel-adjacent mechanism the P0 design
# enumerates is classified EXACTLY ONCE, either
#   - in K0, carrying a circularity witness (see members above), or
#   - out of K0, naming the governed act that INSTALLS it.
#
# An excluded candidate with no installer is a minimality violation: it means the mechanism is
# non-installable and therefore belongs in K0. A member that also appears here with an installer
# is the opposite violation. Both fail the check.
#
# Candidates are drawn from the P0 requirements (R0.1-R0.12) of
# first-init-requirements-graph-2026-07-09.yaml -- the design's own enumeration of the
# kit-and-kernel surface -- so the world is closed against a stated source, not against intuition.

#: mechanism -> the governed act that installs it (why it is NOT kernel)
EXCLUDED: Mapping[str, str] = {
    "kit-distribution-integrity": (
        "R0.1 — installed by `install`: signature verification is performed BY the distribution "
        "before K0 arms. Pre-K0, not kernel."
    ),
    "durable-root-declaration": (
        "R0.7 — installed by `install`, re-attested by `reconcile`. A host fact, established "
        "before the kernel and re-checked after; the kernel consumes it."
    ),
    "stage0-kernel-package": (
        "R0.3 — installed by `install`. The PACKAGE is the delivery vehicle for K0; it is not "
        "itself presupposed by the acts. Confusing the vehicle with the cargo is the trap here."
    ),
    "bootstrap-phase-fsm": (
        "R0.6 — installed by `install`. NOTE the split: the phase-legality LAW is kernel "
        "(carried as data); the FSM that EXECUTES it is ordinary code and is installable."
    ),
    "single-instance-lock": (
        "R0.6 — installed by `install`. Prevents concurrent first-inits forking the chain, but a "
        "lock is not presupposed by an act; it constrains concurrency of acts."
    ),
    "boot-lever-class-mapping": (
        "R0.8 — installed by `ratify`. It is a stipulation ABOUT the lattice, not a mechanism the "
        "acts presuppose."
    ),
    "portable-doctrine-corpus": (
        "R0.9 — installed by `install`. Teaching material shipped in the kit; acts are legal "
        "without it, merely less legible."
    ),
    "pii-consent-scanner": (
        "R0.10 — installed by `install`, its patterns by `ratify`. Mandatory before the first "
        "model call (a MEASURED_PROBE precondition), which is later than K0."
    ),
    "ratifier-signing-key": (
        "R0.11 — installed by `mint`. The identity SEED is kernel; the KEY binding receipts to "
        "the sovereign is minted during the ceremony and is replaceable by a recovery ceremony."
    ),
    "purge-registration-law": (
        "R0.12 — installed by `ratify`. A law about stores created at genesis; it presupposes the "
        "receipt primitive rather than being presupposed by every act."
    ),
    "license-publication-posture": (
        "R0.2 — installed by `ratify` (RATIFIED 2026-07-30). A posture decision, not a mechanism."
    ),
}


def verify_minimality(manifest: KernelManifest = K0) -> int:
    """Closed-world check. Returns the number of classified candidates.

    Fails closed on: a candidate classified both ways, an exclusion with no named installer, or
    an exclusion whose stated installer is not a bootstrap act.
    """
    member_ids = {m.id for m in manifest.members}

    both = member_ids & set(EXCLUDED)
    if both:
        raise KernelManifestError(
            f"classified twice: {sorted(both)} — a mechanism is either presupposed-and-circular "
            f"(in K0) or installable (excluded), never both"
        )

    for mech, reason in EXCLUDED.items():
        if not reason.strip():
            raise KernelManifestError(
                f"{mech}: excluded with no installer named. An excluded mechanism with no "
                f"installing act is non-installable, which means it BELONGS IN K0 — this is the "
                f"minimality violation the ledger exists to catch."
            )
        # A pre-K0 act is a legitimate installer: the kit delivers things before the kernel arms.
        # Excluding `install` from the GOVERNED acts (option C) does not stop it installing.
        installers = tuple(BOOTSTRAP_ACTS) + tuple(PRE_K0_ACTS)
        if not any(f"`{act}`" in reason for act in installers):
            raise KernelManifestError(
                f"{mech}: exclusion does not name an installing act (got: {reason[:60]}...)"
            )
    return len(member_ids) + len(EXCLUDED)


def kernel_identity() -> dict[str, str]:
    """What the kernel says it is. Feed this to genesis_self_attest so the kernel attests the
    manifest it actually carries, rather than one a caller hands it."""
    return {"kernel_version": K0.kernel_version, "kernel_manifest_sha256": K0_DRIFT_PIN}


if __name__ == "__main__":  # pragma: no cover
    print(f"kernel_version {K0.kernel_version}")
    print(f"members        {len(K0.members)}")
    print(f"drift pin      {K0_DRIFT_PIN}")
