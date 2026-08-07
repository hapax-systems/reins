"""R2.16 — provider ToS conformance: the forbidden class is unrepresentable, legality shapes."""

from __future__ import annotations

import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import append_receipt, genesis_self_attest
from k0.key_capture import PROVIDER_PROBE_ENDPOINTS, MemoryStore, validate_key
from k0.provider_legality import (
    PROVIDER_LEGALITY,
    AcquisitionMode,
    capturable_providers,
    key_capture_legal,
    legal_acquisition_modes,
)

ESTATE = "estate-0000000000000000"
KERNEL = "k0-test"
NAME = "frontier-provider-key"


def _key(tmp_path: Path) -> Path:
    key = tmp_path / "ratifier_ed25519"
    subprocess.run(
        ["ssh-keygen", "-t", "ed25519", "-N", "", "-C", "ratifier@test", "-f", str(key)],
        check=True,
        capture_output=True,
    )
    return key


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


def test_the_forbidden_class_is_unrepresentable() -> None:
    """The gateway/relay/proxy/shared-entitlement wiring the ToS determination rejected has no
    VALUE in the acquisition-mode vocabulary — the kit cannot architect what it cannot name."""
    forbidden = ("gateway", "relay", "proxy", "shared", "resell", "pool")
    for mode in AcquisitionMode:
        for word in forbidden:
            assert word not in mode.value and word not in mode.name.lower(), (
                f"{mode} drifts toward the forbidden class — the vocabulary's negative space "
                "is the control, and adding a member here is a governance act"
            )


def test_legality_is_fail_closed_for_unknown_providers() -> None:
    with pytest.raises(KeyError, match="no ToS determination on record"):
        legal_acquisition_modes("a-provider-with-no-determination")
    assert not key_capture_legal("a-provider-with-no-determination")


def test_the_determinations_on_record() -> None:
    assert legal_acquisition_modes("anthropic") == frozenset(
        {AcquisitionMode.BYOK, AcquisitionMode.OAUTH_ENTITLEMENT}
    )
    assert legal_acquisition_modes("openai") == frozenset({AcquisitionMode.BYOK})
    for entry in PROVIDER_LEGALITY.values():
        assert entry.basis.strip(), "a mode with no cited determination is an assertion"
    assert PROVIDER_LEGALITY["anthropic"].determination_class == "estate-verdict"
    assert PROVIDER_LEGALITY["openai"].determination_class == "provider-terms", (
        "the weaker standing must be visible as data, never silently equal"
    )


def test_elicitation_shaping_matches_the_probe_registry() -> None:
    got = capturable_providers()
    assert got == tuple(sorted(PROVIDER_PROBE_ENDPOINTS)), (
        "every probeable provider is BYOK-legal today; a divergence here means one of the two "
        "registries changed without the other"
    )


def test_the_intersection_is_computed_not_assumed(monkeypatch: pytest.MonkeyPatch) -> None:
    """glm r2: the shaping test must exercise the intersection, not assert an identity."""
    import k0.provider_legality as pl
    from k0.provider_legality import ProviderLegality

    oauth_only = {
        **dict(PROVIDER_LEGALITY),
        "openai": ProviderLegality(
            "openai",
            frozenset({AcquisitionMode.OAUTH_ENTITLEMENT}),
            basis="test fixture: entitlement-only",
            determination_class="provider-terms",
        ),
    }
    monkeypatch.setattr(pl, "PROVIDER_LEGALITY", oauth_only)
    assert capturable_providers() == ("anthropic",), (
        "an OAuth-only provider drops out of the capture offer — the ceremony asks only for "
        "what may legally be captured"
    )


def test_validate_key_refuses_a_byok_capture_when_it_is_not_legal(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The wiring, exercised: a provider whose legal path is entitlement-OAuth gets a refusal
    naming the legal alternative, not a key capture."""
    import k0.provider_legality as pl
    from k0.provider_legality import ProviderLegality

    # Only the DATA is swapped; the predicate under test runs for real (codex/claude r1).
    oauth_only = {
        **dict(PROVIDER_LEGALITY),
        "openai": ProviderLegality(
            "openai",
            frozenset({AcquisitionMode.OAUTH_ENTITLEMENT}),
            basis="test fixture: entitlement-only",
            determination_class="provider-terms",
        ),
    }
    monkeypatch.setattr(pl, "PROVIDER_LEGALITY", oauth_only)

    root = _root(tmp_path)
    with pytest.raises(ValueError, match="not a legal acquisition path") as exc:
        validate_key(
            root,
            MemoryStore(),
            NAME,
            provider="openai",
            estate_id=ESTATE,
            kernel_version=KERNEL,
            allowed_signers=tmp_path / "a",
            principal="p",
            scratch_dir=tmp_path,
        )
    assert "oauth_entitlement" in str(exc.value), (
        "the refusal names the legal alternative — a refusal without the next move is the "
        "dead end executive_function forbids"
    )
