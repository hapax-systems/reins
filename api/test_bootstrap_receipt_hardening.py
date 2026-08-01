"""Regression tests for the bootstrap-receipt-v1 hardening pass.

Each test corresponds to a defect that the Slice 1 suite (PR 20, 12 tests) did not catch.
The concurrency test is the important one: before the fix, 60/60 trials with 4 concurrent
writers admitted every writer and produced a chain that verify_chain then rejected — and
because the chain is append-only, that corruption is terminal.
"""
from __future__ import annotations

import multiprocessing as mp
import os
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from pydantic import ValidationError

from bootstrap_receipt import (
    LADDER_DIGEST,
    PHASE_LADDER,
    BootstrapAct,
    BootstrapLock,
    BootstrapPhase,
    BootstrapReceipt,
    append_receipt,
    assert_ladder_undrifted,
    genesis_self_attest,
    ladder_digest,
    load_chain,
    verify_chain,
)


def _genesis(root: Path, estate: str = "e") -> BootstrapReceipt:
    receipt = genesis_self_attest(
        estate_id=estate, kernel_version="k0", kernel_manifest_sha256="ab" * 32
    )
    append_receipt(root, receipt)
    return receipt


def _next(prev_hash: str, rid: str, estate: str = "e") -> BootstrapReceipt:
    return BootstrapReceipt(
        receipt_id=rid,
        estate_id=estate,
        kernel_version="k0",
        phase=BootstrapPhase.HOST_RECONCILE,
        act=BootstrapAct.RECONCILED,
        prev_receipt_hash=prev_hash,
        observed_at=datetime.now(UTC),
    )


def _racer(root: str, rid: str, prev_hash: str, barrier, queue) -> None:
    receipt = _next(prev_hash, rid)
    barrier.wait()
    try:
        append_receipt(Path(root), receipt)
        queue.put("appended")
    except Exception as exc:  # noqa: BLE001 — the refusal type is the assertion below
        queue.put(f"refused:{type(exc).__name__}")


# --- R0.6: concurrent first-inits must not fork the chain -------------------------------


def test_concurrent_appends_serialise_and_cannot_fork_the_chain(tmp_path: Path) -> None:
    """The defect: append_receipt did load_chain() then open("a") with no lock between
    them. BootstrapLock existed and was tested, but nothing on the write path consulted it,
    so it could not prevent the thing it was built to prevent."""
    genesis = _genesis(tmp_path)
    writers = 4
    barrier = mp.Barrier(writers)
    queue: mp.Queue = mp.Queue()
    procs = [
        mp.Process(
            target=_racer,
            args=(str(tmp_path), f"r{i}", genesis.receipt_hash(), barrier, queue),
        )
        for i in range(writers)
    ]
    for proc in procs:
        proc.start()
    for proc in procs:
        proc.join()
    # BOUNDED. An unbounded queue.get() blocks forever if a child dies before reporting, turning
    # a failed test into a hung CI job that reports nothing at all.
    outcomes = [queue.get(timeout=60) for _ in procs]

    assert outcomes.count("appended") == 1, f"more than one writer admitted: {outcomes}"
    chain = load_chain(tmp_path)
    assert len(chain) == 2
    verdict = verify_chain(chain)
    assert verdict.ok, verdict.errors


def test_append_refuses_a_symlinked_chain(tmp_path: Path) -> None:
    """O_NOFOLLOW: the chain path must not be redirectable by replacing it with a symlink."""
    _genesis(tmp_path, "e7")
    chain_path = tmp_path / "bootstrap-receipts.jsonl"
    elsewhere = tmp_path / "elsewhere.jsonl"
    elsewhere.write_text(chain_path.read_text(encoding="utf-8"), encoding="utf-8")
    chain_path.unlink()
    chain_path.symlink_to(elsewhere)

    with pytest.raises(OSError):
        append_receipt(tmp_path, _next("x" * 64, "via-symlink", "e7"))


# --- R0.6: the lock must not be stealable ----------------------------------------------


def test_release_does_not_delete_a_successors_lock(tmp_path: Path) -> None:
    """After an explicit human takeover, the crashed-out holder's release() must not
    delete the NEW holder's lock — that silently readmits concurrency at exactly the
    moment the operator intervened to prevent it."""
    first = BootstrapLock(tmp_path)
    first.acquire()
    os.unlink(first.path)  # operator inspects and takes over

    second = BootstrapLock(tmp_path)
    second.acquire()
    first.release()  # the stale holder unwinds

    assert second.path.exists(), "successor's lock was deleted by a stale holder"
    second.release()
    assert not second.path.exists()


# --- R1.3: clock sanity -----------------------------------------------------------------


def test_receipt_may_not_expire_before_it_was_observed() -> None:
    now = datetime.now(UTC)
    with pytest.raises(ValidationError, match="clock-sanity"):
        BootstrapReceipt(
            receipt_id="incoherent",
            estate_id="e",
            kernel_version="k0",
            phase=BootstrapPhase.K0_ACTIVE,
            act=BootstrapAct.MINTED,
            prev_receipt_hash=None,
            observed_at=now,
            stale_after=now - timedelta(days=365),
        )


def test_coherent_freshness_still_accepted() -> None:
    now = datetime.now(UTC)
    receipt = BootstrapReceipt(
        receipt_id="coherent",
        estate_id="e",
        kernel_version="k0",
        phase=BootstrapPhase.K0_ACTIVE,
        act=BootstrapAct.MINTED,
        prev_receipt_hash=None,
        observed_at=now,
        stale_after=now + timedelta(hours=1),
    )
    assert receipt.stale_after is not None


# --- R0.4: the phase-legality law is data, and pinned -----------------------------------


def test_phase_ladder_is_declared_not_derived_from_enum_order() -> None:
    """The law must not be an artifact of the order enum members appear in the source:
    an alphabetising refactor previously redefined the ceremony's phase law silently."""
    assert_ladder_undrifted()
    assert ladder_digest() == LADDER_DIGEST
    assert set(PHASE_LADDER) == set(BootstrapPhase), "every phase must be placed explicitly"
    assert len(PHASE_LADDER) == len(set(PHASE_LADDER))
    # the well-ordering the ladder exists to encode
    assert PHASE_LADDER.index(BootstrapPhase.SURFACE_OBSERVE) < PHASE_LADDER.index(
        BootstrapPhase.AUTH_MATERIALIZE
    )
    assert PHASE_LADDER.index(BootstrapPhase.AUTH_MATERIALIZE) < PHASE_LADDER.index(
        BootstrapPhase.MEASURED_PROBE
    )
    assert PHASE_LADDER.index(BootstrapPhase.ENFORCE_FLIP) < PHASE_LADDER.index(
        BootstrapPhase.KERNEL_DEMOTE
    )


# --- Second hardening pass: defects found by an independent audit lane ------------------
# All four were reproduced by execution against the merged module before being fixed.


def test_durable_root_denies_when_it_cannot_evaluate(monkeypatch, tmp_path: Path) -> None:
    """The guard enforcing R0.7 was itself violating K0.1.

    _mount_fstype returns the literal 'unknown' when /proc/mounts is unreadable, and 'unknown'
    is not in the volatile set — so declare_durable_root PASSED whenever it could not evaluate.
    Measured: with /proc/mounts unreadable it accepted /dev/shm. A durability guard that fails
    open is worse than none, because it reports a durable root that is not one."""
    import bootstrap_receipt as br

    monkeypatch.setattr(br, "_mount_fstype", lambda _p: br._UNKNOWN_FSTYPE)
    with pytest.raises(br.DurableRootError, match="could not determine"):
        br.declare_durable_root(tmp_path)


def test_complete_chain_must_evidence_the_ceremony() -> None:
    """verify_chain accepted [K0_ACTIVE, COMPLETE] as ok=True — a two-link chain certifying a
    ceremony in which nothing was ratified, nothing minted, and no flip occurred. The ladder
    forbade going backward but said nothing about skipping forward."""
    genesis = genesis_self_attest(
        estate_id="e", kernel_version="k", kernel_manifest_sha256="ab" * 32
    )
    straight_to_done = BootstrapReceipt(
        receipt_id="done",
        estate_id="e",
        kernel_version="k",
        phase=BootstrapPhase.COMPLETE,
        act=BootstrapAct.FLIPPED,
        prev_receipt_hash=genesis.receipt_hash(),
        observed_at=datetime.now(UTC),
    )
    verdict = verify_chain([genesis, straight_to_done])
    assert not verdict.ok
    assert any("without evidencing the ceremony" in error for error in verdict.errors)


def test_payload_refs_refuse_bare_values() -> None:
    """'refs, never secrets' was a comment on an unvalidated list[str]. A receipt carrying
    credential-shaped bare tokens was accepted."""
    now = datetime.now(UTC)
    for leaked in ("sk-ant-api03-NOTAREALKEY", "password=hunter2", "AKIAIOSFODNN7EXAMPLE"):
        with pytest.raises(ValidationError, match="is not a ref"):
            BootstrapReceipt(
                receipt_id="leak",
                estate_id="e",
                kernel_version="k",
                phase=BootstrapPhase.K0_ACTIVE,
                act=BootstrapAct.MINTED,
                prev_receipt_hash=None,
                observed_at=now,
                payload_refs=[leaked],
            )
    # a properly schemed ref still passes
    ok = BootstrapReceipt(
        receipt_id="fine",
        estate_id="e",
        kernel_version="k",
        phase=BootstrapPhase.K0_ACTIVE,
        act=BootstrapAct.MINTED,
        prev_receipt_hash=None,
        observed_at=now,
        payload_refs=["kernel-manifest:sha256:" + "ab" * 32],
    )
    assert ok.payload_refs


def test_corrupt_ledger_is_a_verdict_not_a_traceback(tmp_path: Path) -> None:
    """load_chain raised on a malformed line, so the fail-closed verifier could not report on
    the one thing it most needs to report on."""
    from bootstrap_receipt import verify_chain_at

    genesis = genesis_self_attest(
        estate_id="e", kernel_version="k", kernel_manifest_sha256="ab" * 32
    )
    append_receipt(tmp_path, genesis)
    with (tmp_path / "bootstrap-receipts.jsonl").open("a", encoding="utf-8") as handle:
        handle.write("{not valid json\n")

    verdict = verify_chain_at(tmp_path)
    assert not verdict.ok
    assert any("unreadable or corrupt" in error for error in verdict.errors)
