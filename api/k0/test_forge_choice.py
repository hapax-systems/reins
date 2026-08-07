"""R2.13 — the forge choice as a ratified stipulation, tested at the properties."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import append_receipt, genesis_self_attest, verify_chain_at
from k0.forge_choice import (
    FORGE_PROFILES,
    ForgeChoice,
    ForgeConsentError,
    accept,
    present,
    ratified_forge,
)
from k0.ratification import SIGNATURE_DIRNAME, pending

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"
GITHUB = FORGE_PROFILES[ForgeChoice.GITHUB_ONLY]


def _key(tmp_path: Path) -> Path:
    key = tmp_path / "ratifier_ed25519"
    subprocess.run(
        ["ssh-keygen", "-t", "ed25519", "-N", "", "-C", "ratifier@test", "-f", str(key)],
        check=True,
        capture_output=True,
    )
    return key


def _materials(tmp_path: Path, key: Path) -> dict:
    from k0.ratifier import write_allowed_signers

    allowed = tmp_path / "allowed_signers"
    write_allowed_signers(allowed, "ratifier@test", key.with_suffix(".pub").read_text().strip())
    scratch = tmp_path / "scratch"
    scratch.mkdir(exist_ok=True)
    return {"allowed_signers": allowed, "principal": "ratifier@test", "scratch_dir": scratch}


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


def test_the_vocabulary_is_exactly_the_three_legal_answers() -> None:
    assert set(ForgeChoice) == {
        ForgeChoice.GITHUB_ONLY,
        ForgeChoice.FORGE_AGNOSTIC,
        ForgeChoice.LOCAL_GIT_ONLY,
    }, "a fourth rail is a governance act, not an edit"
    for profile in FORGE_PROFILES.values():
        assert profile.tradeoffs, "a choice with no named cost is a marketing page"


def test_present_then_ratify_then_read_back(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    materials = _materials(tmp_path, key)

    assert ratified_forge(root, allow_unauthenticated=True) is None, (
        "no default forge — unconsented reads dark"
    )

    present(root, GITHUB, estate_id=ESTATE, kernel_version=KERNEL)
    assert GITHUB.stipulation_id() in pending(root)
    accept(root, GITHUB, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    got = ratified_forge(root, **materials)
    assert got is not None and got.choice is ForgeChoice.GITHUB_ONLY
    assert got.signature_verified and got.amendments == 0
    assert verify_chain_at(root).ok


def test_the_degraded_posture_is_ratifiable_with_its_cost_named(tmp_path: Path) -> None:
    """local-git-only is the degraded option — and it is RATIFIABLE, with the trade-off in the
    consented bytes, never an invisible fallback."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    local = FORGE_PROFILES[ForgeChoice.LOCAL_GIT_ONLY]
    assert any("DEGRADED" in t for t in local.tradeoffs)

    present(root, local, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, local, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    got = ratified_forge(root, **_materials(tmp_path, key))
    assert got.choice is ForgeChoice.LOCAL_GIT_ONLY


def test_the_hash_only_read_requires_the_opt_in(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    present(root, GITHUB, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, GITHUB, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    with pytest.raises(ForgeConsentError, match="allow_unauthenticated"):
        ratified_forge(root)
    got = ratified_forge(root, allow_unauthenticated=True)
    assert got is not None and got.signature_verified is False


def test_a_tampered_body_refuses_loudly(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    present(root, GITHUB, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, GITHUB, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    body = root / SIGNATURE_DIRNAME / f"{GITHUB.stipulation_id()}.body"
    body.write_text('{"choice":"forge-agnostic","tradeoffs":[]}', encoding="utf-8")
    with pytest.raises(ForgeConsentError):
        ratified_forge(root, allow_unauthenticated=True)


def test_amendment_surfaces_as_data(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    present(root, GITHUB, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, GITHUB, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    agnostic = FORGE_PROFILES[ForgeChoice.FORGE_AGNOSTIC]
    present(root, agnostic, estate_id=ESTATE, kernel_version=KERNEL)
    accept(root, agnostic, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    got = ratified_forge(root, allow_unauthenticated=True)
    assert got.choice is ForgeChoice.FORGE_AGNOSTIC, "the latest consent governs"
    assert got.amendments == 1, "the override is visible, never silent"
