"""R2.13 — the forge and delivery rail is a ratified stipulation, never an assumption.

The estate's own rails are GitHub-throughout with repo-specific merge governance — but that is
THIS estate's answer, and a stranger's kit that hardcoded it would be assuming an answer the
sovereign never gave. The graph gap: "forge choice as ratified stipulation (GitHub-only ceiling
OR forge-agnostic) — delivery + governance rails are IN the functioning floor; local-git-only
degraded posture as a ratifiable option".

The form is the K0 ceremony shape this kernel now uses for every consented narrowing (the third
instance — boot profile, egress allowlist, now forge choice — and the shared scaffolding is a
recorded consolidation follow-up, not a third reinvention by silence):

  * THREE LEGAL ANSWERS, each with its trade-offs in the consented bytes. `local-git-only` is
    the DEGRADED posture — no delivery rail, no governance rail — and it is ratifiable like any
    other narrowing: the operator consents to operating without a forge, with the cost named.
  * PRESENTED AND RATIFIED through the existing ceremony (propose → sign → witness), body
    durable before the consent that pins it, verified against the chain at every read.
  * `ratified_forge(root, ...)` is the ONLY legal runtime source of the choice: unratified
    renders dark (never a default forge), authentication status is data, and the hash-only read
    requires an explicit opt-in. HONESTY NOTE (codex r1): no consumer reads this yet — the
    rail-bearing consumers arrive with the delivery work, and the "only legal source" claim is
    this kernel's law, witnessed here only by the ceremony round-trip. When a consumer lands,
    its read is the integration witness.
"""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from pathlib import Path

from bootstrap_receipt import BootstrapAct, load_chain, verify_chain_at

from .degradation import _write_body_durably
from .ratification import (
    SIGNATURE_DIRNAME,
    Stipulation,
    _id_of,
    artifact_digest,
    propose,
    ratify,
)
from .refusal import Refusal


class ForgeChoice(StrEnum):
    """The three legal answers. The vocabulary is closed: a fourth rail is a governance act."""

    GITHUB_ONLY = "github-only-ceiling"  # delivery + governance rails are GitHub's
    FORGE_AGNOSTIC = "forge-agnostic"  # the rails are abstracted; any forge plugs in
    LOCAL_GIT_ONLY = "local-git-only"  # DEGRADED: no forge at all — local git, no rails


@dataclass(frozen=True)
class ForgeProfile:
    """One legal forge posture, with the cost of choosing it in the consented bytes."""

    choice: ForgeChoice
    tradeoffs: tuple[str, ...]

    def body(self) -> bytes:
        """The exact bytes consented to. Canonical JSON so the digest is stable across readers."""
        return json.dumps(
            {"choice": str(self.choice), "tradeoffs": list(self.tradeoffs)},
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def digest_short(self) -> str:
        return hashlib.sha256(self.body()).hexdigest()[:16]

    def stipulation_id(self) -> str:
        return f"forge-choice.{self.choice.value}.{self.digest_short()}"

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"FORGE CHOICE: {self.choice.value} — the delivery and governance rails run here",
            self.body(),
        )


FORGE_PROFILES: dict[ForgeChoice, ForgeProfile] = {
    p.choice: p
    for p in (
        ForgeProfile(
            ForgeChoice.GITHUB_ONLY,
            tradeoffs=(
                "the delivery and governance rails are GitHub's — a forge outage is a floor "
                "outage, and the dependency is consented, not discovered",
                "forge-portable work is out of scope until this choice is amended; the ceiling "
                "is the point",
            ),
        ),
        ForgeProfile(
            ForgeChoice.FORGE_AGNOSTIC,
            tradeoffs=(
                "every rail integration is written against the abstraction first — slower now, "
                "portable forever; the indirection is consented",
                "governance features that exist on only one forge degrade to the least common "
                "rail until ratified otherwise",
            ),
        ),
        ForgeProfile(
            ForgeChoice.LOCAL_GIT_ONLY,
            tradeoffs=(
                "DEGRADED: no delivery rail and no governance rail — reviews, merges, and "
                "releases are local git acts with no remote, consented as a posture",
                "lift condition: ratify one of the rail-bearing choices when a forge exists",
            ),
        ),
    )
}


class ForgeConsentError(RuntimeError):
    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


def _require_sanctioned(profile: ForgeProfile) -> None:
    """The profile must BE a registry object (codex r2 major): a caller-constructed profile
    could carry empty or invented trade-offs, and the trade-offs are what the sovereign signs.
    Identity — the same discipline as the key-capture wire."""
    if all(profile is not p for p in FORGE_PROFILES.values()):
        raise ForgeConsentError(
            f"{profile.choice.value}: not a registry profile — the sovereign signs only the "
            "sanctioned trade-offs, never caller-invented ones",
            Refusal(
                gate="forge.profile-registry",
                why="a profile outside FORGE_PROFILES carries costs nobody vetted",
                legal_next="present one of FORGE_PROFILES; a new posture is a governance act",
            ),
        )


def present(
    root: Path,
    profile: ForgeProfile,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Put the forge choice to the sovereign: a HELD row, pending until ratified."""
    _require_sanctioned(profile)
    return propose(
        root,
        profile.stipulation(),
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def accept(
    root: Path,
    profile: ForgeProfile,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The sovereign consents to the rails. The body is durable BEFORE the row that pins it."""
    _require_sanctioned(profile)
    stip = profile.stipulation()
    _write_body_durably(root / SIGNATURE_DIRNAME / f"{stip.stipulation_id}.body", profile.body())
    return ratify(
        root,
        stip,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def _ratified_rows(root: Path) -> list:
    return [
        receipt
        for receipt in load_chain(root)
        if receipt.act is BootstrapAct.RATIFIED
        and _id_of(receipt).startswith("forge-choice.")
    ]


@dataclass(frozen=True)
class RatifiedForge:
    """The consented rails, with amendment count and authentication status as data."""

    choice: ForgeChoice
    tradeoffs: tuple[str, ...]
    amendments: int
    signature_verified: bool


def ratified_forge(
    root: Path,
    *,
    allowed_signers: Path | None = None,
    principal: str | None = None,
    scratch_dir: Path | None = None,
    allow_unauthenticated: bool = False,
) -> RatifiedForge | None:
    """The ONLY legal runtime source of the forge choice. None renders dark — never a default.

    Same discipline as the egress allowlist: chain verified first, signature authenticated when
    the materials are supplied (mandatory on enforcement paths), the hash-only read requires an
    explicit opt-in, and the id binds the body's digest.
    """
    if not verify_chain_at(root).ok:
        raise ForgeConsentError(
            "the bootstrap chain fails verification — no row in it may ground the forge choice",
            Refusal(
                gate="forge.chain-integrity",
                why="an unverifiable chain cannot prove what was consented to",
                legal_next="run verify_chain to find the break, restore from backup, then re-run",
            ),
        )
    rows = _ratified_rows(root)
    if not rows:
        return None
    receipt = rows[-1]
    sid = _id_of(receipt)
    gate = "forge.choice-integrity"

    materials_supplied = allowed_signers is not None and principal is not None and scratch_dir is not None
    if not materials_supplied and not allow_unauthenticated:
        raise ForgeConsentError(
            "the hash-only read must be opted into explicitly (allow_unauthenticated=True)",
            Refusal(
                gate="forge.signature-verification",
                why="the consent row's signature was not verified and the caller did not "
                "explicitly accept that",
                legal_next="pass the signing materials, or allow_unauthenticated=True and render it",
            ),
        )
    signature_ok = False
    verified_sids: tuple[str, ...] = ()
    if materials_supplied:
        from .ratification import verify_ratifications
        from .refusal import RefusalError

        try:
            verdict = verify_ratifications(
                root, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
            )
            ok = sid in verdict.verified
            why = dict(verdict.unverified).get(sid, "not present among the verified ratifications")
            verified_sids = verdict.verified if ok else ()
            if ok and not verdict.ok:
                ok = False
                why = f"unverified ratification rows present: {[u for u, _ in verdict.unverified]}"
        except RefusalError as exc:
            ok = False
            why = f"the ratifier REFUSED: {exc}"
        if not ok:
            raise ForgeConsentError(
                f"{sid}: the ratification does not verify against the sovereign's key: {why}",
                Refusal(
                    gate="forge.signature-verification",
                    why="a consent row that does not verify is not consent",
                    legal_next="verify_ratifications() shows every unverified row with its reason",
                ),
            ) from None
        signature_ok = True

    pinned = artifact_digest(receipt)
    if pinned is None:
        raise ForgeConsentError(
            f"{sid}: the ratified forge row pins NO artifact digest",
            Refusal(
                gate=gate,
                why="the row claims ratification without naming what was ratified",
                legal_next="verify_chain; establish where the stipulation ref went",
            ),
        )
    path = root / SIGNATURE_DIRNAME / f"{sid}.body"
    try:
        raw = path.read_bytes()
    except OSError as exc:
        raise ForgeConsentError(
            f"{sid}: the ratified forge body cannot be read ({exc.strerror or 'missing'})",
            Refusal(
                gate=gate,
                why="the chain consents to an artifact that is gone or unreadable",
                legal_next="restore the body from backup, or present and accept a fresh choice",
            ),
        ) from None
    actual = hashlib.sha256(raw).hexdigest()
    if not re.fullmatch(r"forge-choice\.[a-z0-9-]+\.[0-9a-f]{16}", sid) or sid.rsplit(".", 1)[1] != actual[:16]:
        raise ForgeConsentError(
            f"{sid}: the stipulation id does not bind this body",
            Refusal(
                gate=gate,
                why="an id that does not embed its artifact's digest lets any signed row wear "
                "the forge prefix",
                legal_next="verify_chain; the id/body pair was minted apart — establish which moved",
            ),
        )
    if actual != pinned:
        raise ForgeConsentError(
            f"{sid}: the stored body does not hash to the digest the chain pins",
            Refusal(
                gate=gate,
                why="the stored body is not the artifact the operator ratified",
                legal_next="restore the pinned body, or present and accept the changed terms",
            ),
        )
    try:
        parsed = json.loads(raw.decode("utf-8"))
        if not isinstance(parsed, dict) or set(parsed) != {"choice", "tradeoffs"}:
            raise TypeError("a forge body is exactly {'choice', 'tradeoffs'}")
        choice = ForgeChoice(parsed["choice"])
        tradeoffs = parsed["tradeoffs"]
        if not isinstance(tradeoffs, list) or not all(isinstance(t, str) for t in tradeoffs):
            raise TypeError("tradeoffs is not a list of strings")
    except (UnicodeDecodeError, ValueError, KeyError, TypeError):
        raise ForgeConsentError(
            f"{sid}: the consented forge body is not decodable as a forge choice",
            Refusal(
                gate=gate,
                why="the consented bytes are unusable",
                legal_next="restore the body from backup, or present and accept a fresh choice",
            ),
        ) from None
    if signature_ok:
        amendments = sum(1 for s in verified_sids if s.startswith("forge-choice.")) - 1
    else:
        amendments = sum(1 for r in rows if artifact_digest(r) is not None) - 1
    return RatifiedForge(
        choice=choice,
        tradeoffs=tuple(tradeoffs),
        amendments=amendments,
        signature_verified=signature_ok,
    )
