"""K0 seed member `ratification-act` — PERFORMING a ratification, not recording one. (R2.8)

The kernel's own manifest has said, since the fixed point was ratified:

    seed  ratification-act -> R2.8  NOT BUILT (recordable, never performed)

`BootstrapAct.RATIFIED` and the `operator_ratification` field have existed all along, and
`ratifier.py` can bind bytes to the sovereign via SSHSIG. What did not exist is the act: something
that takes a stipulation, requires the sovereign to sign it, and lands an `act=ratified` row that a
stranger can verify later. Recording an act and performing one are different things, and the gap
between them is exactly the estate's measured defect — a plane that can represent every outcome and
produce none of them.

WHY THIS IS THE SEED MEMBER AND NOT A CORE ONE. Every other K0 member is machinery the kernel runs
on its own. This one cannot execute without the sovereign: a ratification with no human signature is
not a weaker ratification, it is not one at all. The kernel supplies the ceremony; it never supplies
the consent.

## Ceremony progress is REGISTRY STATE, not a wizard

R2.8 names the anti-wizardry requirement explicitly: *no linear wizard FSM*. So there is no step
counter, no `current_step`, no resumable cursor, and nothing to corrupt or desynchronise. What is
pending is DERIVED, every time, by folding the receipt chain:

    pending = {stipulations proposed} - {stipulations ratified}

That derivation is total and idempotent. It survives a crash mid-ceremony, an operator who walks
away for a month, a chain copied to another host, and two processes racing — because it holds no
state of its own to lose. `resume` is not a feature here; it is the absence of a thing that could
fail to resume. This is R2.9 (ceremony resumability) obtained by construction rather than by code.

## The historical-verification trap

`ratifier.verify_ratification` warns that a signature must be checked at the moment it was made,
not at "now": once a key is rotated, `recovery.rotate` closes its validity window, and every
ratification that key ever made begins failing at the present instant. Verifying history at the
present moment is the wrong question, and getting it wrong looks exactly like tampering.

So `verify_ratifications` passes each receipt's own `observed_at` as `verify_time`. A ratification
made under a since-rotated key must still verify forever; that is the entire point of signing it.

## What a receipt may never do

`may_authorize` is `Literal[False]` on every receipt, and this module does not and cannot change
that. A ratification receipt WITNESSES that the sovereign consented; it never itself grants
anything. Any caller reading a `RATIFIED` row as authority has made the never-mint error, and the
chain cannot stop them — but nothing here invites it.
"""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

# `api/` is the import root in this repo (see how the sibling tests import), and `k0` is a
# top-level package inside it — so the receipt primitive is reached absolutely, not as `..`.
# The kernel manifest already records this crossing: `receipt-primitive -> ../bootstrap_receipt.py`.
from bootstrap_receipt import (
    BootstrapAct,
    BootstrapPhase,
    BootstrapReceipt,
    EvidenceStatus,
    append_receipt,
    load_chain,
)
from .ratifier import RatifierError, sign_ratification, verify_ratification
from .refusal import Refusal

#: Where signatures live, relative to the durable root. The receipt carries a POINTER
#: (`operator_ratification`), never the signature itself: a receipt is a reference-carrier.
SIGNATURE_DIRNAME = "ratifications"

#: `operator_ratification` and `payload_refs` grammar. `_REF_GRAMMAR` on the receipt model demands
#: a scheme; these are the two schemes this module mints.
SIGNATURE_REF_SCHEME = "ratification-sig"
STIPULATION_REF_SCHEME = "stipulation"

#: TWO digests exist and conflating them is a real trap. `stipulation:sha256:…` pins the ARTIFACT
#: that was consented to; `ratified-bytes:sha256:…` pins the exact BYTES the sovereign signed,
#: which embed the artifact digest but are not equal to it. Verification needs both: the second to
#: check the signature against something, the first to prove that something is about the right
#: artifact.
RATIFIED_BYTES_REF_SCHEME = "ratified-bytes"

#: A stipulation id is part of a filename and of a ref, so it is constrained rather than trusted.
_STIPULATION_ID = re.compile(r"^[a-z][a-z0-9._-]{2,63}$")

#: Ratification happens at exactly one rung of the ladder. Stated as data so a reader can see the
#: coupling instead of inferring it from a call site.
RATIFY_PHASE = BootstrapPhase.STIPULATION_RATIFY


class RatificationError(RuntimeError):
    """Raised when a ratification cannot be performed. Carries a `Refusal` when the cause is
    governance rather than mechanism — the caller can project it; it is not a bare message."""

    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


@dataclass(frozen=True)
class Stipulation:
    """A thing put to the sovereign for consent.

    `digest` is over the EXACT BYTES ratified, not over `subject`. The subject is how a human
    refers to it; the digest is what was actually consented to. Ratifying a description rather than
    an artifact is how a ceremony comes to attest to something nobody signed.
    """

    stipulation_id: str
    subject: str
    digest: str

    def __post_init__(self) -> None:
        if not _STIPULATION_ID.match(self.stipulation_id):
            raise ValueError(
                f"stipulation id {self.stipulation_id!r} is not usable: it becomes a filename and a "
                "receipt ref, so it must match ^[a-z][a-z0-9._-]{2,63}$"
            )
        if not self.subject.strip():
            raise ValueError(
                f"stipulation {self.stipulation_id!r}: a stipulation with no subject cannot be "
                "consented to — the operator would be signing a blank"
            )
        if not re.fullmatch(r"[0-9a-f]{64}", self.digest):
            raise ValueError(
                f"stipulation {self.stipulation_id!r}: digest must be a sha256 hex digest over the "
                "exact bytes being ratified"
            )

    @classmethod
    def over(cls, stipulation_id: str, subject: str, body: bytes) -> Stipulation:
        """Build a stipulation over the exact bytes the sovereign will sign."""
        return cls(
            stipulation_id=stipulation_id,
            subject=subject,
            digest=hashlib.sha256(body).hexdigest(),
        )

    def payload(self) -> bytes:
        """The exact bytes signed.

        Includes the id and subject, not only the digest: a signature over a bare digest is
        transferable to any other artifact whose digest someone can arrange to match the record.
        Binding the identifier into the signed payload is what stops a valid signature being
        replayed onto a different stipulation.
        """
        return f"{self.stipulation_id}\n{self.subject}\n{self.digest}\n".encode()

    def ref(self) -> str:
        return f"{STIPULATION_REF_SCHEME}:sha256:{self.digest}"

    def bytes_ref(self) -> str:
        """Pin of the exact signed bytes (NOT the artifact digest — see the scheme comment)."""
        return f"{RATIFIED_BYTES_REF_SCHEME}:sha256:{hashlib.sha256(self.payload()).hexdigest()}"

    def signature_ref(self) -> str:
        return f"{SIGNATURE_REF_SCHEME}:{self.stipulation_id}"


def _require_chain(root: Path) -> list[BootstrapReceipt]:
    """Read the chain, or refuse. FAIL-CLOSED: an unreadable chain is not an empty one.

    Treating a chain we could not read as a chain with nothing in it would let a ratification be
    proposed twice, or a ratified stipulation reappear as pending — the estate's own
    absence-into-zero defect, in the ledger that exists to prevent it.
    """
    try:
        return load_chain(root)
    except (OSError, ValueError) as exc:
        raise RatificationError(
            f"ceremony state is unreadable at {root}: {exc}",
            Refusal(
                gate="k0.ratification",
                why=(
                    "the receipt chain could not be read, so what has already been ratified is "
                    "unknown. An unreadable ledger is not an empty ledger."
                ),
                legal_next=(
                    "repair or restore the chain, then re-run; verify_chain_at(root) reports where "
                    "it stops parsing"
                ),
                teaches="k0.receipt-primitive: absence of evidence is not evidence of absence",
            ),
        ) from exc


def _proposed(chain: list[BootstrapReceipt]) -> dict[str, str]:
    """stipulation_id -> stipulation ref, for every proposal in the chain."""
    out: dict[str, str] = {}
    for receipt in chain:
        if receipt.act is not BootstrapAct.HELD or receipt.phase is not RATIFY_PHASE:
            continue
        for ref in receipt.payload_refs:
            if ref.startswith(f"{STIPULATION_REF_SCHEME}:"):
                out[_id_of(receipt)] = ref
    return {k: v for k, v in out.items() if k}


def _ratified(chain: list[BootstrapReceipt]) -> dict[str, BootstrapReceipt]:
    """stipulation_id -> the receipt that ratified it."""
    out: dict[str, BootstrapReceipt] = {}
    for receipt in chain:
        if receipt.act is not BootstrapAct.RATIFIED:
            continue
        sid = _id_of(receipt)
        if sid:
            out[sid] = receipt
    return out


def _id_of(receipt: BootstrapReceipt) -> str:
    """Recover the stipulation id a receipt is about, from its signature ref."""
    ratification = receipt.operator_ratification or ""
    if ratification.startswith(f"{SIGNATURE_REF_SCHEME}:"):
        return ratification.split(":", 1)[1]
    for ref in receipt.payload_refs:
        if ref.startswith(f"{SIGNATURE_REF_SCHEME}:"):
            return ref.split(":", 1)[1]
    return ""


def pending(root: Path) -> tuple[str, ...]:
    """Stipulations proposed and not yet ratified — DERIVED, never stored.

    This is the whole of "ceremony progress". There is no cursor to resume from because there is no
    cursor: the answer is recomputed from the ledger on every call, so it is identical before and
    after a crash, on a copied chain, and under concurrency.
    """
    chain = _require_chain(root)
    ratified = _ratified(chain)
    return tuple(sorted(sid for sid in _proposed(chain) if sid not in ratified))


def propose(
    root: Path,
    stipulation: Stipulation,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Put a stipulation to the sovereign: an `act=held` row at the ratify rung.

    HELD, not ELICITED: the ceremony is waiting on a person, and `held` is the estate's vocabulary
    for exactly that. The row IS the pending state.
    """
    chain = _require_chain(root)
    sid = stipulation.stipulation_id
    if sid in _ratified(chain):
        raise RatificationError(
            f"{sid} is already ratified",
            Refusal(
                gate="k0.ratification.propose",
                why=f"{sid} has already been ratified; re-proposing it would invite a second consent to the same act",
                legal_next="propose a superseding stipulation with its own id, or amend via R6.4",
                teaches="k0.ratification-act: consent is given once, to exact bytes",
            ),
        )
    if sid in _proposed(chain):
        raise RatificationError(
            f"{sid} is already pending",
            Refusal(
                gate="k0.ratification.propose",
                why=f"{sid} is already awaiting the sovereign; a duplicate proposal makes the ledger ambiguous about which one was consented to",
                legal_next=f"ratify {sid}, or leave it pending — pending(root) lists it",
                teaches="k0.ratification-act: ceremony progress is the ledger, not a queue",
            ),
        )
    return _append(
        root,
        chain,
        act=BootstrapAct.HELD,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[stipulation.ref(), stipulation.signature_ref()],
        operator_ratification=None,
        evidence_status=EvidenceStatus.ASSERTED,
        observed_at=observed_at,
        receipt_id=f"stipulation-held-{stipulation.stipulation_id}",
    )


def ratify(
    root: Path,
    stipulation: Stipulation,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """PERFORM the ratification: obtain the sovereign's signature, then witness it.

    Order matters and is not an implementation detail. The signature is taken FIRST and persisted
    BEFORE the receipt is appended, because a receipt claiming a ratification whose signature does
    not exist is precisely a false green — the chain would attest to consent nobody gave. If the
    signing fails, no row is written and the stipulation stays pending, which is true.
    """
    chain = _require_chain(root)
    sid = stipulation.stipulation_id
    if sid in _ratified(chain):
        raise RatificationError(
            f"{sid} is already ratified",
            Refusal(
                gate="k0.ratification.ratify",
                why=f"{sid} already carries a ratification; signing it twice would put two consents for one act in the ledger",
                legal_next="verify the existing ratification with verify_ratifications(), or supersede it",
                teaches="k0.ratification-act: consent is given once, to exact bytes",
            ),
        )
    if sid not in _proposed(chain):
        raise RatificationError(
            f"{sid} was never proposed",
            Refusal(
                gate="k0.ratification.ratify",
                why=(
                    f"{sid} has no pending row, so there is nothing on the record that the sovereign "
                    "was asked to consent to. A ratification that appears without a proposal cannot "
                    "be audited: the ledger would show consent to a question never posed."
                ),
                legal_next=f"propose({sid}) first, then ratify",
                teaches="k0.ratification-act: an act is legible only against the request that prompted it",
            ),
        )

    try:
        signature = sign_ratification(stipulation.payload(), key_path)
    except RatifierError as exc:
        raise RatificationError(
            f"the sovereign did not sign {sid}: {exc}",
            Refusal(
                gate="k0.ratification.ratify",
                why=f"signing failed, so there is no consent to witness: {exc}",
                legal_next="fix the ratifier key (k0.recovery handles rotation and loss), then re-run; the stipulation is still pending",
                teaches="k0.ratification-act: the kernel supplies the ceremony, never the consent",
            ),
        ) from exc

    # Persist the EXACT signed bytes next to the signature. A signature is only checkable against
    # the bytes it was made over, and those bytes are not recoverable from the chain (receipts
    # carry refs, never values). The chain pins their digest, so this file cannot be swapped
    # without verify_ratifications() saying so.
    sig_dir = root / SIGNATURE_DIRNAME
    sig_dir.mkdir(parents=True, exist_ok=True)
    (sig_dir / f"{sid}.payload").write_bytes(stipulation.payload())
    sig_path = sig_dir / f"{sid}.sig"
    sig_path.write_text(signature, encoding="utf-8")

    return _append(
        root,
        chain,
        act=BootstrapAct.RATIFIED,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[stipulation.ref(), stipulation.bytes_ref()],
        operator_ratification=stipulation.signature_ref(),
        evidence_status=EvidenceStatus.OBSERVED,
        observed_at=observed_at,
        receipt_id=f"stipulation-ratified-{sid}",
    )


def _append(
    root: Path,
    chain: list[BootstrapReceipt],
    *,
    act: BootstrapAct,
    estate_id: str,
    kernel_version: str,
    payload_refs: list[str],
    operator_ratification: str | None,
    evidence_status: EvidenceStatus,
    observed_at: datetime | None,
    receipt_id: str,
) -> Path:
    if not chain:
        raise RatificationError(
            "no genesis self-attest",
            Refusal(
                gate="k0.ratification",
                why="the chain is empty, so the kernel has not attested to itself; there is no ceremony to ratify within",
                legal_next="run genesis_self_attest() to open the chain, then propose",
                teaches="k0.receipt-primitive: the chain opens with the kernel witnessing itself",
            ),
        )
    receipt = BootstrapReceipt(
        receipt_id=receipt_id,
        estate_id=estate_id,
        kernel_version=kernel_version,
        phase=RATIFY_PHASE,
        act=act,
        payload_refs=payload_refs,
        evidence_status=evidence_status,
        operator_ratification=operator_ratification,
        prev_receipt_hash=chain[-1].receipt_hash(),
        observed_at=observed_at or datetime.now(UTC),
    )
    return append_receipt(root, receipt)


@dataclass(frozen=True)
class RatificationVerdict:
    """What the ledger's ratifications actually prove, as data."""

    verified: tuple[str, ...]
    unverified: tuple[tuple[str, str], ...]  # (stipulation_id, why)

    @property
    def ok(self) -> bool:
        return not self.unverified


def verify_ratifications(
    root: Path,
    *,
    allowed_signers: Path,
    principal: str,
    scratch_dir: Path,
) -> RatificationVerdict:
    """Re-check every ratification in the chain against the sovereign's key.

    EACH IS VERIFIED AT ITS OWN `observed_at`, never at "now". Once a key is rotated its validity
    window closes, and every ratification it ever made fails at the present instant — so verifying
    history against the present clock reports tampering where there is none. The receipt records
    when consent was given; that is the moment the signature must be checked at.
    """
    chain = _require_chain(root)
    verified: list[str] = []
    unverified: list[tuple[str, str]] = []

    for sid, receipt in sorted(_ratified(chain).items()):
        sig_path = root / SIGNATURE_DIRNAME / f"{sid}.sig"
        # SUBSUMED BUT KEPT, FOR THE MESSAGE ONLY. Deleting this survives the suite: the read
        # below raises OSError and lands in the same `unverified` list. It stays because
        # "the chain claims a ratification but its signature is missing at <path>" tells an
        # operator what to do, and "[Errno 2] No such file or directory" does not. It is a
        # diagnostic, NOT a control — nothing downstream may treat its presence as protection.
        if not sig_path.is_file():
            unverified.append(
                (sid, f"the chain claims a ratification but its signature is missing at {sig_path}")
            )
            continue
        refs = [r for r in receipt.payload_refs if r.startswith(f"{STIPULATION_REF_SCHEME}:")]
        if len(refs) != 1:
            unverified.append((sid, "the ratified row does not reference exactly one stipulation"))
            continue
        digest = refs[0].rsplit(":", 1)[-1]
        # THE SIGNED BYTES ARE STORED, NOT RECONSTRUCTED. A receipt carries refs, never values, so
        # the subject is not in the chain — and the signature was made over a payload that includes
        # it. Reconstructing the payload from the row alone is impossible, and guessing at it makes
        # every verification fail in a way that looks exactly like tampering.
        #
        # So the exact payload is persisted beside the signature, and the chain pins its digest.
        # Verification is then a closed loop with no room to fudge:
        #   1. the stored payload must hash to the digest the RECEIPT names  (tamper check)
        #   2. the signature must verify over those same bytes               (consent check)
        # Failing (1) means the artifact changed under a ratification that still points at it,
        # which is the case a digest exists to catch.
        payload_path = root / SIGNATURE_DIRNAME / f"{sid}.payload"
        if not payload_path.is_file():
            unverified.append(
                (sid, f"the ratified bytes are missing at {payload_path}; the signature cannot be checked against anything")
            )
            continue
        payload = payload_path.read_bytes()
        byte_refs = [r for r in receipt.payload_refs if r.startswith(f"{RATIFIED_BYTES_REF_SCHEME}:")]
        if len(byte_refs) != 1:
            unverified.append((sid, "the ratified row does not pin the exact signed bytes"))
            continue
        pinned_bytes = byte_refs[0].rsplit(":", 1)[-1]
        actual = hashlib.sha256(payload).hexdigest()
        if actual != pinned_bytes:
            unverified.append(
                (
                    sid,
                    f"the stored ratified bytes hash to {actual[:12]}… but the chain pins "
                    f"{pinned_bytes[:12]}… — the signed bytes changed after consent",
                )
            )
            continue
        # NO ARTIFACT-DIGEST RECHECK HERE, DELIBERATELY. An earlier version also re-derived the
        # artifact digest from the payload's last line and compared it to `digest`. Mutation
        # testing showed that guard SURVIVED deletion under the whole suite: the byte pin above is
        # strictly stronger, because the consented bytes contain the artifact digest by
        # construction, so any payload passing the pin necessarily carries the right digest. The
        # only way to desynchronise the two refs is to edit the receipt itself, which breaks the
        # chain's hash link and is verify_chain's job, not this function's.
        #
        # It was removed rather than kept "for defence in depth". A check no test can independently
        # exercise reads as safety and provides none — and it inflates the apparent rigour of
        # exactly the module whose whole purpose is to make consent provable.
        try:
            verify_ratification(
                payload,
                sig_path.read_text(encoding="utf-8"),
                allowed_signers=allowed_signers,
                principal=principal,
                scratch_dir=scratch_dir,
                verify_time=receipt.observed_at,
            )
        except (RatifierError, OSError) as exc:
            unverified.append((sid, str(exc)))
            continue
        verified.append(sid)

    return RatificationVerdict(verified=tuple(verified), unverified=tuple(unverified))
