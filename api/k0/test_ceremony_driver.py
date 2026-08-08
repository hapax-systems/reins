"""The ceremony driver: the operator's key act as one command, pinned."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest
from bootstrap_receipt import load_chain, verify_chain_at
from k0.ceremony import ceremony_complete
from k0.crow_cold_start import cold_start

DRIVER = ["python", "-m", "k0.ceremony_driver"]


def _key(tmp_path: Path) -> Path:
    key = tmp_path / "sovereign_ed25519"
    subprocess.run(
        ["ssh-keygen", "-t", "ed25519", "-N", "", "-C", "operator@test", "-f", str(key)],
        check=True,
        capture_output=True,
    )
    return key


def _scratch(tmp_path: Path) -> Path:
    """A durable scratch root: tmp_path is tmpfs on some hosts, and the durable-root
    guard (correctly) refuses tmpfs. Home-cache scratch is disk-backed everywhere."""
    import shutil

    base = Path.home() / ".cache" / "hapax" / "tmp" / "ceremony-driver-tests"
    root = base / tmp_path.name
    shutil.rmtree(root, ignore_errors=True)
    root.mkdir(parents=True)
    return root


def _run(root: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [*DRIVER, "--root", str(root), *args],
        capture_output=True,
        text=True,
        timeout=120,
        check=False,
    )


def test_full_ceremony_end_to_end(tmp_path: Path) -> None:
    root = _scratch(tmp_path)
    key = _key(tmp_path)
    run = _run(root, "--key", str(key), "--principal", "operator@test", "--with-r22")
    assert run.returncode == 0, run.stderr
    assert ceremony_complete(root)
    assert verify_chain_at(root).ok
    # the R2.2 ratification is on the chain, signed, over the pinned bytes
    import deterministic_segment

    assert any(
        "r22" in r.receipt_id for r in load_chain(root)
    ), "no r22 receipt on the chain"
    assert (
        deterministic_segment.DETERMINISTIC_SEGMENT.digest()
        == deterministic_segment.R22_RATIFIED_PIN
    )
    # and the terminal act can follow it (same estate — one chain, one estate)
    result = cold_start(root, estate_id="estate:local", kernel_version="k0:test")
    assert result.store_receipt == "crow-store-created"


def test_dry_run_writes_nothing(tmp_path: Path) -> None:
    root = tmp_path / "k0"
    run = _run(root, "--key", "/nonexistent", "--principal", "operator@test", "--dry-run")
    assert run.returncode == 0
    assert not root.exists()


def test_missing_key_refuses_with_the_generation_hint(tmp_path: Path) -> None:
    root = tmp_path / "k0"
    run = _run(root, "--key", str(tmp_path / "absent"), "--principal", "operator@test")
    assert run.returncode == 2
    assert "ssh-keygen" in run.stderr


def test_second_genesis_is_refused(tmp_path: Path) -> None:
    root = _scratch(tmp_path)
    key = _key(tmp_path)
    first = _run(root, "--key", str(key), "--principal", "operator@test")
    assert first.returncode == 0, first.stderr
    second = _run(root, "--key", str(key), "--principal", "operator@test")
    assert second.returncode == 0  # idempotent-by-refusal: "already complete", exit 0
    assert "already complete" in second.stdout


def test_volatile_root_is_refused(tmp_path: Path) -> None:
    # /dev/shm is tmpfs — the durable-root declaration must fail closed.
    shm = Path("/dev/shm")
    if not shm.is_dir():
        pytest.skip("no /dev/shm on this host")
    root = shm / f"k0-ceremony-test-{tmp_path.name}"
    key = _key(tmp_path)
    try:
        run = _run(root, "--key", str(key), "--principal", "operator@test")
        assert run.returncode != 0
        assert not (root / "bootstrap-receipts.jsonl").exists()
    finally:
        import shutil

        shutil.rmtree(root, ignore_errors=True)
