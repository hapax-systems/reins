"""R2.14 — sovereign identity + role registry, minted by the ceremony."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import append_receipt, genesis_self_attest, verify_chain_at
from k0.role_registry import (
    RoleRegistryError,
    RoleSet,
    SovereignIdentity,
    mint_role_registry,
    mint_sovereign_identity,
    role_known,
    sovereign_principal,
)

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


def _fingerprint(key: Path) -> str:
    out = subprocess.run(
        ["ssh-keygen", "-l", "-f", str(key.with_suffix(".pub"))],
        check=True,
        capture_output=True,
        text=True,
    ).stdout.split()
    return out[1]


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


def test_the_sovereign_identity_is_minted_and_legible(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    identity = SovereignIdentity("operator@estate-0", _fingerprint(key))

    assert sovereign_principal(root) is None, "unminted identity reads dark"

    mint_sovereign_identity(root, identity, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert sovereign_principal(root) == "operator@estate-0"
    assert verify_chain_at(root).ok


def test_the_identity_carries_the_fingerprint_not_the_key(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    identity = SovereignIdentity("operator@estate-0", _fingerprint(key))
    mint_sovereign_identity(root, identity, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    from bootstrap_receipt import RECEIPT_CHAIN_FILENAME
    from k0.ratification import SIGNATURE_DIRNAME

    chain_bytes = (root / RECEIPT_CHAIN_FILENAME).read_bytes()
    pub = key.with_suffix(".pub").read_text().strip().split()[1]
    assert pub not in chain_bytes.decode(), "the public key MATERIAL stays off the ledger"
    body = (root / SIGNATURE_DIRNAME / f"{identity.stipulation_id()}.body").read_bytes()
    assert _fingerprint(key).encode() in body, (
        "the fingerprint rides the consented body — the chain pins its digest, the artifact "
        "carries the identity"
    )
    assert pub not in body.decode(), "and the key material is not in the artifact either"

    with pytest.raises(ValueError, match="SHA256"):
        SovereignIdentity("operator@estate-0", "not-a-fingerprint")
    with pytest.raises(ValueError, match="name@host"):
        SovereignIdentity("Operator At Estate", _fingerprint(key))


def test_the_registry_is_elicited_content_and_dispatch_legible(tmp_path: Path) -> None:
    root = _root(tmp_path)
    key = _key(tmp_path)
    roles = RoleSet(("alpha", "beta", "cc-worker", "dev-1"))

    with pytest.raises(RoleRegistryError, match="no role registry is minted"):
        role_known(root, "alpha"), "before the mint, every role is equally unknown — and says so"

    mint_role_registry(root, roles, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    assert role_known(root, "alpha")
    assert role_known(root, "dev-1")
    assert not role_known(root, "gamma"), (
        "a well-formed but unregistered role answers False — legibly distinct from both "
        "malformed input and a missing registry"
    )
    with pytest.raises(RoleRegistryError, match="not a role name"):
        role_known(root, "BAD ROLE")
    assert verify_chain_at(root).ok


def test_registry_shape_laws() -> None:
    with pytest.raises(ValueError, match="empty registry"):
        RoleSet(())
    with pytest.raises(ValueError, match="duplicate roles"):
        RoleSet(("alpha", "alpha"))
    with pytest.raises(ValueError, match="lowercase kebab"):
        RoleSet(("Not A Role",))


def test_a_tampered_registry_body_refuses(tmp_path: Path) -> None:
    from k0.ratification import SIGNATURE_DIRNAME

    root = _root(tmp_path)
    key = _key(tmp_path)
    roles = RoleSet(("alpha", "beta"))
    mint_role_registry(root, roles, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    body = root / SIGNATURE_DIRNAME / f"{roles.stipulation_id()}.body"
    body.write_text('{"roles":["alpha","intruder"]}', encoding="utf-8")
    with pytest.raises(RoleRegistryError, match="not the artifact"):
        role_known(root, "intruder")
