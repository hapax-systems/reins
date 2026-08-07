"""The ceremony spine: the P2 stipulations, performed in order, read back through the readers."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import append_receipt, genesis_self_attest, verify_chain_at
from k0.boot_profile import PROFILES, ratified_profile
from k0.ceremony import ceremony_complete, ratify_genesis_stipulations
from k0.egress_consent import EgressAllowlist, ratified_allowlist
from k0.forge_choice import FORGE_PROFILES, ForgeChoice, ratified_forge
from k0.role_registry import role_known, sovereign_principal

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


def test_the_spine_performs_every_consent_and_the_readers_answer(tmp_path: Path) -> None:
    """The integration witness (r3/r4): after the spine runs, every narrowing reads back
    through its own module's reader — the ceremony is a thing that happened, not a claim."""
    root = _root(tmp_path)
    key = _key(tmp_path)
    materials = _materials(tmp_path, key)

    assert not ceremony_complete(root), "before the spine, the ceremony is honestly incomplete"

    result = ratify_genesis_stipulations(
        root,
        principal="operator@estate-0",
        roles=("alpha", "beta"),
        boot_profile=PROFILES["existing-agent-harness"],
        allowlist=EgressAllowlist(hosts=("api.anthropic.com",)),
        forge_profile=FORGE_PROFILES[ForgeChoice.GITHUB_ONLY],
        key_path=key,
        estate_id=ESTATE,
        kernel_version=KERNEL,
    )

    assert sovereign_principal(root) == "operator@estate-0"
    assert role_known(root, "alpha")
    assert ratified_profile(root).profile_id == "existing-agent-harness"
    assert ratified_allowlist(root, **materials).allowlist.hosts == ("api.anthropic.com",)
    assert ratified_forge(root, **materials).choice is ForgeChoice.GITHUB_ONLY
    assert ceremony_complete(root)
    assert verify_chain_at(root).ok
    assert result.sovereign_identity.startswith("sovereign-identity.")


def test_a_refusal_midway_stops_the_ceremony_honestly(tmp_path: Path) -> None:
    """A half-consented genesis is a true state: the chain holds exactly what was consented,
    and completeness reads false rather than papering over the gap."""
    from k0.forge_choice import ForgeConsentError, ForgeProfile

    root = _root(tmp_path)
    key = _key(tmp_path)

    invented = ForgeProfile(ForgeChoice.GITHUB_ONLY, tradeoffs=("caller-invented",))
    with pytest.raises(ForgeConsentError, match="not a registry profile"):
        ratify_genesis_stipulations(
            root,
            principal="operator@estate-0",
            roles=("alpha",),
            boot_profile=PROFILES["existing-agent-harness"],
            allowlist=EgressAllowlist(hosts=("api.anthropic.com",)),
            forge_profile=invented,
            key_path=key,
            estate_id=ESTATE,
            kernel_version=KERNEL,
        )
    assert not ceremony_complete(root)
    assert sovereign_principal(root) == "operator@estate-0", (
        "identity and registry DID land — the chain holds exactly the consents given"
    )
    assert ratified_profile(root) is not None
    assert ratified_allowlist(root, allow_unauthenticated=True) is not None
    assert ratified_forge(root, allow_unauthenticated=True) is None, (
        "the refused act never reached the chain"
    )


def test_completeness_requires_the_foundation_too(tmp_path: Path) -> None:
    """codex r5: a ceremony missing its identity or registry is not complete, however many
    narrowings landed."""
    root = _root(tmp_path)
    key = _key(tmp_path)

    # Narrowings only — no identity, no registry.
    from k0.boot_profile import present as present_profile
    from k0.egress_consent import accept as accept_allowlist
    from k0.egress_consent import elicit_allowlist
    from k0.forge_choice import accept as accept_forge
    from k0.forge_choice import present as present_forge
    from k0.ratification import ratify

    profile = PROFILES["existing-agent-harness"]
    allowlist = EgressAllowlist(hosts=("api.anthropic.com",))
    forge = FORGE_PROFILES[ForgeChoice.GITHUB_ONLY]
    present_profile(root, profile, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, profile.stipulation(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    elicit_allowlist(root, allowlist, estate_id=ESTATE, kernel_version=KERNEL)
    accept_allowlist(root, allowlist, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    present_forge(root, forge, estate_id=ESTATE, kernel_version=KERNEL)
    accept_forge(root, forge, key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    assert not ceremony_complete(root), (
        "every narrowing present but no identity and no registry — complete would be a lie"
    )
