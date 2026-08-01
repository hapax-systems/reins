"""The ratifier key. Every failure path must REFUSE, including the one that cannot run."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest

from k0 import RefusalError
from k0.ratifier import (
    RATIFICATION_NAMESPACE,
    RatifierError,
    sign_ratification,
    verify_ratification,
    write_allowed_signers,
)

PAYLOAD = b"ratify estate-abc123 at 2026-08-01T00:00:00+00:00\n"
PRINCIPAL = "sovereign@hapax"


@pytest.fixture
def keypair(tmp_path) -> Path:
    key = tmp_path / "ratifier"
    subprocess.run(
        ["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(key), "-C", "test"],
        check=True, capture_output=True,
    )
    return key


@pytest.fixture
def signers(tmp_path, keypair) -> Path:
    p = tmp_path / "allowed_signers"
    write_allowed_signers(p, PRINCIPAL, (keypair.with_suffix(".pub")).read_text())
    return p


def test_a_signature_verifies(tmp_path, keypair, signers):
    sig = sign_ratification(PAYLOAD, keypair)
    assert "BEGIN SSH SIGNATURE" in sig
    verify_ratification(
        PAYLOAD, sig, allowed_signers=signers, principal=PRINCIPAL, scratch_dir=tmp_path
    )  # no raise == bound


def test_a_tampered_payload_refuses(tmp_path, keypair, signers):
    sig = sign_ratification(PAYLOAD, keypair)
    with pytest.raises(RefusalError) as e:
        verify_ratification(
            b"ratify estate-EVIL\n", sig,
            allowed_signers=signers, principal=PRINCIPAL, scratch_dir=tmp_path,
        )
    assert "not bound" in e.value.refusal.why
    assert e.value.refusal.legal_next


def test_a_different_key_refuses(tmp_path, keypair, signers):
    other = tmp_path / "impostor"
    subprocess.run(
        ["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(other), "-C", "x"],
        check=True, capture_output=True,
    )
    sig = sign_ratification(PAYLOAD, other)
    with pytest.raises(RefusalError):
        verify_ratification(
            PAYLOAD, sig, allowed_signers=signers, principal=PRINCIPAL, scratch_dir=tmp_path
        )


def test_an_unknown_principal_refuses(tmp_path, keypair, signers):
    sig = sign_ratification(PAYLOAD, keypair)
    with pytest.raises(RefusalError):
        verify_ratification(
            PAYLOAD, sig, allowed_signers=signers, principal="nobody@nowhere", scratch_dir=tmp_path
        )


def test_a_signature_from_another_namespace_cannot_be_replayed(tmp_path, keypair, signers):
    """The property a hand-rolled scheme would omit: any SSHSIG the sovereign made for another
    purpose — a signed commit, a signed file — must not pass as a ratification."""
    foreign = subprocess.run(
        ["ssh-keygen", "-Y", "sign", "-f", str(keypair), "-n", "git", "-"],
        input=PAYLOAD, capture_output=True, check=True,
    ).stdout.decode()
    assert "BEGIN SSH SIGNATURE" in foreign
    with pytest.raises(RefusalError):
        verify_ratification(
            PAYLOAD, foreign, allowed_signers=signers, principal=PRINCIPAL, scratch_dir=tmp_path
        )


def test_a_missing_signers_file_refuses(tmp_path, keypair):
    sig = sign_ratification(PAYLOAD, keypair)
    with pytest.raises(RefusalError):
        verify_ratification(
            PAYLOAD, sig, allowed_signers=tmp_path / "nope",
            principal=PRINCIPAL, scratch_dir=tmp_path,
        )


def test_verification_that_CANNOT_RUN_refuses(tmp_path, keypair, signers, monkeypatch):
    """The arm a naive implementation turns into a pass: no ssh-keygen means UNEVALUABLE, and an
    unevaluable predicate DENIES. It must not silently succeed."""
    sig = sign_ratification(PAYLOAD, keypair)
    monkeypatch.setattr(shutil, "which", lambda _: None)
    with pytest.raises(RefusalError) as e:
        verify_ratification(
            PAYLOAD, sig, allowed_signers=signers, principal=PRINCIPAL, scratch_dir=tmp_path
        )
    assert "ssh-keygen not found" in e.value.refusal.why
    assert e.value.refusal.legal_next


def test_signing_with_a_missing_key_raises(tmp_path):
    with pytest.raises(RatifierError, match="not a file"):
        sign_ratification(PAYLOAD, tmp_path / "absent")


def test_allowed_signers_rejects_junk(tmp_path):
    with pytest.raises(RatifierError, match="OpenSSH public key"):
        write_allowed_signers(tmp_path / "s", PRINCIPAL, "not-a-key")
    with pytest.raises(RatifierError, match="no spaces"):
        write_allowed_signers(tmp_path / "s", "two words", "ssh-ed25519 AAAA")


def test_the_namespace_is_pinned():
    """Changing it invalidates every prior ratification, so it changes only under enforce-flip."""
    assert RATIFICATION_NAMESPACE == "hapax-ratification"


def test_the_payload_never_reaches_disk(tmp_path, keypair, signers):
    sig = sign_ratification(PAYLOAD, keypair)
    verify_ratification(
        PAYLOAD, sig, allowed_signers=signers, principal=PRINCIPAL, scratch_dir=tmp_path
    )
    marker = b"estate-abc123"
    for p in tmp_path.rglob("*"):
        if p.is_file():
            assert marker not in p.read_bytes(), f"payload leaked to {p.name}"
