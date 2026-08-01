"""Hardening from the R2.1 review pass, pinned as tests.

Each of these is a defect found by review rather than by the suite — which is the point of recording
them here. Every one had passed 300+ tests on the machine that wrote it.
"""

from __future__ import annotations

import os
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from bootstrap_receipt import (
    LOCK_FILENAME,
    BootstrapAct,
    BootstrapLock,
    BootstrapPhase,
    BootstrapReceipt,
    append_receipt,
    genesis_self_attest,
    load_chain,
    verify_chain,
)
from k0.identity import EstateIdentity, IdentitySeedError


def _genesis(estate: str = "e1") -> BootstrapReceipt:
    return genesis_self_attest(estate_id=estate, kernel_version="k0", kernel_manifest_sha256="ab" * 32)


# --- the ledger --------------------------------------------------------------------------------


def test_the_lock_token_cannot_collide_by_coincidence(tmp_path: Path) -> None:
    """pid + timestamp is not unique: pids are reused, and two processes in different pid
    namespaces can hold the same one. release() treats an equal token as proof of ownership, so a
    collision lets the wrong process delete a lock it never held."""
    tokens = set()
    for _ in range(20):
        with BootstrapLock(tmp_path):
            tokens.add((tmp_path / LOCK_FILENAME).read_text(encoding="utf-8"))
    assert len(tokens) == 20, "lock tokens repeated — the token is not collision-proof"


def test_an_undecodable_chain_is_reported_as_corruption(tmp_path: Path) -> None:
    """The chain is UTF-8 JSON lines, so undecodable bytes mean the LEDGER is damaged. Surfacing a
    raw codec error makes the caller interpret it."""
    append_receipt(tmp_path, _genesis())
    chain_path = tmp_path / "bootstrap-receipts.jsonl"
    chain_path.write_bytes(b'{"a": "\xff\xfe not utf8"}\n')
    with pytest.raises(ValueError, match="chain is corrupt: not valid UTF-8"):
        load_chain(tmp_path)


def test_a_timezone_naive_receipt_is_refused() -> None:
    """A naive datetime reads like a moment and means a different instant on every host that loads
    it — so the chain orders differently depending on who reads it, and a ratification verified at
    observed_at lands at the wrong instant."""
    with pytest.raises(ValueError, match="timezone-naive"):
        BootstrapReceipt(
            receipt_id="r1",
            estate_id="e1",
            kernel_version="k0",
            phase=BootstrapPhase.K0_ACTIVE,
            act=BootstrapAct.MINTED,
            payload_refs=[],
            prev_receipt_hash=None,
            observed_at=datetime(2026, 8, 1, 12, 0, 0),  # noqa: DTZ001 — the point of the test
        )


def test_the_genesis_manifest_digest_must_be_a_digest() -> None:
    """The genesis receipt is the chain's root claim about WHICH kernel ran. An unparseable digest
    makes every drift check downstream compare against nothing."""
    for bad in ("", "not-a-digest", "ab" * 31, "zz" * 32):
        with pytest.raises(ValueError, match="is not a sha256 digest"):
            genesis_self_attest(estate_id="e1", kernel_version="k0", kernel_manifest_sha256=bad)
    genesis_self_attest(estate_id="e1", kernel_version="k0", kernel_manifest_sha256="AB" * 32)


def test_one_chain_belongs_to_exactly_one_estate() -> None:
    """Two estates spliced into one chain would let a foreign receipt inherit this chain's
    hash-linkage and read as locally attested."""
    first = _genesis("estate-a")
    intruder = BootstrapReceipt(
        receipt_id="r2",
        estate_id="estate-b",
        kernel_version="k0",
        phase=BootstrapPhase.K0_ACTIVE,
        act=BootstrapAct.RATIFIED,
        payload_refs=[],
        prev_receipt_hash=first.receipt_hash(),
        observed_at=datetime.now(UTC) + timedelta(seconds=1),
    )
    verdict = verify_chain([first, intruder])
    assert not verdict.ok
    assert any("one chain belongs to exactly one estate" in e for e in verdict.errors)


# --- the identity seed -------------------------------------------------------------------------


def test_a_corrupt_minted_at_raises_the_seed_error(tmp_path: Path) -> None:
    """Every other corruption in the loader raises IdentitySeedError. A bare ValueError here escapes
    the contract, so a caller catching IdentitySeedError to refuse cleanly would crash instead."""
    seed = tmp_path / "estate-identity.json"
    seed.write_text('{"seed_schema": 1, "estate_id": "abc", "minted_at": "not-a-date"}', encoding="utf-8")
    with pytest.raises(IdentitySeedError, match="unparseable minted_at"):
        EstateIdentity.from_json(seed.read_text(encoding="utf-8"))


def test_the_seed_is_fsynced_not_merely_renamed(tmp_path: Path, monkeypatch) -> None:
    """os.replace guarantees no reader sees a torn file; it guarantees nothing about power loss. The
    estate identity is the one file that cannot be re-derived — a lost seed orphans the whole chain."""
    from k0 import identity as ident

    synced: list[str] = []
    real_fsync = os.fsync
    monkeypatch.setattr(os, "fsync", lambda fd: (synced.append("fsync"), real_fsync(fd))[1])
    ident.load_or_mint(tmp_path, chain_exists=False)
    assert len(synced) >= 2, "expected an fsync of the seed AND of its directory"


# --- the host floor ----------------------------------------------------------------------------


def test_a_version_agnostic_dependency_is_satisfied_by_presence(monkeypatch) -> None:
    """git is declared at 'any version'. Refusing because an unfamiliar build printed an unfamiliar
    banner would deny on a fact the floor never claimed to care about."""
    from k0 import host_floor as hf

    monkeypatch.setattr(hf, "_detect", lambda entry: None)  # nothing's version is readable
    monkeypatch.setattr(
        hf, "FLOOR", tuple(e for e in hf.FLOOR if e.min_version is None)
    )
    hf.require()  # must NOT raise: presence is all this entry ever required


def test_corruption_reaches_the_verdict_not_the_callers_stack(tmp_path: Path) -> None:
    """verify_chain_at exists to REPORT corruption as ok=False. The undecodable-bytes arm added a
    new ValueError; without it in the tuple, the one corruption this verifier exists for is the one
    that escapes as a traceback."""
    from bootstrap_receipt import verify_chain_at

    append_receipt(tmp_path, _genesis())
    (tmp_path / "bootstrap-receipts.jsonl").write_bytes(b"\xff\xfe not utf8\n")
    verdict = verify_chain_at(tmp_path)
    assert verdict.ok is False
    assert any("unreadable or corrupt" in e for e in verdict.errors)


def test_only_one_minter_can_win_the_seed(tmp_path: Path) -> None:
    """Two processes can both pass the existence check before either writes. With os.replace both
    would mint and the loser would return an identity that is NOT on disk — attributing its receipts
    to an estate that does not exist."""
    import multiprocessing as mp

    from k0 import identity as ident

    def mint(root: str, q) -> None:
        q.put(ident.load_or_mint(Path(root), chain_exists=False).estate_id)

    ctx = mp.get_context("fork")
    q = ctx.Queue()
    procs = [ctx.Process(target=mint, args=(str(tmp_path), q)) for _ in range(6)]
    for p in procs:
        p.start()
    got = [q.get(timeout=60) for _ in procs]
    for p in procs:
        p.join(timeout=60)

    on_disk = ident.EstateIdentity.from_json((tmp_path / ident.SEED_FILENAME).read_text(encoding="utf-8"))
    assert set(got) == {on_disk.estate_id}, f"minters disagreed with the persisted seed: {set(got)}"
