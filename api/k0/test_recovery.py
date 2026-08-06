"""Key rotation and loss. The test that matters: rotation must not erase history."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from k0 import RefusalError
from k0.ratifier import RatifierError, sign_ratification, verify_ratification
from k0.recovery import SignerEntry, rotate, rotation_record, write_signers

PAYLOAD = b"ratify estate-abc123\n"


def _key(tmp_path: Path, name: str) -> Path:
    k = tmp_path / name
    subprocess.run(
        ["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(k), "-C", name],
        check=True, capture_output=True,
    )
    return k


def test_a_rotated_key_STILL_VERIFIES_ITS_OWN_HISTORY(tmp_path):
    """The whole point. ssh-keygen validates against the CURRENT time, so closing a key's window
    breaks every ratification it made — unless verification supplies the signature's own time.
    The receipt chain carries observed_at, so it can."""
    old = _key(tmp_path, "old")
    new = _key(tmp_path, "new")

    signed_at = datetime.now(UTC) - timedelta(days=30)
    rotated_at = datetime.now(UTC) - timedelta(days=1)
    sig = sign_ratification(PAYLOAD, old)

    entries = rotate(
        retiring=SignerEntry("old@hapax", old.with_suffix(".pub").read_text(),
                             valid_after=signed_at - timedelta(days=1)),
        successor_principal="new@hapax",
        successor_public_key=new.with_suffix(".pub").read_text(),
        at=rotated_at,
    )
    signers = tmp_path / "allowed_signers"
    write_signers(signers, entries)

    # verified at the moment it was made: still good
    verify_ratification(PAYLOAD, sig, allowed_signers=signers, principal="old@hapax",
                        scratch_dir=tmp_path, verify_time=signed_at)

    # verified at NOW: correctly refused — the key is retired and cannot bind anything new
    with pytest.raises(RefusalError):
        verify_ratification(PAYLOAD, sig, allowed_signers=signers, principal="old@hapax",
                            scratch_dir=tmp_path)


def test_the_successor_binds_new_ratifications(tmp_path):
    old, new = _key(tmp_path, "old"), _key(tmp_path, "new")
    entries = rotate(
        retiring=SignerEntry("old@hapax", old.with_suffix(".pub").read_text()),
        successor_principal="new@hapax",
        successor_public_key=new.with_suffix(".pub").read_text(),
        at=datetime.now(UTC) - timedelta(days=1),
    )
    signers = tmp_path / "s"
    write_signers(signers, entries)
    verify_ratification(PAYLOAD, sign_ratification(PAYLOAD, new), allowed_signers=signers,
                        principal="new@hapax", scratch_dir=tmp_path)


def test_the_retired_key_is_KEPT_not_deleted(tmp_path):
    old, new = _key(tmp_path, "old"), _key(tmp_path, "new")
    entries = rotate(
        retiring=SignerEntry("old@hapax", old.with_suffix(".pub").read_text()),
        successor_principal="new@hapax",
        successor_public_key=new.with_suffix(".pub").read_text(),
    )
    assert len(entries) == 2
    signers = tmp_path / "s"
    write_signers(signers, entries)
    body = signers.read_text()
    assert "old@hapax" in body and "valid-before=" in body
    assert "new@hapax" in body and "valid-after=" in body


def test_reusing_the_principal_is_refused(tmp_path):
    old = _key(tmp_path, "old")
    with pytest.raises(RatifierError, match="DIFFERENT principal"):
        rotate(
            retiring=SignerEntry("same@hapax", old.with_suffix(".pub").read_text()),
            successor_principal="same@hapax",
            successor_public_key=old.with_suffix(".pub").read_text(),
        )


def test_rotating_before_the_key_was_valid_is_refused(tmp_path):
    old = _key(tmp_path, "old")
    with pytest.raises(RatifierError, match="never-usable"):
        rotate(
            retiring=SignerEntry("old@hapax", old.with_suffix(".pub").read_text(),
                                 valid_after=datetime.now(UTC)),
            successor_principal="new@hapax",
            successor_public_key=old.with_suffix(".pub").read_text(),
            at=datetime.now(UTC) - timedelta(days=5),
        )


def test_an_empty_signers_file_is_refused(tmp_path):
    with pytest.raises(RatifierError, match="no key could ratify"):
        write_signers(tmp_path / "s", [])


def test_an_inverted_window_is_refused(tmp_path):
    old = _key(tmp_path, "old")
    now = datetime.now(UTC)
    with pytest.raises(RatifierError, match="never be usable"):
        write_signers(tmp_path / "s", [
            SignerEntry("x@hapax", old.with_suffix(".pub").read_text(),
                        valid_after=now, valid_before=now - timedelta(days=1))
        ])


def test_a_rotation_record_must_state_its_reason(tmp_path):
    """An auditor must be able to tell a planned rotation from a key LOSS: only the former could
    have been authorised by the outgoing sovereign."""
    with pytest.raises(RatifierError, match="stated reason"):
        rotation_record(retiring_principal="a", successor_principal="b", reason="  ")


def test_the_rotation_record_is_honest_about_its_authority():
    rec = rotation_record(
        retiring_principal="old@hapax", successor_principal="new@hapax",
        reason="key lost: laptop failure, no backup",
    )
    assert rec["act"] == "rotated"
    assert "cannot derive from the receipt chain" in rec["authority"]
    assert "proves WHEN" in rec["authority"]
    assert "key lost" in rec["reason"]
