"""K0 — the lifecycle-choice and degradation ledger. (R2.6)

The assembly L6 lattice is `FULL > DEGRADED > HELD > REFUSED`, *always receipted*. This module is
the "always receipted" half, and it exists because of a defect this estate can point at rather than
imagine.

## The canonical case, which is not hypothetical

R2.6's own gap names it: **the one-family review deficit**. A review floor requiring two
independent model families, on a host that can seat only one, is a system running DEGRADED. That
was observed in practice, and it was reported as a *quota* problem — which it was not. The system
was operating below its own floor and said so nowhere a person would look.

(No host is named here on purpose: K0 is estate-independent by construction, and `test_k0.py`
enforces it. The incident belongs in the estate's records, not in the kernel that any stranger
runs.)

That is the failure this ledger removes. Not "detect degradation" — the dispatcher detected it
fine. **Record that the estate CHOSE to keep going, what was given up, and what would restore it.**

## Why a degradation must be ratified

A degradation is a decision to accept less, and accepting less is the sovereign's call, not the
kernel's. So a degradation is not a status field somebody sets; it is a `Stipulation` put through
the ratification act (R2.8) and signed. An unratified degradation is a system quietly deciding what
the operator is willing to live without.

This is also why `tradeoff` and `lift_condition` are REQUIRED and non-empty. A degradation with no
stated cost cannot be consented to — the operator would be agreeing to an unnamed loss. One with no
lift condition is a permanent downgrade wearing a temporary name, and it is the same dead-end that
`refusal.py` refuses to construct (INV-3: BLOCKED always escapes).

## Honest-dark, not silently-full

`state()` returns the CURRENT lifecycle per subject, derived from the chain — and a subject that was
degraded and never lifted stays degraded. Nothing decays back to FULL on its own; only a ratified
lift moves it up. The auto-lift wiring itself is R6.2 and is deliberately not here: this module
records and renders, it does not decide.

`render()` produces the projection the Reins representation kernel demands — *"a datum may remain
visible while some context is unavailable only by rendering partial/DARK with the missing reasons
and with semantic actions held"*. A degraded subject renders with its deficit and its lift
condition attached, so the surface teaches from the row rather than showing a number that lies.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from pathlib import Path

from bootstrap_receipt import BootstrapAct, load_chain

from .ratification import (
    RatificationError,
    Stipulation,
    _id_of,
    artifact_digest,
    propose,
    ratify,
)
from .refusal import Refusal


class Lifecycle(StrEnum):
    """The L6 lattice, ordered. Lower is less capable."""

    FULL = "full"
    DEGRADED = "degraded"
    HELD = "held"
    REFUSED = "refused"


#: Explicit order, as data. Deriving it from the enum's declaration order would make the lattice an
#: artifact of source layout — the same trap `PHASE_LADDER` is written out to avoid.
LATTICE: tuple[Lifecycle, ...] = (
    Lifecycle.FULL,
    Lifecycle.DEGRADED,
    Lifecycle.HELD,
    Lifecycle.REFUSED,
)


def rank(level: Lifecycle) -> int:
    """Position in the lattice. FULL is 0; larger is more degraded."""
    return LATTICE.index(level)


class DegradationError(RuntimeError):
    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


@dataclass(frozen=True)
class Degradation:
    """A decision to operate below FULL, with its cost and its exit named."""

    subject: str
    level: Lifecycle
    why: str
    tradeoff: str
    lift_condition: str

    def __post_init__(self) -> None:
        if not self.subject.strip():
            raise ValueError("a degradation must name what is degraded")
        if not self.why.strip():
            raise ValueError(
                f"{self.subject}: a degradation with no stated deficit cannot be audited or taught from"
            )
        if not self.tradeoff.strip():
            raise ValueError(
                f"{self.subject}: a degradation with no stated TRADE-OFF cannot be consented to — "
                "the operator would be agreeing to an unnamed loss"
            )
        if not self.lift_condition.strip():
            raise ValueError(
                f"{self.subject}: a degradation with no LIFT CONDITION is a permanent downgrade "
                "wearing a temporary name. Every degradation must say what would restore it "
                "(INV-3: BLOCKED always escapes)"
            )
        if self.level is Lifecycle.FULL:
            raise ValueError(
                f"{self.subject}: FULL is not a degradation. Use lift() to return a subject to FULL "
                "— recording 'full' as a degradation would make the ledger unreadable"
            )

    def stipulation_id(self) -> str:
        """One id per DECISION, not per subject.

        Keying on the subject alone conflated two different things and the tests caught it: consent
        is given once per stipulation, but a subject may legitimately be degraded again later for a
        DIFFERENT deficit. With a subject-keyed id the second degradation was refused as a duplicate
        consent — so a genuine new deficit could not be recorded, and the estate would keep
        reporting the stale one.

        Including the body digest makes each distinct decision its own stipulation, while a literal
        re-proposal of the identical body is still correctly refused as a duplicate.
        """
        stem = f"degradation.{self.subject}".lower().replace("_", "-")[:48]
        return f"{stem}.{self.digest_short()}"

    def digest_short(self) -> str:
        return hashlib.sha256(self.body()).hexdigest()[:8]

    def body(self) -> bytes:
        """The exact bytes consented to. Canonical JSON so the digest is stable across readers."""
        return json.dumps(
            {
                "subject": self.subject,
                "level": str(self.level),
                "why": self.why,
                "tradeoff": self.tradeoff,
                "lift_condition": self.lift_condition,
            },
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"OPERATE {self.level.upper()}: {self.subject} — {self.tradeoff}",
            self.body(),
        )


def declare(
    root: Path,
    degradation: Degradation,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Put a degradation to the sovereign. It is NOT in effect until ratified."""
    return propose(
        root,
        degradation.stipulation(),
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def accept(
    root: Path,
    degradation: Degradation,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The sovereign consents to operating degraded. This is what puts it in effect."""
    return _accept_body(
        root,
        degradation.stipulation(),
        degradation.body(),
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def _accept_body(
    root: Path,
    stip: Stipulation,
    body: bytes,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None,
) -> Path:
    """Ratify, then persist the ARTIFACT the ratification is over.

    The ratification act stores the signed payload (`id\nsubject\ndigest`) and pins its bytes. The
    payload names the artifact by digest but does not contain it, so without this the ledger could
    prove consent was given and not what it was given to. The digest is re-checked here rather than
    trusted: writing a body that does not hash to what the chain pins would create exactly the
    unverifiable claim this module exists to prevent.
    """
    if hashlib.sha256(body).hexdigest() != stip.digest:
        raise DegradationError(
            "refusing to store a body that is not the artifact the stipulation pins"
        )
    try:
        path = ratify(
            root,
            stip,
            key_path=key_path,
            estate_id=estate_id,
            kernel_version=kernel_version,
            observed_at=observed_at,
        )
    except RatificationError as exc:
        raise DegradationError(str(exc), exc.refusal) from exc
    (root / "ratifications" / f"{stip.stipulation_id}.body").write_bytes(body)
    return path


def lift(
    root: Path,
    subject: str,
    *,
    evidence: str,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Return a subject to FULL — itself a ratified act, never an inference.

    A lift is the claim "the deficit is gone", and that claim is exactly as consequential as the
    degradation was: it restores capability the operator agreed to do without. So it is signed too,
    and it carries the EVIDENCE that the lift condition was met. R6.2 may later propose lifts
    automatically; performing one is still an act of consent.
    """
    current = state(root)
    if subject not in current:
        raise DegradationError(
            f"{subject} is not degraded",
            Refusal(
                gate="k0.degradation.lift",
                why=f"{subject} carries no ratified degradation, so there is nothing to lift",
                legal_next="render(root) lists what is actually degraded",
                teaches="k0.degradation: the ledger is the state; nothing is degraded by assumption",
            ),
        )
    if not evidence.strip():
        raise DegradationError(
            f"{subject}: a lift with no evidence is an assertion",
            Refusal(
                gate="k0.degradation.lift",
                why=(
                    f"lifting {subject} claims its deficit is gone; with no evidence that claim "
                    "cannot be checked, and an unverifiable lift silently restores capability"
                ),
                legal_next=f"supply what shows the lift condition was met: {current[subject].lift_condition}",
                teaches="k0.degradation: restoring capability is as consequential as giving it up",
            ),
        )
    d = current[subject]
    body = json.dumps(
        {
            "subject": subject,
            "level": str(Lifecycle.FULL),
            "why": d.why,
            "tradeoff": d.tradeoff,
            "lift_condition": d.lift_condition,
            "lifted": True,
            "evidence": evidence,
        },
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")
    stip = Stipulation.over(
        f"{d.stipulation_id()}.lift"[:64],
        f"LIFT to FULL: {subject} — {evidence}",
        body,
    )
    # A LIFT IS POSTED BEFORE IT IS SIGNED, like any other act. The ratification act refuses
    # consent to a question never posed, and that invariant is right here too: without the proposal
    # row, the ledger would show a restoration of capability with no record of what was claimed in
    # order to obtain it. Both rows land inside this call because the operator supplies the
    # evidence and the signature in the same breath — but the audit trail is still two acts.
    propose(
        root,
        stip,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )
    return _accept_body(
        root,
        stip,
        body,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def state(root: Path) -> dict[str, Degradation]:
    """Current lifecycle per subject, DERIVED from the chain.

    NOTHING DECAYS BACK TO FULL. A subject degraded and never lifted is still degraded, however
    long ago it was accepted — because the deficit does not heal by being ignored. Only a ratified
    lift removes a subject from this map. Auto-lift is R6.2 and is deliberately elsewhere: deciding
    that a deficit has passed is a judgement, and this module records rather than judges.
    """
    out: dict[str, Degradation] = {}
    # CHAIN ORDER IS THE WHOLE POINT. A dict of ratified rows has no order, and a lift ratified
    # after a degradation must supersede it — reading them in an arbitrary order would let a lifted
    # subject reappear as degraded, or worse, hide a re-degradation behind an older lift.
    for receipt in load_chain(root):
        if receipt.act is not BootstrapAct.RATIFIED:
            continue
        # `_id_of` finds the stipulation on PROPOSED rows too (they carry the signature ref in
        # payload_refs), so the act check above is what actually distinguishes a consented
        # degradation from one merely put forward. Reading `operator_ratification` directly would
        # make that check unexercisable — it survived mutation for exactly that reason.
        sid = _id_of(receipt)
        if not sid.startswith("degradation."):
            continue
        # The digest comes from the CHAIN, not from the body's neighbourhood on disk. Reading it
        # from a file beside the body would let whoever edited one edit the other.
        body = _body_for(root, sid, digest=artifact_digest(receipt))
        if body is None:
            continue
        subject = body["subject"]
        if body.get("lifted"):
            out.pop(subject, None)
            continue
        out[subject] = Degradation(
            subject=subject,
            level=Lifecycle(body["level"]),
            why=body["why"],
            tradeoff=body["tradeoff"],
            lift_condition=body["lift_condition"],
        )
    return out


def _body_for(root: Path, stipulation_id: str, *, digest: str | None) -> dict | None:
    """Recover the consented body, VERIFYING it against the digest the chain pins.

    The payload is `id\\nsubject\\ndigest\\n`; the BODY is the artifact that digest is over, stored
    beside it. `_accept_body` checks that hash at WRITE time — and until this function checked it
    at READ time, editing a `.body` afterwards changed what the estate believed had been consented
    to, while the chain still said `ratified` and still pointed at the old digest. The ledger would
    have proved that consent was given, and lied about what it was given to. Found in review.

    WHETHER THE ROW PINS A DIGEST IS THE WHOLE DISCRIMINATOR. An earlier revision tolerated a
    missing body unconditionally, on the reasoning that "an old chain is not a corrupt one" — true,
    but only of a row that claims no artifact. A row that PINS one is asserting that an artifact
    exists; against that assertion, a missing file is a DELETION. Tolerating it let `state()` skip
    the subject, and a degradation deleted from disk read as FULL — the same absence-into-zero this
    function was written to close, one cell over in the matrix. Both found in review.

      digest is None, no body   -> None. Nothing was claimed and nothing is here; a ledger
                                   predating this module has nothing to verify.
      digest is None, body      -> REFUSES. "The row names no artifact" is not "the artifact
                                   is fine": what was consented to cannot be established.
      digest pinned, no body    -> REFUSES. The row claims an artifact that is gone.
      digest pinned, unreadable -> REFUSES. Claimed, present, and unverifiable is not "absent".
      digest mismatch           -> REFUSES. The tampering case.
      digest matches, not JSON  -> REFUSES. The consented bytes are themselves unusable; the
                                   deficit is real and cannot be rendered.

    EVERY REFUSAL HERE IS A DEFICIT THAT CANNOT BE READ, AND NONE OF THEM MAY RESOLVE TO "no
    deficit". A corrupted degradation must never resolve to FULL, because that is the one wrong
    answer the operator cannot detect by looking.
    """
    path = root / "ratifications" / f"{stipulation_id}.body"
    gate = "degradation.body-integrity"
    if digest is None:
        if not path.is_file():
            return None
        raise DegradationError(
            f"{stipulation_id}: a body is stored but the ratified row pins no artifact digest, so "
            f"what was consented to cannot be established.",
            Refusal(
                gate=gate,
                why="a stored body with no pinned digest cannot be tied to what was consented to",
                legal_next=(
                    "re-ratify the subject so the chain pins the digest of the body it consents "
                    "to, or remove the unpinned body if it was never consented to"
                ),
            ),
        )
    # THE ROW PINS AN ARTIFACT. From here, every failure is corruption rather than antiquity.
    try:
        raw = path.read_bytes()
    except FileNotFoundError:
        reason = "the ratified row pins an artifact digest but no body is stored"
        fix = (
            "restore the consented body from backup, or post a fresh degradation if the deficit "
            "still holds — a ratified deficit cannot be retired by deleting its artifact"
        )
    except OSError as exc:
        # `strerror` only. `str(exc)` and the traceback chain both carry `filename`, which is an
        # absolute estate path — K0 must be readable by a stranger, so the reason is raised OUTSIDE
        # this handler rather than `from exc` (which retains `__context__` even when display is
        # suppressed).
        reason = f"the pinned artifact could not be read ({exc.strerror})"
        fix = "restore read access to the ratifications directory, then re-run"
    else:
        actual = hashlib.sha256(raw).hexdigest()
        if actual != digest:
            raise DegradationError(
                f"{stipulation_id}: the stored body hashes to {actual[:12]}… but the chain pins "
                f"{digest[:12]}… — the artifact changed after consent. The ledger can prove consent "
                f"was given and not what it was given to, so this refuses rather than reporting a "
                f"degradation the operator never accepted.",
                Refusal(
                    gate=gate,
                    why="the stored body is not the artifact the operator ratified",
                    legal_next=(
                        "restore the body whose sha256 is the pinned digest, or ratify a new "
                        "degradation carrying the changed terms — the edit itself is never consent"
                    ),
                ),
            )
        try:
            return json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, ValueError):
            reason = "the consented artifact is not decodable JSON"
            fix = (
                "restore the body from backup; the chain consents to these exact bytes, so they "
                "cannot be rewritten into valid JSON without a fresh ratification"
            )
    raise DegradationError(
        f"{stipulation_id}: {reason}. Refusing to report the estate as healthier than its ledger "
        f"can establish.",
        Refusal(gate=gate, why=reason, legal_next=fix),
    )


def render(root: Path) -> tuple[str, ...]:
    """Honest-dark projection: every degraded subject, its deficit, and its exit.

    The Reins representation kernel forbids turning absence into zero and requires that a datum
    rendered without full context show `partial`/`DARK` WITH the missing reasons and with actions
    held. A degraded subject rendered as a bare number would be exactly the lie that law names.
    """
    lines: list[str] = []
    for subject, d in sorted(state(root).items()):
        lines.append(
            f"{d.level.upper()}  {subject}\n"
            f"    deficit : {d.why}\n"
            f"    cost    : {d.tradeoff}\n"
            f"    lifts by: {d.lift_condition}"
        )
    return tuple(lines)
