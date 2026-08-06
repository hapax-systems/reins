"""R2.2 — the deterministic pre-model segment, as DATA. DRAFT (spec is a ratification candidate).

Lives beside the K0 manifest for the same reason the manifest lives inside the kernel: the segment
must be able to attest itself. Before this module, "everything before the first legal model call is
deterministic" was folk knowledge repeated across three design docs and pinned nowhere — the graph
gap is verbatim "unstated anywhere as a regime".

Mirrors the R0.4 form (api/k0/manifest.py): the structure is a frozen dataclass, the canonical
value is a module constant, invariants are machine checks rather than prose, and a sha256 literal
pins the whole against silent drift. Changes arrive by ratification act, never by edit.

THE REGIME (spec: first-init-r22-deterministic-pre-model-segment-spec-2026-08-06.md §1):

    The deterministic pre-model segment is the closed opening segment of the bootstrap in which
    the execution of NO governed act depends on a model. Governed acts are exactly
    {elicit, ratify, mint, probe, reconcile, flip} (ratified K0 spec). Membership is the eight
    elements below, closed-world; anything not listed is out, and every near-miss is named in the
    exclusion ledger with the act that installs it.

THE BOUNDARY (spec §3): the segment's terminal act is the Crow seat cold-start (R2.15) — the first
receipted act whose execution involves a model (local_only, stipulated-admission, UNMEASURED).
The first TRANSMITTING call (MEASURED_PROBE) is the later wall, keyed on transmit_class
(local_only >= K0_ACTIVE; transmitting >= AUTH_MATERIALIZE), and is outside this regime.

HONEST SUBSTRATE ACCOUNTING: membership is declared regardless of build state; each element carries
its substrate state as data ("built" / "partial" / "unbuilt"). A regime that claimed its members
were all built would be a false green: as of the declaration date three elements are unbuilt and
two are partial, and verify() refuses any member whose state field is upgraded without the
supporting receipt class being named.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass

#: The governed acts, ratified 2026-08-01 (option C). Restated from api/k0/manifest.py so this
#: module is readable without import coupling; the test suite asserts the two agree.
BOOTSTRAP_ACTS: tuple[str, ...] = (
    "elicit",
    "ratify",
    "mint",
    "probe",
    "reconcile",
    "flip",
)

#: Substrate states an element may declare. "unbuilt" and "partial" are first-class: the regime
#: declares membership, not completion.
SUBSTRATE_STATES: tuple[str, ...] = ("built", "partial", "unbuilt")

#: The segment's terminal act (spec §3). Everything before it is no-LLM by law.
TERMINAL_ACT_ID = "R2.15-crow-cold-start"

#: The well-ordering keying (spec §3, KIDS amendment): local-only model execution is legal at
#: K0_ACTIVE; transmitting calls require AUTH_MATERIALIZE. This module cites the keying but does
#: not pin it — open ratification question 8.1 defers that to the R0.6/R3.2 amendments.
TRANSMIT_CLASS_LAW: tuple[str, ...] = (
    "local_only>=K0_ACTIVE",
    "transmitting>=AUTH_MATERIALIZE",
)


@dataclass(frozen=True)
class SegmentElement:
    """One member of the deterministic pre-model segment."""

    id: str
    #: Bootstrap phase the element executes in ("pre-K0" for the ungoverned install act).
    phase: str
    #: Governed acts the element executes. install-verify runs pre-K0 and carries ("install",).
    acts: tuple[str, ...]
    #: Requirements-graph nodes covering the element.
    r_nodes: tuple[str, ...]
    #: "built" | "partial" | "unbuilt" as of the declaration date.
    substrate_state: str
    #: What substantiates the state claim — a receipt/artifact class, never a bare assertion.
    #: An upgraded state without a named evidence class is a false green and verify() refuses it.
    evidence: str


#: The closed-world membership. Order here is presentation only; canonical() sorts.
SEGMENT_MEMBERS: tuple[SegmentElement, ...] = (
    SegmentElement(
        id="install-verify",
        phase="pre-K0",
        acts=("install",),
        r_nodes=("R0.1",),
        substrate_state="unbuilt",
        evidence="absent: signed artifact + operator-side verification before first run",
    ),
    SegmentElement(
        id="durable-root-declaration",
        phase="pre-K0+HOST_RECONCILE",
        acts=("reconcile",),
        r_nodes=("R0.7",),
        substrate_state="built",
        evidence="api/bootstrap_receipt.py declare_durable_root; unevaluable=>DENY hardening 2026-08-01",
    ),
    SegmentElement(
        id="k0-arm-genesis-lock",
        phase="K0_ACTIVE",
        acts=(),  # kernel self-attestation; no governed act is executed by this element
        r_nodes=("R0.3", "R0.5", "R0.6"),
        substrate_state="built",
        evidence="api/k0/ package on reins main; genesis_self_attest; BootstrapLock; pinned PHASE_LADDER",
    ),
    SegmentElement(
        id="host-reconcile",
        phase="HOST_RECONCILE",
        acts=("probe", "reconcile"),
        r_nodes=("R1.2", "R1.3", "R1.4"),
        substrate_state="partial",
        evidence="api/k0/host_floor.py (floor as data, probe()/require()); host registry unbuilt",
    ),
    SegmentElement(
        id="identity",
        phase="AUTH_MATERIALIZE-preface",
        acts=("mint",),
        r_nodes=("R0.11",),
        substrate_state="built",
        evidence="api/k0/identity.py (exclusive-create estate_id); ratifier.py (SSHSIG); recovery.py (rotation/loss)",
    ),
    SegmentElement(
        id="key-capture",
        phase="AUTH_MATERIALIZE",
        acts=("elicit", "mint"),
        r_nodes=("R2.3",),
        substrate_state="unbuilt",
        evidence="absent: portable store, guided capture, working-key validation receipt, decline path",
    ),
    SegmentElement(
        id="first-consent",
        phase="AUTH_MATERIALIZE",
        acts=("elicit", "ratify"),
        r_nodes=("R2.4",),
        substrate_state="partial",
        evidence="AIR default-deny renderer built and estate-free; ceremony step + consent receipt unbuilt",
    ),
    SegmentElement(
        id="first-stipulations",
        phase="STIPULATION_RATIFY",
        acts=("elicit", "ratify"),
        r_nodes=("R2.5", "R2.6", "R2.7", "R2.11", "R2.12", "R2.13"),
        substrate_state="partial",
        evidence="R2.8 ratification act + R2.6 degradation ledger on reins PR #7 (on-branch, 2026-08-06)",
    ),
)


@dataclass(frozen=True)
class Exclusion:
    """A near-miss kept OUT of the segment, with the act/phase that installs it.

    Minimality is falsifiable the same way R0.4's is: every exclusion names its installer, and
    verify_minimality() raises on an exclusion with none.
    """

    id: str
    reason: str
    installed_by: str


EXCLUSIONS: tuple[Exclusion, ...] = (
    Exclusion(
        id="crow-narration",
        reason="model-executing; post-boundary",
        installed_by="R2.15 (the segment's terminal act)",
    ),
    Exclusion(
        id="kids-self-tests",
        reason="model-executing, probe-receipted",
        installed_by="P3 machinery",
    ),
    Exclusion(
        id="measured-probe",
        reason="the first TRANSMITTING call; a later, harder wall",
        installed_by="P3, post-AUTH_MATERIALIZE (transmit_class law)",
    ),
    Exclusion(
        id="enforce-flip",
        reason="P5 governed act, post-ceremony",
        installed_by="P5",
    ),
    Exclusion(
        id="kit-signature-verification",
        reason="in coverage (element 1) but ungoverned — a pre-K0 fact, not a governed act",
        installed_by="R0.1",
    ),
    Exclusion(
        id="free-text-learner-grading",
        reason="model-adjacent judgment; only deterministic learner signals pre-model",
        installed_by="R2.15 design (forced-choice, summon/dismiss, timing)",
    ),
    Exclusion(
        id="estate-tuned-k0-hook-implementations",
        reason="estate-bound implementations, not segment law; the segment pins the extracted law only",
        installed_by="R0.3 (landed)",
    ),
)


@dataclass(frozen=True)
class DeterministicSegment:
    version: str
    members: tuple[SegmentElement, ...]
    exclusions: tuple[Exclusion, ...]
    terminal_act_id: str
    transmit_class_law: tuple[str, ...]
    #: The honest mandatory-act tally (spec §8.3): carried with provenance; ratification fixes it.
    mandatory_act_count: int

    def canonical(self) -> str:
        """Stable serialization. Sorting makes the pin insensitive to declaration order."""
        return json.dumps(
            {
                "version": self.version,
                "terminal_act_id": self.terminal_act_id,
                "transmit_class_law": sorted(self.transmit_class_law),
                "mandatory_act_count": self.mandatory_act_count,
                "members": sorted(
                    (
                        {
                            "id": m.id,
                            "phase": m.phase,
                            "acts": sorted(m.acts),
                            "r_nodes": sorted(m.r_nodes),
                            "substrate_state": m.substrate_state,
                            "evidence": m.evidence,
                        }
                        for m in self.members
                    ),
                    key=lambda m: m["id"],
                ),
                "exclusions": sorted(
                    (
                        {
                            "id": e.id,
                            "reason": e.reason,
                            "installed_by": e.installed_by,
                        }
                        for e in self.exclusions
                    ),
                    key=lambda e: e["id"],
                ),
            },
            sort_keys=True,
            separators=(",", ":"),
        )

    def digest(self) -> str:
        return hashlib.sha256(self.canonical().encode("utf-8")).hexdigest()


class SegmentViolation(ValueError):
    """Raised on every invariant failure. There is no 'mostly valid' segment."""


def _fail(reason: str) -> None:
    raise SegmentViolation(reason)


def verify(segment: DeterministicSegment, *, expect_pin: str | None = None) -> str:
    """Machine-check the segment's invariants; return its drift pin. Raises on ANY violation."""
    if not segment.members:
        _fail("the segment has no members")
    seen: set[str] = set()
    for member in segment.members:
        if member.id in seen:
            _fail(f"duplicate member id: {member.id}")
        seen.add(member.id)
        if member.substrate_state not in SUBSTRATE_STATES:
            _fail(f"{member.id}: unknown substrate state {member.substrate_state!r}")
        if not member.evidence:
            _fail(f"{member.id}: a state claim with no evidence class is a false green")
        if member.substrate_state == "built" and member.evidence.startswith("absent:"):
            _fail(f"{member.id}: declared built with absent evidence")
        for act in member.acts:
            if act not in BOOTSTRAP_ACTS and act != "install":
                _fail(
                    f"{member.id}: {act!r} is not a governed act (nor the pre-K0 install act)"
                )
        if not member.r_nodes:
            _fail(f"{member.id}: no requirements-graph coverage")
    required = {
        "install-verify",
        "durable-root-declaration",
        "k0-arm-genesis-lock",
        "host-reconcile",
        "identity",
        "key-capture",
        "first-consent",
        "first-stipulations",
    }
    missing = required - seen
    if missing:
        _fail(f"segment membership is not closed-world: missing {sorted(missing)}")
    if not segment.terminal_act_id:
        _fail("no terminal act: the segment's boundary is undefined")
    if not segment.transmit_class_law:
        _fail("no transmit-class law: the well-ordering keying is undefined")
    if segment.mandatory_act_count < len(required):
        _fail(
            f"mandatory_act_count {segment.mandatory_act_count} is below the membership floor "
            f"{len(required)} — the tally is dishonest"
        )
    digest = segment.digest()
    if expect_pin is not None and digest != expect_pin:
        _fail(f"drift pin mismatch: {digest} != {expect_pin}")
    return digest


def verify_minimality(segment: DeterministicSegment) -> None:
    """Every exclusion must name its installer; an unnamed one is a minimality violation."""
    for exclusion in segment.exclusions:
        if not exclusion.installed_by:
            _fail(f"exclusion {exclusion.id} names no installer — minimality violation")
        if not exclusion.reason:
            _fail(f"exclusion {exclusion.id} carries no reason")


#: The canonical segment, verified at import. A drift pin that recomputes itself is not a pin,
#: so R22_DRAFT_PIN is a literal; CI recomputes and compares.
DETERMINISTIC_SEGMENT = DeterministicSegment(
    version="r2.2-draft-2026-08-06",
    members=SEGMENT_MEMBERS,
    exclusions=EXCLUSIONS,
    terminal_act_id=TERMINAL_ACT_ID,
    transmit_class_law=TRANSMIT_CLASS_LAW,
    mandatory_act_count=13,
)

R22_DRAFT_PIN = "efbcb368351aecdcf2c7d54854c9654f411fbcebcc8fc9518b7221079be131fe"

SEGMENT_DRIFT_PIN = verify(DETERMINISTIC_SEGMENT, expect_pin=R22_DRAFT_PIN)
verify_minimality(DETERMINISTIC_SEGMENT)
