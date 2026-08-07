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
    ratify-phase row is chain-illegal — the ceremony's number is locked, and the read is loud
    rather than pretending the amendment took."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    first = BudgetStipulation(1)
    present(root, first, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, first, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    elicit_capture(root, "alpha-secret", estate_id=ESTATE, kernel_version=KERNEL)
    assert budget_state(root).exhausted

    second = BudgetStipulation(5)
    present(root, second, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, second, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    with pytest.raises(FatigueBudgetError, match="fails verification"):
        effective_budget(root)


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
