"""R2.12 — the support boundary, ratified as the ceremony's closing act.

The estate pattern exists (SUPPORT.md: redirect surface, blank issues disabled, no-perk funding,
in/out-of-scope enumerated; no auth gates to the operator). What the graph names as missing:
"support stipulation ratified as the ceremony's closing act; dead-ends terminate in
refusal-shaped cards (out-of-scope + published-answer pointer + self-diagnosis receipt), never
a contact channel".

Two properties as machinery:

  * THE BOUNDARY IS A RATIFIED STIPULATION, and it is the ceremony's CLOSING act — the spine
    performs it last, after every narrowing, so the terms under which support exists are
    consented with full knowledge of what was just consented to.
  * DEAD-ENDS ARE REFUSAL-SHAPED CARDS, never a contact channel. `dead_end(root, topic)`
    answers with the out-of-scope determination, a pointer to the published answer, and a
    self-diagnosis receipt the user can run. There is no contact field anywhere in the shape —
    a contact channel is unrepresentable, which is the only way "never" holds.
"""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass
from datetime import datetime
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

_TOPIC_GRAMMAR = re.compile(r"[a-z][a-z0-9-]{1,63}")

#: The answer surface is a DOCUMENT PATH — never a URL, never a scheme. `mailto:`, `https://`,
#: `irc:` and friends would make a contact channel representable in the pointer field, and the
#: entire clause is that the channel does not exist (claude r1 major).
_ANSWER_SURFACE_GRAMMAR = re.compile(r"[A-Za-z0-9][A-Za-z0-9./_-]{2,127}")


class SupportBoundaryError(RuntimeError):
    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


@dataclass(frozen=True)
class SupportBoundary:
    """The consented support terms. What is not enumerated as in-scope is out — the closed
    world is the point."""

    in_scope: tuple[str, ...]
    out_scope: tuple[str, ...]
    #: Where published answers live (a doc surface ref, never a person).
    answer_surface: str

    def __post_init__(self) -> None:
        object.__setattr__(self, "in_scope", tuple(self.in_scope))
        object.__setattr__(self, "out_scope", tuple(self.out_scope))
        if not self.in_scope:
            raise ValueError("a boundary with nothing in scope supports nothing — decline instead")
        for topic in (*self.in_scope, *self.out_scope):
            if not _TOPIC_GRAMMAR.fullmatch(topic):
                raise ValueError(f"{topic!r}: topics are lowercase kebab — they become refs")
        overlap = set(self.in_scope) & set(self.out_scope)
        if overlap:
            raise ValueError(f"declared both in and out of scope: {sorted(overlap)}")
        if not _ANSWER_SURFACE_GRAMMAR.fullmatch(self.answer_surface):
            raise ValueError(
                f"{self.answer_surface!r}: the answer surface is a document path "
                "(docs/SUPPORT.md-shaped) — a scheme or channel would make contact "
                "representable, and the clause is that it is not"
            )

    def body(self) -> bytes:
        return json.dumps(
            {
                "in_scope": sorted(self.in_scope),
                "out_scope": sorted(self.out_scope),
                "answer_surface": self.answer_surface,
            },
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def stipulation_id(self) -> str:
        return f"support-boundary.{hashlib.sha256(self.body()).hexdigest()[:16]}"

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"SUPPORT BOUNDARY: {len(self.in_scope)} topics in scope, everything else out",
            self.body(),
        )


def present(
    root: Path,
    boundary: SupportBoundary,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    return propose(
        root, boundary.stipulation(), estate_id=estate_id, kernel_version=kernel_version,
        observed_at=observed_at,
    )


def accept(
    root: Path,
    boundary: SupportBoundary,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The closing act: the body durable before the row that pins it, as ever."""
    stip = boundary.stipulation()
    _write_body_durably(root / SIGNATURE_DIRNAME / f"{stip.stipulation_id}.body", boundary.body())
    return ratify(
        root, stip, key_path=key_path, estate_id=estate_id, kernel_version=kernel_version,
        observed_at=observed_at,
    )


def ratified_boundary(root: Path, *, allow_unauthenticated: bool = False) -> SupportBoundary | None:
    """The consented boundary, verified at read. None renders dark. The hash-only read must be
    opted into explicitly — same law as every reader in this kernel."""
    if not allow_unauthenticated:
        raise SupportBoundaryError(
            "the hash-only read must be opted into explicitly (allow_unauthenticated=True)",
            Refusal(
                gate="support.signature-verification",
                why="the consent row's signature was not verified and the caller did not accept that",
                legal_next="pass the opt-in knowingly, or read with the signing materials",
            ),
        )
    if not verify_chain_at(root).ok:
        raise SupportBoundaryError(
            "the bootstrap chain fails verification",
            Refusal(
                gate="support.chain-integrity",
                why="an unverifiable chain cannot prove what was consented",
                legal_next="run verify_chain to find the break, restore from backup, then re-run",
            ),
        )
    rows = [
        r
        for r in load_chain(root)
        if r.act is BootstrapAct.RATIFIED and _id_of(r).startswith("support-boundary.")
    ]
    if not rows:
        return None
    sid = _id_of(rows[-1])
    pinned = artifact_digest(rows[-1])
    gate = "support.integrity"
    try:
        raw = (root / SIGNATURE_DIRNAME / f"{sid}.body").read_bytes()
    except OSError as exc:
        raise SupportBoundaryError(
            f"{sid}: the consented boundary body cannot be read ({exc.strerror or 'missing'})",
            Refusal(
                gate=gate,
                why="the artifact is gone or unreadable",
                legal_next="restore the body from backup, or ratify a fresh boundary",
            ),
        ) from None
    actual = hashlib.sha256(raw).hexdigest()
    if pinned is None or actual != pinned or sid.rsplit(".", 1)[1] != actual[:16]:
        raise SupportBoundaryError(
            f"{sid}: the stored body is not the artifact the chain pins",
            Refusal(
                gate=gate,
                why="the artifact changed after consent",
                legal_next="restore the pinned body, or ratify the changed terms",
            ),
        )
    parsed = json.loads(raw.decode("utf-8"))
    if set(parsed) != {"in_scope", "out_scope", "answer_surface"}:
        raise SupportBoundaryError(
            f"{sid}: the consented boundary carries unexpected fields",
            Refusal(gate=gate, why="not the canonical shape", legal_next="restore or re-ratify"),
        )
    try:
        return SupportBoundary(
            in_scope=tuple(parsed["in_scope"]),
            out_scope=tuple(parsed["out_scope"]),
            answer_surface=parsed["answer_surface"],
        )
    except ValueError as exc:
        raise SupportBoundaryError(
            f"{sid}: the consented boundary violates the construction laws: {exc}",
            Refusal(gate=gate, why="parses but is not canonical", legal_next="restore or re-ratify"),
        ) from None


@dataclass(frozen=True)
class RefusalCard:
    """What a dead-end terminates in. NOTE WHAT IS ABSENT: no contact field of any kind —
    a channel is unrepresentable in this shape, which is the only way 'never' holds."""

    topic: str
    determination: str  # "out-of-scope" | "unenumerated"
    pointer: str  # the published-answer surface
    self_diagnosis: str  # the receipt the user can run/keep


def dead_end(root: Path, topic: str) -> RefusalCard:
    """Every dead-end terminates in a refusal-shaped card: the determination, the published
    pointer, and a self-diagnosis receipt. Never a contact channel."""
    boundary = ratified_boundary(root, allow_unauthenticated=True)
    if boundary is None:
        return RefusalCard(
            topic=topic,
            determination="unenumerated",
            pointer="(no support boundary ratified — the estate is dark on support terms)",
            self_diagnosis="run the ceremony and ratify a support boundary first",
        )
    determination = "out-of-scope" if topic in boundary.out_scope else (
        "in-scope" if topic in boundary.in_scope else "unenumerated"
    )
    return RefusalCard(
        topic=topic,
        determination=determination,
        pointer=boundary.answer_surface,
        self_diagnosis=(
            f"verify_chain(root) ok + ratified_boundary(root) names the consented terms; "
            f"topic {topic!r} evaluates {determination} against them"
        ),
    )
