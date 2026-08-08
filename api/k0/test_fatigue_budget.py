"""R2.11 — the fatigue budget, tested at the laws."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import append_receipt, genesis_self_attest, verify_chain_at
from k0.fatigue_budget import (
    PROVISIONAL_BUDGET,
    BudgetState,
    BudgetStipulation,
    BudgetTier,
    FatigueBudgetError,
    accept,
    budget_state,
    effective_budget,
    present,
    require_budget,
)
from k0.key_capture import elicit_capture

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"


def _key(tmp_path: Path) -> Path:
    key = tmp_path / "ratifier_ed25519"
    subprocess.run(
        ["ssh-keygen", "-t", "ed25519", "-N", "", "-C", "ratifier@test", "-f", str(key)],
        check=True,
        capture_output=True,
    )
    return key


def _chain(root: Path):
    from bootstrap_receipt import load_chain

    return load_chain(root)


def _root(tmp_path: Path) -> Path:
    root = tmp_path / "root"
    root.mkdir()
    append_receipt(
        root,
        genesis_self_attest(
            estate_id=ESTATE,
            kernel_version=KERNEL,
            kernel_manifest_sha256="a" * 64,
            observed_at=datetime.now(UTC) - timedelta(days=365),
        ),
    )
    return root


def test_the_unratified_budget_is_provisional_with_its_provenance(tmp_path: Path) -> None:
    root = _root(tmp_path)
    budget = effective_budget(root)
    assert budget.tier is BudgetTier.PROVISIONAL
    assert budget.max_elicitations == PROVISIONAL_BUDGET
    assert "unratified" in budget.provenance, (
        "a provisional value must SAY so — never silent ratification"
    )


def test_a_ratified_number_governs_and_reads_ratified(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    budget = BudgetStipulation(3)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    got = effective_budget(root)
    assert got.tier is BudgetTier.RATIFIED and got.max_elicitations == 3
    assert verify_chain_at(root).ok


def test_the_metrics_are_derived_and_exhaustion_refuses_with_the_revisit_path(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    budget = BudgetStipulation(2)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    state = budget_state(root)
    assert state == BudgetState(spent=0, remaining=2, tier=BudgetTier.RATIFIED, exhausted=False)
    require_budget(root)  # legal while budget remains

    elicit_capture(root, "alpha-secret", estate_id=ESTATE, kernel_version=KERNEL)
    elicit_capture(root, "beta-secret", estate_id=ESTATE, kernel_version=KERNEL)

    state = budget_state(root)
    assert state.spent == 2 and state.exhausted
    with pytest.raises(FatigueBudgetError, match="exhausted") as exc:
        require_budget(root)
    assert "ratify a fresh budget" in exc.value.refusal.legal_next, (
        "the revisit affordance is named in the refusal"
    )


def test_the_revisit_affordance_supersedes_visibly(tmp_path: Path) -> None:
    """Amendment inside the stipulation-phase window: the latest governs, never an edit."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    first = BudgetStipulation(1)
    present(root, first, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, first, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    second = BudgetStipulation(5)
    present(root, second, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, second, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    assert effective_budget(root).max_elicitations == 5, "the amended number governs"


def test_the_window_closes_when_elicitation_begins(tmp_path: Path) -> None:
    """The phase ladder never regresses: once an AUTH_MATERIALIZE elicitation lands, a new
    ratify-phase row would be chain-illegal — so the amendment is REFUSED BEFORE IT WRITES
    (claude r1), the chain stays legal, and the locked number still governs."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    first = BudgetStipulation(1)
    present(root, first, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, first, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    elicit_capture(root, "alpha-secret", estate_id=ESTATE, kernel_version=KERNEL)
    assert budget_state(root).exhausted

    with pytest.raises(FatigueBudgetError, match="locked"):
        present(root, BudgetStipulation(5), estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(FatigueBudgetError, match="locked"):
        accept(root, BudgetStipulation(5), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert verify_chain_at(root).ok
    assert effective_budget(root).max_elicitations == 1


def test_a_tampered_budget_body_refuses(tmp_path: Path) -> None:
    from k0.ratification import SIGNATURE_DIRNAME

    root = _root(tmp_path)
    key = _key(tmp_path)
    budget = BudgetStipulation(4)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    (root / SIGNATURE_DIRNAME / f"{budget.stipulation_id()}.body").write_text(
        '{"max_elicitations":999}', encoding="utf-8"
    )
    with pytest.raises(FatigueBudgetError, match="not the artifact"):
        effective_budget(root)


def test_the_budget_shape_laws() -> None:
    with pytest.raises(ValueError, match="below 1"):
        BudgetStipulation(0)
    with pytest.raises(ValueError, match="below 1"):
        BudgetStipulation(-3)


def test_the_guard_is_on_the_elicitation_path(tmp_path: Path) -> None:
    """codex/claude r1: the budget is wired — an exhausted budget refuses the elicitation
    itself, before any row is written."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    budget = BudgetStipulation(1)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    elicit_capture(root, "alpha-secret", estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(FatigueBudgetError, match="exhausted"):
        elicit_capture(root, "beta-secret", estate_id=ESTATE, kernel_version=KERNEL)
    assert verify_chain_at(root).ok, "the refusal wrote nothing"


def test_a_late_amendment_is_refused_before_it_mutates(tmp_path: Path) -> None:
    """claude r1 critical: after elicitation begins, present/accept REFUSE — the chain stays
    legal, and the refusal names the honest paths."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    budget = BudgetStipulation(2)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    elicit_capture(root, "alpha-secret", estate_id=ESTATE, kernel_version=KERNEL)

    with pytest.raises(FatigueBudgetError, match="window-locked|locked"):
        present(root, BudgetStipulation(9), estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(FatigueBudgetError, match="locked"):
        accept(root, BudgetStipulation(9), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert verify_chain_at(root).ok, "the ledger was not mutated by the refused amendment"
    assert effective_budget(root).max_elicitations == 2


def test_the_effective_budget_refusal_branches(tmp_path: Path) -> None:
    """claude r1: pins-nothing, missing body, unexpected fields — each refuses distinctly."""
    import hashlib as _hashlib

    from bootstrap_receipt import (
        BootstrapAct,
        BootstrapPhase,
        BootstrapReceipt,
        EvidenceStatus,
        load_chain,
    )
    from k0.ratification import SIGNATURE_DIRNAME, Stipulation, propose, ratify

    # pins-nothing
    sub = tmp_path / "b1"
    sub.mkdir()
    root = _root(sub)
    chain = load_chain(root)
    append_receipt(
        root,
        BootstrapReceipt(
            receipt_id="budget-pinning-nothing",
            estate_id=ESTATE,
            kernel_version=KERNEL,
            phase=BootstrapPhase.STIPULATION_RATIFY,
            act=BootstrapAct.RATIFIED,
            payload_refs=["ratification-sig:fatigue-budget.0000000000000000"],
            evidence_status=EvidenceStatus.OBSERVED,
            prev_receipt_hash=chain[-1].receipt_hash(),
            observed_at=datetime.now(UTC),
        ),
    )
    with pytest.raises(FatigueBudgetError, match="pins no artifact"):
        effective_budget(root)

    # missing body
    sub = tmp_path / "b2"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    budget = BudgetStipulation(4)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    (root / SIGNATURE_DIRNAME / f"{budget.stipulation_id()}.body").unlink()
    with pytest.raises(FatigueBudgetError, match="cannot be read"):
        effective_budget(root)

    # unexpected fields
    sub = tmp_path / "b3"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    bad = b'{"max_elicitations":4,"surprise":true}'
    sid = f"fatigue-budget.{_hashlib.sha256(bad).hexdigest()[:16]}"
    stip = Stipulation.over(sid, "FATIGUE BUDGET: extra-field test", bad)
    (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
    (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
    propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(FatigueBudgetError, match="unexpected fields"):
        effective_budget(root)


def test_an_illegal_number_on_readback_is_refused(tmp_path: Path) -> None:
    """codex r2: parses-but-illegal is not a budget, even signed."""
    import hashlib as _hashlib

    from k0.ratification import SIGNATURE_DIRNAME, Stipulation, propose, ratify

    root = _root(tmp_path)
    key = _key(tmp_path)
    bad = b'{"max_elicitations":-5}'
    sid = f"fatigue-budget.{_hashlib.sha256(bad).hexdigest()[:16]}"
    stip = Stipulation.over(sid, "FATIGUE BUDGET: illegal value test", bad)
    (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
    (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
    propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(FatigueBudgetError, match="construction law"):
        effective_budget(root)


def test_the_metrics_land_in_an_actual_chain_receipt(tmp_path: Path) -> None:
    """codex r3: the metrics are emitted as a RECONCILED row on the chain itself — the ledger
    holds them, and the row matches the derived state exactly."""
    from k0.fatigue_budget import emit_run_receipt

    root = _root(tmp_path)
    key = _key(tmp_path)
    budget = BudgetStipulation(2)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    elicit_capture(root, "alpha-secret", estate_id=ESTATE, kernel_version=KERNEL)

    emit_run_receipt(root, estate_id=ESTATE, kernel_version=KERNEL)
    last = _chain(root)[-1]
    assert last.act.value == "reconciled"
    assert any(
        ref == "fatigue-metrics:spent=1,remaining=1,tier=ratified,exhausted=false"
        for ref in last.payload_refs
    ), "the receipt carries the derived metrics verbatim"
    assert verify_chain_at(root).ok


def test_the_run_receipt_block_is_shaped_and_derived(tmp_path: Path) -> None:
    """codex r2: the metrics exist AS a run-receipt block — shaped, JSON-safe, derived."""
    import json as _json

    from k0.fatigue_budget import run_receipt_metrics

    root = _root(tmp_path)
    key = _key(tmp_path)
    budget = BudgetStipulation(2)
    present(root, budget, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, budget, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    elicit_capture(root, "alpha-secret", estate_id=ESTATE, kernel_version=KERNEL)

    block = run_receipt_metrics(root)
    serialized = _json.dumps(block)
    assert _json.loads(serialized) == block, "the block must serialize into any receipt format"
    assert block == {
        "fatigue": {"spent": 1, "remaining": 1, "exhausted": False, "tier": "ratified"}
    }
