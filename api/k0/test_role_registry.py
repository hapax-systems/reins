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


def test_the_read_back_validates_the_registry_shape(tmp_path: Path) -> None:
    """codex r1: a parsed-but-wrong body is not a registry."""
    import hashlib as _hashlib

    from k0.ratification import SIGNATURE_DIRNAME, Stipulation, propose, ratify

    sub = tmp_path / "bad"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    bad = b'{"roles":["alpha",1]}'
    sid = f"role-registry.{_hashlib.sha256(bad).hexdigest()[:16]}"
    stip = Stipulation.over(sid, "ROLE REGISTRY: shape test", bad)
    (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
    (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
    propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RoleRegistryError, match="not the canonical shape"):
        role_known(root, "alpha")


def test_the_identity_unexpected_fields_branch_and_the_chain_break_branch(tmp_path: Path) -> None:
    """claude r1: both refusal branches, exercised."""
    import hashlib as _hashlib
    import json as _json

    from bootstrap_receipt import RECEIPT_CHAIN_FILENAME
    from k0.ratification import SIGNATURE_DIRNAME, Stipulation, propose, ratify

    sub = tmp_path / "extra"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    bad = b'{"key_fingerprint":"SHA256:x","principal":"operator@estate-0","surprise":true}'
    sid = f"sovereign-identity.{_hashlib.sha256(bad).hexdigest()[:16]}"
    stip = Stipulation.over(sid, "SOVEREIGN IDENTITY: extra-field test", bad)
    (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
    (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
    propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RoleRegistryError, match="unexpected fields"):
        sovereign_principal(root)

    sub = tmp_path / "broken"
    sub.mkdir()
    root = _root(sub)
    key = _key(sub)
    mint_sovereign_identity(
        root, SovereignIdentity("operator@estate-0", _fingerprint(key)),
        key_path=key, estate_id=ESTATE, kernel_version=KERNEL,
    )
    chain_path = root / RECEIPT_CHAIN_FILENAME
    rows = chain_path.read_text(encoding="utf-8").splitlines()
    forged = _json.loads(rows[1])
    forged["payload_refs"] = ["stipulation:sha256:" + "f" * 64]
    rows[1] = _json.dumps(forged)
    chain_path.write_text("\n".join(rows) + "\n", encoding="utf-8")
    with pytest.raises(RoleRegistryError, match="fails verification"):
        sovereign_principal(root)


def test_an_identity_naming_a_different_key_is_refused(tmp_path: Path) -> None:
    """codex r2: the sovereign identity must be the signer's own key — a mismatch is a false
    witness, refused before any row is written."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    (tmp_path / "other").mkdir()
    other = _key(tmp_path / "other")

    impostor = SovereignIdentity("operator@estate-0", _fingerprint(other))
    with pytest.raises(RoleRegistryError, match="the signer's own"):
        mint_sovereign_identity(root, impostor, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert sovereign_principal(root) is None, "a refused identity writes nothing"


def test_the_ceremony_composition_mints_identity_then_registry(tmp_path: Path) -> None:
    """codex r3: the ceremony invokes the mints — one entry point, identity first, registry
    second, and the registry's consent rows are signed by the identity just bound."""
    from k0.role_registry import mint_genesis_identity

    root = _root(tmp_path)
    key = _key(tmp_path)
    mint_genesis_identity(
        root,
        principal="operator@estate-0",
        roles=("alpha", "beta"),
        key_path=key,
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )
    assert sovereign_principal(root) == "operator@estate-0"
    assert role_known(root, "alpha")
    assert verify_chain_at(root).ok


def test_the_missing_body_branch_refuses(tmp_path: Path) -> None:
    """claude r3: a consented body that is gone refuses with a legal next, never a bare OSError."""
    from k0.ratification import SIGNATURE_DIRNAME

    root = _root(tmp_path)
    key = _key(tmp_path)
    roles = RoleSet(("alpha",))
    mint_role_registry(root, roles, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    (root / SIGNATURE_DIRNAME / f"{roles.stipulation_id()}.body").unlink()
    with pytest.raises(RoleRegistryError, match="cannot be read") as exc:
        role_known(root, "alpha")
    assert exc.value.refusal is not None and exc.value.refusal.legal_next.strip()


def test_a_noncanonical_principal_on_readback_is_refused(tmp_path: Path) -> None:
    """codex r3: parses-but-illegal is not canonical, even signed."""
    import hashlib as _hashlib

    from k0.ratification import SIGNATURE_DIRNAME, Stipulation, propose, ratify

    root = _root(tmp_path)
    key = _key(tmp_path)
    bad = b'{"key_fingerprint":"SHA256:x","principal":"Not A Principal"}'
    sid = f"sovereign-identity.{_hashlib.sha256(bad).hexdigest()[:16]}"
    stip = Stipulation.over(sid, "SOVEREIGN IDENTITY: noncanonical test", bad)
    (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
    (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
    propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RoleRegistryError, match="construction laws"):
        sovereign_principal(root)


def test_undecodable_body_is_a_refusal_with_a_next_action(tmp_path: Path) -> None:
    """codex r3: malformed bytes carry a governed next action, never a bare decode error."""
    import hashlib as _hashlib

    from k0.ratification import SIGNATURE_DIRNAME, Stipulation, propose, ratify

    root = _root(tmp_path)
    key = _key(tmp_path)
    bad = b"not json at all{"
    sid = f"role-registry.{_hashlib.sha256(bad).hexdigest()[:16]}"
    stip = Stipulation.over(sid, "ROLE REGISTRY: decode test", bad)
    (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
    (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
    propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RoleRegistryError, match="not decodable") as exc:
        role_known(root, "alpha")
    assert exc.value.refusal is not None and exc.value.refusal.legal_next.strip()


def test_readback_enforces_the_empty_and_duplicate_laws(tmp_path: Path) -> None:
    """codex r5: the read-back enforces ALL the construction laws, not only the grammar."""
    import hashlib as _hashlib

    from k0.ratification import SIGNATURE_DIRNAME, Stipulation, propose, ratify

    for bad in (b'{"roles":[]}', b'{"roles":["alpha","alpha"]}'):
        sub = tmp_path / str(abs(hash(bad)))
        sub.mkdir()
        root = _root(sub)
        key = _key(sub)
        sid = f"role-registry.{_hashlib.sha256(bad).hexdigest()[:16]}"
        stip = Stipulation.over(sid, "ROLE REGISTRY: law test", bad)
        (root / SIGNATURE_DIRNAME).mkdir(exist_ok=True)
        (root / SIGNATURE_DIRNAME / f"{sid}.body").write_bytes(bad)
        propose(root, stip, estate_id=ESTATE, kernel_version=KERNEL)
        ratify(root, stip, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
        with pytest.raises(RoleRegistryError, match="not the canonical shape"):
            role_known(root, "alpha")


def test_a_role_list_cannot_mutate_after_construction() -> None:
    """CodeRabbit: a list passed to RoleSet must be normalized — frozen blocks reassignment,
    not in-place mutation of the caller's list."""
    as_list = ["alpha", "beta"]
    roles = RoleSet(as_list)  # type: ignore[arg-type]
    as_list.append("intruder")
    assert "intruder" not in roles.roles and isinstance(roles.roles, tuple)


def test_trailing_newline_is_not_a_role() -> None:
    """CodeRabbit: match() with $ accepts a trailing newline — fullmatch does not."""
    with pytest.raises(ValueError, match="lowercase kebab"):
        RoleSet(("alpha\n",))


def test_partial_mint_inputs_validate_before_any_durable_act(tmp_path: Path) -> None:
    """CodeRabbit: a bad role set must not leave a minted identity behind it."""
    from k0.role_registry import mint_genesis_identity

    root = _root(tmp_path)
    key = _key(tmp_path)
    with pytest.raises(ValueError, match="lowercase kebab"):
        mint_genesis_identity(
            root,
            principal="operator@estate-0",
            roles=("BAD ROLE",),
            key_path=key,
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )
    assert sovereign_principal(root) is None, "nothing minted before validation succeeded"


def test_key_inspection_failure_is_a_governed_refusal(tmp_path: Path) -> None:
    from k0.role_registry import _fingerprint_of

    with pytest.raises(RoleRegistryError, match="fingerprint"):
        _fingerprint_of(tmp_path / "no-such-key")
