"""R2.2 — the deterministic pre-model segment, as DATA. RATIFIED by the operator-of-record.

Ratification provenance: delegated by the operator on 2026-08-07 ("give me a firm rec on all
rulings, then take them and proceed" — the R2.2 ratification was one of the rulings taken) and
recorded in the task's session log (cc-task-first-init-r22-deterministic-pre-model-segment-20260806)
the same day. The operator may amend or rescind with one word; the drift pin makes any such change
diff-visible by construction.

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

THE BOUNDARY (spec §3): the segment is HALF-OPEN — its terminal act, the Crow seat cold-start
(R2.15), is the FIRST POST-SEGMENT act: the first receipted act whose execution involves a model
(local_only, stipulated-admission, UNMEASURED-marked). MEASURED_PROBE is the later wall — the
first TRANSMITTING call — keyed on transmit_class (local_only >= K0_ACTIVE; transmitting >=
AUTH_MATERIALIZE). Note: the receipt spine's comments call MEASURED_PROBE "the first model call";
this regime's two-layer boundary (first model-INVOLVING act vs first TRANSMITTING act) is the
more precise reading and is what the transmit_class keying says.

HONESTY LAWS (review-hardened, 2026-08-06):

    * DECLARED, NOT ENFORCED. This module performs no I/O and prevents no model call at runtime;
      it is the regime's declaration plus machine-checkable invariants. Enforcement claims belong
      to the chain-order receipt law and the ceremony driver, not to this file.
    * Substrate accounting counts only what is ON REINS MAIN. On-branch work is named as
      on_branch evidence and never licenses "partial".
    * The mandatory-act tally is pending the R2.3/R2.4 count act (spec §8.3; floor: 11 instances)
      and is therefore null, not a guessed number frozen into a pin.
    * As of the declaration date: 3 members unbuilt, 2 partial, 3 built.
"""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass
from typing import NoReturn

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

#: Acts that run before K0 arms (ratified K0 spec). install-verify executes one of these.
PRE_K0_ACTS: tuple[str, ...] = ("install",)

#: Legal phase strings: the pre-K0 pseudo-phase plus the pinned PHASE_LADDER values
#: (api/bootstrap_receipt.py:62-74). Restated as data so the vocabulary is pinnable here; a
#: test asserts agreement with the receipt spine.
PHASES: tuple[str, ...] = (
    "pre-K0",
    "K0_ACTIVE",
    "HOST_RECONCILE",
    "STIPULATION_RATIFY",
    "SURFACE_OBSERVE",
    "AUTH_MATERIALIZE",
    "MEASURED_PROBE",
    "CAPABILITY_MINT",
    "SDLC_GATE_SHADOW",
    "ENFORCE_FLIP",
    "KERNEL_DEMOTE",
    "COMPLETE",
)

#: Substrate states an element may declare. "unbuilt" and "partial" are first-class: the regime
#: declares membership, not completion.
SUBSTRATE_STATES: tuple[str, ...] = ("built", "partial", "unbuilt")

#: Evidence classes, typed (review: free-text evidence let "pinky swear" pass). The legality
#: matrix below is the honesty law: only landed/receipted substrate licenses a build claim.
EVIDENCE_CLASSES: tuple[str, ...] = ("absent", "on_branch", "landed", "receipted")

#: state -> legal evidence classes. "partial" and "built" both require real substrate on main;
#: on_branch evidence is honest only for "unbuilt".
EVIDENCE_LEGALITY: dict[str, tuple[str, ...]] = {
    "built": ("landed", "receipted"),
    "partial": ("landed", "receipted"),
    "unbuilt": ("absent", "on_branch"),
}

#: The segment's terminal act (spec §3): the FIRST POST-SEGMENT act. The segment is everything
#: before it; the interval is half-open.
TERMINAL_ACT_ID = "R2.15-crow-cold-start"
TERMINAL_R_NODE = "R2.15"

#: The well-ordering keying (spec §3, KIDS amendment). ORDER IS SEMANTIC — do not sort.
#: This module cites the keying but does not pin it — open ratification question 8.1 defers
#: that to the R0.6/R3.2 amendments.
TRANSMIT_CLASS_LAW: tuple[str, ...] = (
    "local_only>=K0_ACTIVE",
    "transmitting>=AUTH_MATERIALIZE",
)

#: Members whose execution executes no governed act (self-attestation at kernel arm). The empty
#: acts tuple is legal ONLY for these ids — an unremarked hole otherwise.
SELF_ATTESTING_MEMBERS: frozenset[str] = frozenset({"k0-arm-genesis-lock"})


@dataclass(frozen=True)
class SegmentElement:
    """One member of the deterministic pre-model segment."""

    id: str
    #: Bootstrap phase the element executes in; must be a member of PHASES.
    phase: str
    #: Governed acts the element executes. Empty only for SELF_ATTESTING_MEMBERS.
    acts: tuple[str, ...]
    #: Requirements-graph nodes covering the element.
    r_nodes: tuple[str, ...]
    #: "built" | "partial" | "unbuilt" as of the declaration date.
    substrate_state: str
    #: One of EVIDENCE_CLASSES; legality against substrate_state is machine-checked.
    evidence_class: str
    #: What substantiates the claim — artifact/receipt names, never a bare assertion.
    evidence: str


#: The closed-world membership. Order here is presentation only; canonical() sorts.
SEGMENT_MEMBERS: tuple[SegmentElement, ...] = (
    SegmentElement(
        id="install-verify",
        phase="pre-K0",
        acts=("install",),
        r_nodes=("R0.1",),
        substrate_state="unbuilt",
        evidence_class="absent",
        evidence="absent: signed artifact + operator-side verification before first run",
    ),
    SegmentElement(
        id="durable-root-declaration",
        phase="HOST_RECONCILE",
        acts=("reconcile",),
        r_nodes=("R0.7",),
        substrate_state="built",
        evidence_class="landed",
        evidence="api/bootstrap_receipt.py declare_durable_root on main; initially declared by the pre-K0 install (element 1), re-attested here by reconcile; unevaluable=>DENY hardening 2026-08-01",
    ),
    SegmentElement(
        id="k0-arm-genesis-lock",
        phase="K0_ACTIVE",
        acts=(),  # SELF_ATTESTING_MEMBERS
        r_nodes=("R0.3", "R0.5", "R0.6"),
        substrate_state="built",
        evidence_class="landed",
        evidence="api/k0/ package + genesis_self_attest + BootstrapLock + pinned PHASE_LADDER/phase_legal on main; the R0.6 transmit_class keying is deferred (open ratification question 8.1) and is NOT claimed here",
    ),
    SegmentElement(
        id="host-reconcile",
        phase="HOST_RECONCILE",
        acts=("probe", "reconcile"),
        r_nodes=("R1.2", "R1.3", "R1.4"),
        substrate_state="partial",
        evidence_class="landed",
        evidence="api/k0/host_floor.py on main (floor as data, probe()/require()); host registry unbuilt",
    ),
    SegmentElement(
        id="identity",
        phase="K0_ACTIVE",
        acts=("mint",),
        r_nodes=("R0.11",),
        substrate_state="built",
        evidence_class="landed",
        evidence="api/k0/identity.py (exclusive-create estate_id), ratifier.py (SSHSIG), recovery.py (rotation/loss) on main",
    ),
    SegmentElement(
        id="key-capture",
        phase="AUTH_MATERIALIZE",
        acts=("elicit", "mint"),
        r_nodes=("R2.3",),
        substrate_state="unbuilt",
        evidence_class="absent",
        evidence="absent: portable store, guided capture, working-key validation receipt, decline path",
    ),
    SegmentElement(
        id="first-consent",
        phase="AUTH_MATERIALIZE",
        acts=("elicit", "ratify"),
        r_nodes=("R2.4",),
        substrate_state="partial",
        evidence_class="landed",
        evidence="AIR default-deny renderer landed and estate-free; ceremony step + consent receipt unbuilt",
    ),
    SegmentElement(
        id="first-stipulations",
        phase="STIPULATION_RATIFY",
        acts=("elicit", "ratify"),
        r_nodes=("R2.5", "R2.6", "R2.7", "R2.11", "R2.12", "R2.13"),
        substrate_state="unbuilt",
        evidence_class="on_branch",
        evidence="on_branch: R2.8 ratification act + R2.6 degradation ledger on reins PR #7 (2026-08-06); not substrate until merged",
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
    terminal_r_node: str
    #: ORDER IS SEMANTIC (escalating requirements); canonical() preserves it.
    transmit_class_law: tuple[str, ...]
    #: Pending ratification (spec §8.3): null today. When ratified, verify() refuses a value
    #: below the distinct acts the membership itself executes.
    mandatory_act_count: int | None
    #: False until the operator ratifies the spec. Ratification flips this bit, which changes
    #: the pin — ratification is itself a deliberate, diff-visible act.
    ratified: bool

    def canonical(self) -> str:
        """Stable serialization. Members/exclusions sort (order-free); the transmit-class law
        keeps its semantic order."""
        return json.dumps(
            {
                "version": self.version,
                "terminal_act_id": self.terminal_act_id,
                "transmit_class_law": list(self.transmit_class_law),
                "mandatory_act_count": self.mandatory_act_count,
                "ratified": self.ratified,
                "terminal_r_node": self.terminal_r_node,
                "required_member_ids": sorted(REQUIRED_MEMBER_IDS),
                "self_attesting_members": sorted(SELF_ATTESTING_MEMBERS),
                "members": sorted(
                    (
                        {
                            "id": m.id,
                            "phase": m.phase,
                            "acts": sorted(m.acts),
                            "r_nodes": sorted(m.r_nodes),
                            "substrate_state": m.substrate_state,
                            "evidence_class": m.evidence_class,
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
            ensure_ascii=True,
        )

    def digest(self) -> str:
        return hashlib.sha256(self.canonical().encode("utf-8")).hexdigest()


class SegmentViolation(ValueError):
    """Raised on every invariant failure. There is no 'mostly valid' segment."""


def _fail(reason: str) -> NoReturn:
    raise SegmentViolation(reason)


#: The closed-world membership, by id. Equality, not subset: a surplus member is drift.
REQUIRED_MEMBER_IDS: frozenset[str] = frozenset(
    {
        "install-verify",
        "durable-root-declaration",
        "k0-arm-genesis-lock",
        "host-reconcile",
        "identity",
        "key-capture",
        "first-consent",
        "first-stipulations",
    }
)


def verify(segment: DeterministicSegment, *, expect_pin: str | None = None) -> str:
    """Machine-check the segment's invariants; return its drift pin. Raises on ANY violation."""
    if not segment.members:
        _fail("the segment has no members")
    seen: set[str] = set()
    for member in segment.members:
        if member.id in seen:
            _fail(f"duplicate member id: {member.id}")
        seen.add(member.id)
        if member.phase not in PHASES:
            _fail(
                f"{member.id}: phase {member.phase!r} is not in the pinned ladder vocabulary"
            )
        if member.substrate_state not in SUBSTRATE_STATES:
            _fail(f"{member.id}: unknown substrate state {member.substrate_state!r}")
        if member.evidence_class not in EVIDENCE_CLASSES:
            _fail(f"{member.id}: unknown evidence class {member.evidence_class!r}")
        if member.evidence_class not in EVIDENCE_LEGALITY[member.substrate_state]:
            _fail(
                f"{member.id}: {member.evidence_class} evidence cannot license "
                f"{member.substrate_state} — an upgraded state needs landed substrate"
            )
        if not member.evidence:
            _fail(f"{member.id}: a state claim with no evidence text is a false green")
        if not member.acts and member.id not in SELF_ATTESTING_MEMBERS:
            _fail(
                f"{member.id}: executes no governed acts and is not a self-attesting member"
            )
        for act in member.acts:
            if act not in BOOTSTRAP_ACTS and act not in PRE_K0_ACTS:
                _fail(
                    f"{member.id}: {act!r} is not a governed act (nor the pre-K0 install act)"
                )
            if member.phase == "pre-K0" and act not in PRE_K0_ACTS:
                _fail(f"{member.id}: governed act {act!r} cannot execute pre-K0")
            if member.phase != "pre-K0" and act in PRE_K0_ACTS:
                _fail(
                    f"{member.id}: pre-K0 act {act!r} cannot execute at {member.phase}"
                )
        if not member.r_nodes:
            _fail(f"{member.id}: no requirements-graph coverage")
    if seen != REQUIRED_MEMBER_IDS:
        _fail(
            f"segment membership is not exactly the required closed world: "
            f"missing {sorted(REQUIRED_MEMBER_IDS - seen)}, surplus {sorted(seen - REQUIRED_MEMBER_IDS)}"
        )
    if not segment.terminal_act_id:
        _fail("no terminal act: the segment's boundary is undefined")
    if not re.match(r"^R\d+\.\d+$", segment.terminal_r_node):
        _fail(
            f"terminal_r_node {segment.terminal_r_node!r} is not an R-node id — "
            "the half-open check would go vacuous"
        )
    member_r_nodes = {node for m in segment.members for node in m.r_nodes}
    if segment.terminal_r_node in member_r_nodes:
        _fail(
            f"terminal act {segment.terminal_act_id} is inside the segment it terminates — "
            "the interval is half-open"
        )
    if not segment.transmit_class_law:
        _fail("no transmit-class law: the well-ordering keying is undefined")
    if segment.mandatory_act_count is not None:
        act_instances = sum(len(m.acts) for m in segment.members)
        if segment.mandatory_act_count < act_instances:
            _fail(
                f"mandatory_act_count {segment.mandatory_act_count} is below the "
                f"{act_instances} act instances the membership itself executes — the tally "
                "contradicts the data"
            )
    if not segment.exclusions:
        _fail("the exclusion ledger is empty — minimality is unasserted")
    verify_minimality(segment)
    overlap = {m.id for m in segment.members} & {e.id for e in segment.exclusions}
    if overlap:
        _fail(f"declared both member and exclusion: {sorted(overlap)}")
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


assert set(EVIDENCE_LEGALITY) == set(SUBSTRATE_STATES)

#: The canonical segment, verified at import. A drift pin that recomputes itself is not a pin,
#: so R22_RATIFIED_PIN is a literal; the suite recomputes and compares independently of import.
DETERMINISTIC_SEGMENT = DeterministicSegment(
    version="r2.2-ratified-2026-08-07",
    members=SEGMENT_MEMBERS,
    exclusions=EXCLUSIONS,
    terminal_act_id=TERMINAL_ACT_ID,
    terminal_r_node=TERMINAL_R_NODE,
    transmit_class_law=TRANSMIT_CLASS_LAW,
    mandatory_act_count=None,  # pending the R2.3/R2.4 act (floor: 11 instances)
    ratified=True,  # operator-of-record, 2026-08-07, recorded delegation
)

R22_RATIFIED_PIN = "157db7031f0e407f20e635db0aedd48218c5c9236c9a95bb81fb81cc7dba77ae"

SEGMENT_DRIFT_PIN = verify(DETERMINISTIC_SEGMENT, expect_pin=R22_RATIFIED_PIN)
verify_minimality(DETERMINISTIC_SEGMENT)
