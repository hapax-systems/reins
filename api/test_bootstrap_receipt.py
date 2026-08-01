"""bootstrap-receipt-v1 spine tests: synthetic genesis→COMPLETE chain, phase legality,
fail-closed append/lock/durable-root, and the kill-matrix skeleton (interrupt at every
phase boundary; prefix verifies; resume appends; zero duplicate receipts)."""
from __future__ import annotations

from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from pydantic import ValidationError

from bootstrap_receipt import (
    BootstrapAct,
    BootstrapLock,
    BootstrapLockError,
    BootstrapPhase,
    BootstrapReceipt,
    DurableRootError,
    append_receipt,
    declare_durable_root,
    genesis_self_attest,
    load_chain,
    verify_chain,
)

T0 = datetime(2026, 7, 9, 12, 0, tzinfo=UTC)
#: one receipt per phase, in ladder order — the synthetic full ceremony
CEREMONY = [
    (BootstrapPhase.HOST_RECONCILE, BootstrapAct.RECONCILED),
    (BootstrapPhase.STIPULATION_RATIFY, BootstrapAct.RATIFIED),
    (BootstrapPhase.SURFACE_OBSERVE, BootstrapAct.PROBED),
    (BootstrapPhase.AUTH_MATERIALIZE, BootstrapAct.MINTED),
    (BootstrapPhase.MEASURED_PROBE, BootstrapAct.PROBED),
    (BootstrapPhase.CAPABILITY_MINT, BootstrapAct.MINTED),
    (BootstrapPhase.SDLC_GATE_SHADOW, BootstrapAct.HELD),
    (BootstrapPhase.ENFORCE_FLIP, BootstrapAct.FLIPPED),
    (BootstrapPhase.KERNEL_DEMOTE, BootstrapAct.FLIPPED),
    (BootstrapPhase.COMPLETE, BootstrapAct.RATIFIED),
]


def _receipt(index: int, phase: BootstrapPhase, act: BootstrapAct, prev_hash: str | None) -> BootstrapReceipt:
    return BootstrapReceipt(
        receipt_id=f"r-{index:03d}-{phase.value.lower()}",
        estate_id="estate-test",
        kernel_version="k0-test-0.1",
        phase=phase,
        act=act,
        prev_receipt_hash=prev_hash,
        observed_at=T0 + timedelta(minutes=index),
    )


def _full_chain() -> list[BootstrapReceipt]:
    chain = [genesis_self_attest(
        estate_id="estate-test", kernel_version="k0-test-0.1",
        kernel_manifest_sha256="a" * 64, observed_at=T0,
    )]
    for index, (phase, act) in enumerate(CEREMONY, start=1):
        chain.append(_receipt(index, phase, act, chain[-1].receipt_hash()))
    return chain


def test_synthetic_genesis_to_complete_chain_verifies() -> None:
    verdict = verify_chain(_full_chain())
    assert verdict.ok, verdict.errors
    assert verdict.length == len(CEREMONY) + 1


def test_empty_chain_and_missing_genesis_fail_closed() -> None:
    assert not verify_chain([]).ok
    headless = _full_chain()[1:]
    verdict = verify_chain(headless)
    assert not verdict.ok
    assert any("genesis" in error for error in verdict.errors)


def test_genesis_shape_is_schema_enforced() -> None:
    with pytest.raises(ValidationError, match="genesis"):
        BootstrapReceipt(
            receipt_id="r-bad", estate_id="e", kernel_version="k",
            phase=BootstrapPhase.HOST_RECONCILE, act=BootstrapAct.RECONCILED,
            prev_receipt_hash=None, observed_at=T0,
        )


def test_never_mint_and_local_only_are_pinned() -> None:
    genesis = _full_chain()[0]
    assert genesis.may_authorize is False
    assert genesis.transmit_class == "local_only"
    with pytest.raises(ValidationError):
        BootstrapReceipt.model_validate(
            {**genesis.model_dump(mode="json"), "may_authorize": True}
        )


def test_hash_chain_break_is_detected() -> None:
    chain = _full_chain()
    tampered = chain[3].model_copy(update={"payload_refs": ["tampered:ref"]})
    verdict = verify_chain([*chain[:3], tampered, *chain[4:]])
    assert not verdict.ok
    assert any("hash-chain break" in error for error in verdict.errors)


def test_measured_probe_before_auth_violates_well_ordering() -> None:
    chain = _full_chain()[:4]  # genesis..SURFACE_OBSERVE (no AUTH yet)
    early_probe = _receipt(9, BootstrapPhase.MEASURED_PROBE, BootstrapAct.PROBED, chain[-1].receipt_hash())
    verdict = verify_chain([*chain, early_probe])
    assert not verdict.ok
    assert any("well-ordering" in error for error in verdict.errors)


def test_phase_regression_and_post_complete_receipts_are_illegal() -> None:
    chain = _full_chain()
    regression = _receipt(20, BootstrapPhase.HOST_RECONCILE, BootstrapAct.RECONCILED, chain[5].receipt_hash())
    verdict = verify_chain([*chain[:6], regression])
    assert not verdict.ok
    assert any("regression" in error for error in verdict.errors)

    after_complete = _receipt(21, BootstrapPhase.COMPLETE, BootstrapAct.RATIFIED, chain[-1].receipt_hash())
    verdict = verify_chain([*chain, after_complete])
    assert not verdict.ok
    assert any("terminal" in error for error in verdict.errors)


def test_kernel_demote_requires_enforce_flip() -> None:
    chain = _full_chain()[:8]  # up to SDLC_GATE_SHADOW, no ENFORCE_FLIP yet
    demote = _receipt(30, BootstrapPhase.KERNEL_DEMOTE, BootstrapAct.FLIPPED, chain[-1].receipt_hash())
    verdict = verify_chain([*chain, demote])
    assert not verdict.ok
    assert any("no-gap" in error for error in verdict.errors)


def test_kill_matrix_prefix_verifies_and_resume_appends(tmp_path: Path) -> None:
    """Interrupt at EVERY phase boundary: the stored prefix verifies (resume-by-projection),
    resume appends the remainder, the full chain verifies, zero duplicate receipts."""
    chain = _full_chain()
    for cut in range(1, len(chain) + 1):
        root = tmp_path / f"cut-{cut:02d}"
        for receipt in chain[:cut]:
            append_receipt(root, receipt)
        prefix_verdict = verify_chain(load_chain(root))
        assert prefix_verdict.ok, (cut, prefix_verdict.errors)
        # resume: append the remainder, never re-appending ratified receipts
        for receipt in chain[cut:]:
            append_receipt(root, receipt)
        resumed = load_chain(root)
        assert verify_chain(resumed).ok
        assert len({r.receipt_id for r in resumed}) == len(chain)  # zero duplicates


def test_append_refuses_duplicates_and_stale_tails(tmp_path: Path) -> None:
    chain = _full_chain()
    append_receipt(tmp_path, chain[0])
    append_receipt(tmp_path, chain[1])
    with pytest.raises(ValueError, match="duplicate|tail"):
        append_receipt(tmp_path, chain[1])  # idempotence: double-append refused
    with pytest.raises(ValueError, match="tail"):
        append_receipt(tmp_path, chain[3])  # skipping a link refused
    with pytest.raises(ValueError, match="genesis"):
        append_receipt(tmp_path / "fresh", chain[1])  # empty chain accepts only genesis


def test_single_instance_lock_fails_closed(tmp_path: Path) -> None:
    with BootstrapLock(tmp_path):
        with pytest.raises(BootstrapLockError, match="held"):
            BootstrapLock(tmp_path).acquire()
    # released → acquirable again
    with BootstrapLock(tmp_path):
        pass
    # a stale lock (crashed holder) still refuses: takeover is explicit, never silent
    stale = BootstrapLock(tmp_path)
    stale.acquire()
    stale._held = False  # simulate a crash: lock file remains, holder gone
    with pytest.raises(BootstrapLockError, match="held"):
        BootstrapLock(tmp_path).acquire()


def test_durable_root_rejects_volatile_filesystems() -> None:
    # the repo checkout itself is on durable media (pytest tmp_path may be tmpfs)
    on_disk = Path(__file__).resolve().parent
    declared = declare_durable_root(on_disk)
    assert declared["root"] == str(on_disk)
    with pytest.raises(DurableRootError, match="volatile"):
        declare_durable_root(Path("/dev/shm"))
