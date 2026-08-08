"""K0 — R2.15, the crow seat cold-start: the deterministic segment's TERMINAL act.

The deterministic pre-model segment (api/deterministic_segment.py, ratified) is half-open:
its terminal act is the Crow seat cold-start — the FIRST receipted act whose execution
involves a model: local_only, stipulated-admission, UNMEASURED-marked. Everything before
it is no-LLM by law; the first TRANSMITTING call (MEASURED_PROBE) is the later, harder
wall and is nowhere near this module.

This module is the hydrate-from-nothing arm of the seat design (checkpoint -> admit ->
hydrate -> continue -> witness, minus the checkpoint: at genesis there is nothing to
resume from). Three acts, all receipted on the bootstrap chain at SURFACE_OBSERVE — the
first ladder rung past the ceremony:

1. STORE CREATION FIRST ACT — the crow's working store is created empty-but-valid and
   receipted (MINTED/OBSERVED). An empty store is a true state: the seat has nothing
   yet and says so.
2. STIPULATED BOOTSTRAP ADMISSION, UNMEASURED-MARKED — the seat's first capability
   admission is STIPULATED, never measured: the receipt's evidence_status is ASSERTED
   (claimed, not witnessed) and its refs carry the unmeasured mark. R3.8's law applies
   from birth: nothing is supply until measured — including the horse. An asserted
   admission inherits the blocker class; it does not route work.
3. DESCRIPTOR-SHAPED BOOTSTRAP CHANNEL — the cold-start channel is declared as data
   (CrowBootstrapChannel), with a hardwired local_only transport and the named flip
   target: council's CapabilityIO SESSION send boundary (merged as #4440) replaces the
   transport under the later enforce-flip. The channel validates fail-closed.

LAWS (each pinned by test):

- POST-CEREMONY ONLY. The terminal act is post-segment: ceremony_complete() must hold,
  else Refusal with legal_next. A cold-start before the ceremony is a genesis with no
  sovereign consents — refuse it.
- NO DOUBLE GENESIS. A second cold_start against an existing store refuses; the seat
  re-enters by hydrating from its store, never by re-cold-starting.
- NO TRANSMITTING CALL. The module performs no network I/O and no model client import;
  the receipt spine's transmit_class is local_only by construction. The no-LLM source
  scan covers this module as it covers the segment's set.
- NARRATION, NOT MUTATION. The Crow owns narration only; this module mints receipts and
  a store directory and nothing else.
- DETERMINISTIC LEARNER SIGNALS ONLY. The channel's signal vocabulary is a closed tuple
  (forced-choice, summon/dismiss, timing) — free-text grading is unrepresentable here.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

from bootstrap_receipt import (
    BootstrapAct,
    BootstrapPhase,
    BootstrapReceipt,
    EvidenceStatus,
    append_receipt,
    load_chain,
    verify_chain,
)
from .ceremony import ceremony_complete
from .fail_closed import Evaluation, decide
from .refusal import Refusal, RefusalError

#: The segment's own declaration of its terminal act (api/deterministic_segment.py) — a
#: test asserts the two agree, mirroring the BOOTSTRAP_ACTS cross-check.
TERMINAL_ACT_ID = "R2.15-crow-cold-start"

#: The store's home under the durable root. Created by the first act and never elsewhere.
CROW_STORE_DIRNAME = "crow"

#: The deterministic learner-signal vocabulary (the exclusion ledger's R2.15 clause):
#: closed, enumerable, non-linguistic. Free-text grading has no inhabitant here.
LEARNER_SIGNALS: tuple[str, ...] = ("forced_choice", "summon_dismiss", "timing")

#: The hardwired transport is replaced by CapabilityIO under the enforce-flip (#4440's
#: SESSION send boundary is the named target). Declared as data so the flip is a
#: transport swap, never a protocol change.
BOOTSTRAP_TRANSPORT = "hardwired-local-tmux"
CAPABILITYIO_FLIP_TARGET = "capabilityio-session-send"


class CrowColdStartError(RuntimeError):
    """Raised when the cold-start cannot proceed. Carries the typed Refusal."""

    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


@dataclass(frozen=True)
class CrowBootstrapChannel:
    """The descriptor-shaped bootstrap channel, as data. validate() fails closed."""

    channel_id: str
    descriptor_digest: str  # sha256 over the channel descriptor's canonical form
    transport: str
    admission_class: str  # "stipulated" — the only legal class at cold-start
    flip_target: str
    learner_signals: tuple[str, ...]

    def validate(self) -> None:
        if self.admission_class != "stipulated":
            raise CrowColdStartError(
                f"channel {self.channel_id!r} claims admission class {self.admission_class!r}",
                Refusal(
                    gate="k0.crow-cold-start.channel",
                    why="the bootstrap admission is STIPULATED by construction; any other class "
                    "claims measurement the cold-start has not performed",
                    legal_next="declare admission_class='stipulated'; measured admission is the "
                    "probe layer's, after AUTH_MATERIALIZE",
                    teaches="k0.crow-cold-start: nothing is supply until measured",
                ),
            )
        if self.transport != BOOTSTRAP_TRANSPORT:
            raise CrowColdStartError(
                f"channel {self.channel_id!r} names transport {self.transport!r}",
                Refusal(
                    gate="k0.crow-cold-start.channel",
                    why=f"the bootstrap transport is hardwired as {BOOTSTRAP_TRANSPORT!r} until the "
                    "enforce-flip; an ad-hoc transport is a boutique send path",
                    legal_next=f"use {BOOTSTRAP_TRANSPORT!r}, or flip to {CAPABILITYIO_FLIP_TARGET!r} "
                    "under the governed enforce-flip",
                    teaches="k0.crow-cold-start: one transport, named and replaceable",
                ),
            )
        unknown = set(self.learner_signals) - set(LEARNER_SIGNALS)
        if unknown:
            raise CrowColdStartError(
                f"channel {self.channel_id!r} carries unknown learner signals {sorted(unknown)}",
                Refusal(
                    gate="k0.crow-cold-start.channel",
                    why="the learner-signal vocabulary is closed and deterministic; free-text "
                    "grading is model-adjacent judgment and belongs to no bootstrap channel",
                    legal_next=f"restrict signals to {list(LEARNER_SIGNALS)}",
                    teaches="k0.crow-cold-start: deterministic signals only, pre-model by law",
                ),
            )


@dataclass(frozen=True)
class CrowColdStart:
    """What a completed cold-start proves — receipt ids, never prose."""

    store_receipt: str
    admission_receipt: str
    channel_id: str


def crow_store_path(root: Path) -> Path:
    return root / CROW_STORE_DIRNAME


def default_channel() -> CrowBootstrapChannel:
    """The canonical cold-start channel: stipulated admission, hardwired transport,
    the deterministic signal set. The digest is over the channel's canonical fields,
    so any edit is a new channel — declared, never silent."""
    fields = {
        "transport": BOOTSTRAP_TRANSPORT,
        "admission_class": "stipulated",
        "flip_target": CAPABILITYIO_FLIP_TARGET,
        "learner_signals": list(LEARNER_SIGNALS),
    }
    digest = hashlib.sha256(
        json.dumps(fields, sort_keys=True, separators=(",", ":")).encode("utf-8")
    ).hexdigest()
    return CrowBootstrapChannel(
        channel_id="crow-bootstrap-channel:v1",
        descriptor_digest=digest,
        transport=BOOTSTRAP_TRANSPORT,
        admission_class="stipulated",
        flip_target=CAPABILITYIO_FLIP_TARGET,
        learner_signals=LEARNER_SIGNALS,
    )


def _chain(root: Path) -> list[BootstrapReceipt]:
    """Read and verify the chain, or refuse. FAIL-CLOSED: an unreadable chain is not
    an empty one, and a chain that fails verification is a suspect ledger, not a
    foundation."""
    try:
        chain = load_chain(root)
    except (OSError, ValueError) as exc:
        raise CrowColdStartError(
            f"ceremony state is unreadable at {root}: {exc}",
            Refusal(
                gate="k0.crow-cold-start",
                why=f"the bootstrap chain cannot be read: {exc}",
                legal_next="restore the chain's readability (path, permissions, JSONL shape), then cold-start",
                teaches="k0.receipt-primitive: absence of evidence is never evidence of absence",
            ),
        ) from exc
    verdict = verify_chain(chain)
    if not verdict.ok:
        raise CrowColdStartError(
            "the bootstrap chain does not verify",
            Refusal(
                gate="k0.crow-cold-start",
                why=f"chain verification failed: {verdict.errors}; a cold-start on a suspect "
                "chain would mint receipts nobody can audit",
                legal_next="repair the chain (the verdict names the break), then cold-start",
                teaches="k0.receipt-primitive: the chain is the audit, never a detail",
            ),
        )
    return chain


def _append(
    root: Path,
    chain: list[BootstrapReceipt],
    *,
    act: BootstrapAct,
    estate_id: str,
    kernel_version: str,
    payload_refs: list[str],
    evidence_status: EvidenceStatus,
    receipt_id: str,
    observed_at: datetime | None,
) -> Path:
    receipt = BootstrapReceipt(
        receipt_id=receipt_id,
        estate_id=estate_id,
        kernel_version=kernel_version,
        phase=BootstrapPhase.SURFACE_OBSERVE,
        act=act,
        payload_refs=payload_refs,
        evidence_status=evidence_status,
        operator_ratification=None,
        prev_receipt_hash=chain[-1].receipt_hash(),
        observed_at=observed_at or datetime.now(UTC),
    )
    return append_receipt(root, receipt)


def cold_start(
    root: Path,
    *,
    estate_id: str,
    kernel_version: str,
    channel: CrowBootstrapChannel | None = None,
    observed_at: datetime | None = None,
) -> CrowColdStart:
    """Perform the crow seat cold-start: store creation, then the STIPULATED,
    UNMEASURED-marked bootstrap admission bound to the descriptor-shaped channel.

    Every act is receipted at SURFACE_OBSERVE (the first post-ceremony rung). The
    receipts witness; they authorize nothing (may_authorize=False by construction).
    """
    decide(
        "k0.crow-cold-start",
        Evaluation.SATISFIED if ceremony_complete(root) else Evaluation.VIOLATED,
        legal_next="complete the genesis stipulations first "
        "(ceremony.ratify_genesis_stipulations), then cold-start",
        violated_why="the ceremony is incomplete — the segment's terminal act is "
        "post-ceremony; a cold-start before it is a seat with no sovereign consents",
        unevaluable_why="the ceremony's completeness could not be evaluated; an "
        "unevaluable ceremony DENIES the cold-start",
        teaches="k0.crow-cold-start: the terminal act follows the ceremony, never precedes it",
    )

    channel = channel or default_channel()
    channel.validate()

    store = crow_store_path(root)
    if store.exists():
        raise CrowColdStartError(
            f"crow store already exists at {store}",
            Refusal(
                gate="k0.crow-cold-start",
                why="the store exists, so the seat has a genesis already; a second cold-start "
                "would fork the seat's provenance",
                legal_next="hydrate from the existing store (the recomposition protocol's "
                "resume path), or reconcile per R2.9",
                teaches="k0.crow-cold-start: genesis happens once; re-entry is hydrate",
            ),
        )

    chain = _chain(root)
    if not chain:
        raise CrowColdStartError(
            "no genesis self-attest",
            Refusal(
                gate="k0.crow-cold-start",
                why="the chain is empty; there is no estate to cold-start within",
                legal_next="run genesis_self_attest() to open the chain",
                teaches="k0.receipt-primitive: the chain opens with the kernel witnessing itself",
            ),
        )

    # ACT 1 — store creation first act: the empty-but-valid store, receipted OBSERVED
    # (its existence is directly witnessed, not claimed).
    store.mkdir(parents=False, mode=0o700)
    (store / "README.json").write_text(
        json.dumps(
            {
                "schema": "hapax.crow-store.v1",
                "created_by": TERMINAL_ACT_ID,
                "state": "empty-but-valid",
                "contents": [],
            },
            sort_keys=True,
        )
        + "\n",
        encoding="utf-8",
    )
    store_digest = hashlib.sha256((store / "README.json").read_bytes()).hexdigest()
    _append(
        root,
        chain,
        act=BootstrapAct.MINTED,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[f"crow-store:sha256:{store_digest}"],
        evidence_status=EvidenceStatus.OBSERVED,
        receipt_id="crow-store-created",
        observed_at=observed_at,
    )

    # ACT 2 — the STIPULATED bootstrap admission, UNMEASURED-marked: ASSERTED (claimed,
    # not witnessed), with the unmeasured mark in the refs. This admission routes
    # nothing until the probe layer measures it.
    chain = _chain(root)
    _append(
        root,
        chain,
        act=BootstrapAct.MINTED,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[
            f"crow-bootstrap-admission:stipulated",
            f"crow-bootstrap-admission:unmeasured",
            f"crow-channel:{channel.channel_id}",
            f"crow-channel-descriptor:sha256:{channel.descriptor_digest}",
            f"capabilityio-flip-target:{channel.flip_target}",
        ],
        evidence_status=EvidenceStatus.ASSERTED,
        receipt_id="crow-bootstrap-admission",
        observed_at=observed_at,
    )

    return CrowColdStart(
        store_receipt="crow-store-created",
        admission_receipt="crow-bootstrap-admission",
        channel_id=channel.channel_id,
    )
