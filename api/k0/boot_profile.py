"""R3.6 — the boot profile is a detected-then-ratified stipulation, never an assumed pin.

The estate's first acts stand on a capability floor, and for as long as that floor is an
ASSUMPTION the ledger cannot say what the operator consented into. The requirements graph names
the defect exactly: "the pin is an unratified narrowing". The access-bootstrap amendment
(2026-07-09) gives the correction its shape: the boot profile becomes DETECTED-THEN-RATIFIED
rather than assumed — SURFACE_OBSERVE finds what the host already has, the ceremony presents the
narrowing with its trade-offs, and the sovereign's signature is what puts it in effect.

Two profiles are sanctioned today:

  existing-agent-harness      the operator already runs an agent CLI; the estate's first acts
                              ride it (actions >= {reason, implement, query, observe, verify},
                              authority ceiling repo_mutation, fallback hold, freshness DARK).
  hosted-model-kit-minimal    the operator has only provider keys; the kit-minimal harness does
                              the same acts through a sanctioned direct-key call path.

Detection is SURFACE_OBSERVE-class by construction: it looks at marker PRESENCE (a CLI on PATH,
an environment variable NAME) and never at a credential value, never at a network call. The first
transmitting act belongs to MEASURED_PROBE, after consent — nothing here may transmit.

`ratified_profile(root)` is the ONLY legal runtime source of the boot profile. Where no
boot-profile stipulation is ratified it returns None, and None renders dark — a default profile
substituted for a missing consent would be exactly the assumed pin this module exists to remove.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from enum import StrEnum
from pathlib import Path

from bootstrap_receipt import BootstrapAct, load_chain

from .ratification import Stipulation, _id_of, artifact_digest, propose

#: The authority-ceiling vocabulary's home. The graph pins the harness profile at
#: repo_mutation or below; OBSERVE_ONLY exists so a future profile can be BELOW it without
#: re-minting the vocabulary.
class AuthorityCeiling(StrEnum):
    REPO_MUTATION = "repo_mutation"
    OBSERVE_ONLY = "observe_only"


#: What the ceremony knows a boot profile to be. Keyed by profile_id; `stipulation_id()`
#: derives from the body digest, so a CHANGED definition is a new stipulation and an old
#: ratification can never silently cover new terms.
@dataclass(frozen=True)
class BootProfile:
    profile_id: str
    shape: str
    actions: frozenset[str]
    authority_ceiling: AuthorityCeiling
    fallback_policy: str
    freshness: str
    tradeoffs: tuple[str, ...]

    def body(self) -> bytes:
        """The exact bytes consented to. Canonical JSON so the digest is stable across readers."""
        return json.dumps(
            {
                "profile_id": self.profile_id,
                "shape": self.shape,
                "actions": sorted(self.actions),
                "authority_ceiling": str(self.authority_ceiling),
                "fallback_policy": self.fallback_policy,
                "freshness": self.freshness,
                "tradeoffs": list(self.tradeoffs),
            },
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def digest_short(self) -> str:
        return hashlib.sha256(self.body()).hexdigest()[:8]

    def stipulation_id(self) -> str:
        return f"boot-profile.{self.profile_id}.{self.digest_short()}"

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"BOOT PROFILE: {self.profile_id} — the estate's first acts run on this floor",
            self.body(),
        )


PROFILES: dict[str, BootProfile] = {
    p.profile_id: p
    for p in (
        BootProfile(
            profile_id="existing-agent-harness",
            shape="existing_agent_harness",
            actions=frozenset({"reason", "implement", "query", "observe", "verify"}),
            authority_ceiling=AuthorityCeiling.REPO_MUTATION,
            fallback_policy="hold",
            freshness="dark",
            tradeoffs=(
                "the estate's first acts inherit the harness's own blast radius and its own "
                "sign-in; the kit never provisions or repairs it",
                "a host without an installed, signed-in harness cannot take this floor — "
                "detection says so and the ceremony stalls dark instead of assuming it",
                "the harness's own provider terms bound what the estate may ask of it; the "
                "kit does not architect around them",
            ),
        ),
        BootProfile(
            profile_id="hosted-model-kit-minimal",
            shape="hosted_model",
            actions=frozenset({"reason", "implement", "query", "observe", "verify"}),
            authority_ceiling=AuthorityCeiling.REPO_MUTATION,
            fallback_policy="hold",
            freshness="dark",
            tradeoffs=(
                "every call is metered against the operator's own provider key; absent quota "
                "renders dark, never infinite",
                "the kit-minimal harness implements only the sanctioned call path — no raw "
                "client, no boutique launcher — so capability breadth is narrower until the "
                "first full harness is admitted",
                "key custody stays with the operator's store; the kit captures presence, "
                "never values",
            ),
        ),
    )
}

#: SURFACE_OBSERVE markers. CLI names of public agent harnesses — their PRESENCE on PATH is the
#: observation; nothing is executed and no version is probed (probing is MEASURED_PROBE's job,
#: post-consent).
HARNESS_MARKERS: tuple[str, ...] = ("claude", "codex", "gemini", "kimi", "grok")

#: Provider-key marker NAMES. Presence of the variable is the observation; the value is never
#: read, logged, or copied.
KEY_MARKERS: tuple[str, ...] = (
    "ANTHROPIC_API_KEY",
    "OPENAI_API_KEY",
    "GEMINI_API_KEY",
    "ZAI_API_KEY",
)


@dataclass(frozen=True)
class Detection:
    """What SURFACE_OBSERVE found, with its evidence — never a recommendation to assume."""

    profile_id: str
    evidence: tuple[str, ...]


def detect(
    *,
    which: Callable[[str], str | None] = shutil.which,
    environ: Mapping[str, str] = os.environ,
) -> tuple[Detection, ...]:
    """Observe the host's existing access, pre-auth. Empty tuple is the honest dark answer.

    An installed harness CLI implies the harness's own sign-in (the kit never touches it), so
    the harness profile is detected per marker found. Provider-key marker NAMES imply the
    hosted-model path. Both may be detected; the ceremony presents, the sovereign disposes.
    """
    out: list[Detection] = []
    harnesses = tuple(f"cli:{name} on PATH" for name in HARNESS_MARKERS if which(name))
    if harnesses:
        out.append(Detection("existing-agent-harness", harnesses))
    keys = tuple(
        f"env:{name} present (value unread)" for name in KEY_MARKERS if environ.get(name)
    )
    if keys:
        out.append(Detection("hosted-model-kit-minimal", keys))
    return tuple(out)


def present(
    root: Path,
    profile: BootProfile,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at=None,
) -> Path:
    """Put the detected profile to the sovereign: a HELD row, pending until ratified.

    The trade-offs are IN the consented bytes — the narrowing is what is signed, not a
    description of it. Refusals from the ceremony (already pending, already ratified) propagate
    unchanged: they are legal outcomes, not errors to hide.
    """
    return propose(
        root,
        profile.stipulation(),
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


@dataclass(frozen=True)
class RatifiedBootProfile:
    """The consented floor, read back from the ledger — with its currency, never without it."""

    profile_id: str
    current: bool


def ratified_profile(root: Path) -> RatifiedBootProfile | None:
    """The ONLY legal runtime source of the boot profile. None renders dark — never a default.

    Supersession is by recency: several boot-profile stipulations may be ratified over an
    estate's life (a redefinition is a NEW id, so consent-once is never violated), and the
    latest ratified row is the floor in effect. `current` is False when the ratified bytes are
    not the bytes the current definition would mint — the operator consented to an older shape
    of the narrowing, and that is reported, not repaired.
    """
    found: list[tuple[str, str | None]] = []
    for receipt in load_chain(root):
        if receipt.act is not BootstrapAct.RATIFIED:
            continue
        sid = _id_of(receipt)
        if sid.startswith("boot-profile."):
            found.append((sid, artifact_digest(receipt)))
    if not found:
        return None
    sid, pinned = found[-1]
    profile_id = sid[len("boot-profile.") :].rsplit(".", 1)[0]
    known = PROFILES.get(profile_id)
    current = (
        known is not None
        and pinned == hashlib.sha256(known.body()).hexdigest()
    )
    return RatifiedBootProfile(profile_id=profile_id, current=current)
