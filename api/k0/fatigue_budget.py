"""R2.11 — the fatigue budget and the provisional tier.

Every elicitation costs the sovereign attention, and a ceremony that can ask forever will.
The graph gap: "the budget number (ratifiable default); provisional-tier semantics (defaults
with honest provenance, never silent ratification); revisit affordance; fatigue metrics in run
receipts".

The answers, as machinery:

  * THE BUDGET IS A RATIFIED STIPULATION — the number is consented like every other narrowing
    in this kernel. The ceremony's elicitations are counted from the chain (never a cursor),
    and an exhausted budget refuses further elicitation with the revisit path named.
  * THE PROVISIONAL TIER IS HONEST BY CONSTRUCTION. Before the number is ratified, a module
    default applies — and every answer carries its tier: "provisional" with its provenance, or
    "ratified" with the signature state. A provisional value can never READ as ratified; the
    tier is in the type.
  * THE REVISIT AFFORDANCE IS THE AMENDMENT MACHINERY — and it has an honest window. The
    phase ladder never regresses, so a new budget can be ratified while the ceremony is still
    in its stipulation phase; once the first AUTH_MATERIALIZE elicitation lands, THIS
    ceremony's number is locked, and the refusal says so rather than promising a chain-illegal
    act. The latest ratified number governs within the window, and supersession is visible.
  * THE METRICS ARE DERIVED. `budget_state` recomputes spent/remaining from the chain on every
    call — identical before and after a crash, and impossible to desynchronise. HONESTY NOTE
    (codex r1): the graph asks for these metrics IN RUN RECEIPTS, and the run-receipt container
    is the ceremony runner's layer, not this kernel's — what exists here is the derivation the
    runner reads, not the receipt itself.
  * THE GUARD IS WIRED: `key_capture.elicit_capture` calls `require_budget` before any row is
    written, so an exhausted budget refuses the ask itself.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from pathlib import Path

from bootstrap_receipt import (
    PHASE_LADDER,
    BootstrapAct,
    BootstrapPhase,
    load_chain,
    verify_chain_at,
)

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

#: The provisional default. Chosen to be small enough that an unratified ceremony cannot
#: interrogate its operator indefinitely, large enough to complete the genesis ceremony with
#: room to revisit. It reads PROVISIONAL everywhere it surfaces until the sovereign ratifies
#: a number.
PROVISIONAL_BUDGET = 24

#: Elicitation rows live at these phases; both count against the budget.
_ELICIT_PHASES = frozenset({BootstrapPhase.STIPULATION_RATIFY, BootstrapPhase.AUTH_MATERIALIZE})


class BudgetTier(StrEnum):
    PROVISIONAL = "provisional"  # the module default, unratified — honest provenance
    RATIFIED = "ratified"  # the sovereign's number, signed


class FatigueBudgetError(RuntimeError):
    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


@dataclass(frozen=True)
class BudgetStipulation:
    """The consented number: how many elicitations the ceremony may spend."""

    max_elicitations: int

    def __post_init__(self) -> None:
        if isinstance(self.max_elicitations, bool) or not isinstance(self.max_elicitations, int):
            raise ValueError(
                "a budget must be an integer count of elicitations, not "
                f"{type(self.max_elicitations).__name__} — isinstance(True, int) is True in "
                "Python, and a default-deny law refuses the shape (CodeRabbit)"
            )
        if self.max_elicitations < 1:
            raise ValueError(
                "a budget below 1 consents to no ceremony at all — decline elicitation instead"
            )

    def body(self) -> bytes:
        return json.dumps(
            {"max_elicitations": self.max_elicitations},
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def stipulation_id(self) -> str:
        return f"fatigue-budget.{hashlib.sha256(self.body()).hexdigest()[:16]}"

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"FATIGUE BUDGET: the ceremony may elicit at most {self.max_elicitations} times",
            self.body(),
        )


def _require_stipulation_window(root: Path) -> None:
    """Refuse BEFORE writing a row the ladder would make illegal (claude r1 critical): a
    budget row is STIPULATION_RATIFY-phase, and once the chain has moved past that phase,
    appending one corrupts the ledger instead of amending anything. The locked window is a
    refusal with the legal paths named — never a mutation."""
    chain = load_chain(root)
    if chain and PHASE_LADDER.index(chain[-1].phase) > PHASE_LADDER.index(BootstrapPhase.STIPULATION_RATIFY):
        raise FatigueBudgetError(
            f"the ceremony has moved past the stipulation phase (tail: {chain[-1].phase.value}) "
            "— this ceremony's budget is locked; writing here would corrupt the chain",
            Refusal(
                gate="budget.window-locked",
                why="the phase ladder never regresses; a late amendment is chain-illegal, so it "
                "is refused before it mutates anything",
                legal_next=(
                    "conclude this ceremony with the ratified number, or re-run with the "
                    "amendment ratified before elicitation begins"
                ),
            ),
        )


def present(
    root: Path,
    budget: BudgetStipulation,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Put the budget number to the sovereign."""
    _require_stipulation_window(root)
    return propose(
        root,
        budget.stipulation(),
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def accept(
    root: Path,
    budget: BudgetStipulation,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The sovereign consents to the number. The body is durable BEFORE the row that pins it."""
    _require_stipulation_window(root)
    stip = budget.stipulation()
    _write_body_durably(root / SIGNATURE_DIRNAME / f"{stip.stipulation_id}.body", budget.body())
    return ratify(
        root,
        stip,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


@dataclass(frozen=True)
class EffectiveBudget:
    """The number in force, WITH its tier — a provisional value can never read as ratified."""

    max_elicitations: int
    tier: BudgetTier
    provenance: str


def effective_budget(root: Path) -> EffectiveBudget:
    """The budget in force: the ratified number if one is ratified, else the provisional
    default with its provenance named. The chain is verified first; a suspect ledger refuses."""
    if not verify_chain_at(root).ok:
        raise FatigueBudgetError(
            "the bootstrap chain fails verification",
            Refusal(
                gate="budget.chain-integrity",
                why="an unverifiable chain cannot ground a budget answer",
                legal_next="run verify_chain to find the break, restore from backup, then re-run",
            ),
        )
    rows = [
        r
        for r in load_chain(root)
        if r.act is BootstrapAct.RATIFIED and _id_of(r).startswith("fatigue-budget.")
    ]
    if not rows:
        return EffectiveBudget(
            PROVISIONAL_BUDGET,
            BudgetTier.PROVISIONAL,
            f"module default {PROVISIONAL_BUDGET}, unratified — the sovereign has not consented "
            "to a number",
        )
    receipt = rows[-1]
    sid = _id_of(receipt)
    pinned = artifact_digest(receipt)
    gate = "budget.integrity"
    if pinned is None:
        raise FatigueBudgetError(
            f"{sid}: the ratified budget row pins no artifact digest",
            Refusal(gate=gate, why="a ratification naming nothing", legal_next="verify_chain"),
        )
    try:
        raw = (root / SIGNATURE_DIRNAME / f"{sid}.body").read_bytes()
    except OSError as exc:
        raise FatigueBudgetError(
            f"{sid}: the consented budget body cannot be read ({exc.strerror or 'missing'})",
            Refusal(
                gate=gate,
                why="the artifact is gone or unreadable",
                legal_next="restore the body from backup, or ratify a fresh budget",
            ),
        ) from None
    actual = hashlib.sha256(raw).hexdigest()
    if actual != pinned or sid.rsplit(".", 1)[1] != actual[:16]:
        raise FatigueBudgetError(
            f"{sid}: the stored budget body is not the artifact the chain pins",
            Refusal(
                gate=gate,
                why="the artifact changed after consent",
                legal_next="restore the pinned body, or ratify the changed number",
            ),
        )
    try:
        parsed = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, ValueError):
        raise FatigueBudgetError(
            f"{sid}: the consented budget body is not decodable JSON",
            Refusal(
                gate=gate,
                why="the consented bytes are unusable",
                legal_next="restore the body from backup, or ratify a fresh budget",
            ),
        ) from None
    if not isinstance(parsed, dict) or set(parsed) != {"max_elicitations"}:
        raise FatigueBudgetError(
            f"{sid}: the consented budget carries unexpected fields",
            Refusal(
                gate=gate,
                why="not the canonical shape",
                legal_next="restore the body from backup, or ratify a fresh budget",
            ),
        )
    try:
        # Re-run the construction law on the read-back (codex r2): a parses-but-illegal number
        # (zero, negative, a string) is not a budget, however it got signed.
        validated = BudgetStipulation(parsed["max_elicitations"])
    except ValueError as exc:
        raise FatigueBudgetError(
            f"{sid}: the consented budget violates the construction law: {exc}",
            Refusal(
                gate=gate,
                why="parses but is not a legal budget",
                legal_next="restore the body from backup, or ratify a legal number",
            ),
        ) from None
    return EffectiveBudget(
        validated.max_elicitations,
        BudgetTier.RATIFIED,
        f"ratified as {sid}",
    )


@dataclass(frozen=True)
class BudgetState:
    """Spent/remaining, derived from the chain on every call."""

    spent: int
    remaining: int
    tier: BudgetTier
    exhausted: bool


def budget_state(root: Path) -> BudgetState:
    """The fatigue metrics, DERIVED: elicitation rows counted from the chain, the budget from
    effective_budget — nothing stored, so nothing to desynchronise."""
    budget = effective_budget(root)
    spent = sum(
        1
        for r in load_chain(root)
        if r.act is BootstrapAct.ELICITED and r.phase in _ELICIT_PHASES
    )
    remaining = max(0, budget.max_elicitations - spent)
    return BudgetState(
        spent=spent,
        remaining=remaining,
        tier=budget.tier,
        exhausted=remaining == 0,
    )


def emit_run_receipt(
    root: Path,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """Emit the fatigue metrics as an ACTUAL chain receipt (codex r3): a RECONCILED row at the
    chain's current tail phase (never a regression, so always legal mid-ceremony), carrying the
    derived metrics as refs. The runner calls this at the end of a run; the ledger then holds
    the metrics, not just a derivation that could produce them."""
    state = budget_state(root)
    chain = load_chain(root)
    if not chain:
        raise FatigueBudgetError(
            "no genesis self-attest: there is no run to receipt",
            Refusal(gate="budget.run-receipt", why="no chain", legal_next="open the chain first"),
        )
    if chain[-1].phase is BootstrapPhase.COMPLETE:
        raise FatigueBudgetError(
            "the chain is COMPLETE — a run receipt after it would be a row after terminal",
            Refusal(
                gate="budget.run-receipt",
                why="the ceremony is over; the metrics for it live in the rows already written",
                legal_next="read budget_state for the derived answer",
            ),
        )
    from .key_capture import _append_row_phased

    return _append_row_phased(
        root,
        act=BootstrapAct.RECONCILED,
        phase=chain[-1].phase,
        estate_id=estate_id,
        kernel_version=kernel_version,
        payload_refs=[
            f"fatigue-metrics:spent={state.spent},remaining={state.remaining},"
            f"tier={state.tier.value},exhausted={str(state.exhausted).lower()}"
        ],
        receipt_id=f"fatigue-run-receipt-{len(chain)}",
        observed_at=observed_at,
    )


def run_receipt_metrics(root: Path) -> dict[str, object]:
    """The fatigue metrics as the run-receipt block (R2.11's 'metrics in run receipts'; codex
    r2): a shaped, JSON-safe mapping the ceremony runner embeds in its run receipts. Derived
    from the chain at call time — the receipt can never disagree with the ledger."""
    state = budget_state(root)
    return {
        "fatigue": {
            "spent": state.spent,
            "remaining": state.remaining,
            "exhausted": state.exhausted,
            "tier": state.tier.value,
        }
    }


def require_budget(root: Path) -> None:
    """The guard the elicitation path calls before asking. An exhausted budget refuses with
    the revisit affordance named — the sovereign can always be asked to ratify a new number,
    which is the amendment machinery doing its normal job."""
    state = budget_state(root)
    if state.exhausted:
        raise FatigueBudgetError(
            f"the {state.tier.value} fatigue budget is exhausted "
            f"({state.spent} spent) — no further elicitation is legal",
            Refusal(
                gate="budget.exhausted",
                why="the ceremony's question allowance is spent; more asking is fatigue, and "
                "fatigue is how a sovereign stops reading",
                legal_next=(
                    "if the ceremony is still in its stipulation phase, ratify a fresh budget "
                    "(the amendment is a new consent); if elicitations have already begun, "
                    "this ceremony's number is locked by the phase ladder — conclude with what "
                    "is consented, or re-run with the amendment ratified beforehand"
                ),
            ),
        )
