"""R3.6 — the boot profile as a detected-then-ratified stipulation, tested at the properties."""

from __future__ import annotations

import subprocess
from dataclasses import replace
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from bootstrap_receipt import append_receipt, genesis_self_attest
from k0.boot_profile import (
    HARNESS_MARKERS,
    KEY_MARKERS,
    PROFILES,
    BootProfile,
    detect,
    present,
    ratified_profile,
)
from k0.ratification import RatificationError, pending, ratify

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


HARNESS = PROFILES["existing-agent-harness"]
HOSTED = PROFILES["hosted-model-kit-minimal"]


def test_detection_sees_an_installed_harness_without_executing_it() -> None:
    found = detect(which=lambda name: f"/usr/bin/{name}" if name == "claude" else None, environ={})
    assert len(found) == 1
    assert found[0].profile_id == "existing-agent-harness"
    assert found[0].evidence == ("cli:claude on PATH",)


def test_detection_sees_provider_keys_by_name_only() -> None:
    found = detect(which=lambda name: None, environ={"OPENAI_API_KEY": "sk-secret-value"})
    assert len(found) == 1
    assert found[0].profile_id == "hosted-model-kit-minimal"
    assert found[0].evidence == ("env:OPENAI_API_KEY present (value unread)",)
    assert "sk-secret-value" not in str(found), (
        "the evidence carries marker NAMES; a credential value in evidence is a leak"
    )


def test_detection_reports_both_when_both_exist_and_nothing_when_nothing_does() -> None:
    both = detect(
        which=lambda name: "/usr/bin/codex" if name == "codex" else None,
        environ={"ANTHROPIC_API_KEY": "x"},
    )
    assert [d.profile_id for d in both] == ["existing-agent-harness", "hosted-model-kit-minimal"]
    assert detect(which=lambda name: None, environ={}) == (), (
        "no markers is the honest dark answer — an assumed profile here is the defect R3.6 removes"
    )


def test_every_marker_name_maps_to_a_profile_that_exists() -> None:
    """The marker tables and the profile registry cannot drift apart silently."""
    which = lambda name: f"/usr/bin/{name}"  # noqa: E731 — every marker "present"
    environ = {name: "x" for name in KEY_MARKERS}
    found = detect(which=which, environ=environ)
    assert {d.profile_id for d in found} == set(PROFILES), (
        "every detectable path must land on a sanctioned profile"
    )
    assert HARNESS_MARKERS and KEY_MARKERS, "marker tables must not empty by accident"


def test_the_stipulation_body_is_deterministic_and_digest_stable() -> None:
    again = BootProfile(
        profile_id=HARNESS.profile_id,
        shape=HARNESS.shape,
        actions=frozenset(HARNESS.actions),
        authority_ceiling=HARNESS.authority_ceiling,
        fallback_policy=HARNESS.fallback_policy,
        freshness=HARNESS.freshness,
        tradeoffs=tuple(HARNESS.tradeoffs),
    )
    assert again.body() == HARNESS.body(), "canonical JSON must be byte-stable across constructions"
    assert again.stipulation_id() == HARNESS.stipulation_id()
    HARNESS.stipulation()  # must not raise: the id satisfies the ref grammar
    assert " " not in HARNESS.stipulation_id()


def test_present_then_ratify_then_read_back(tmp_path: Path) -> None:
    """The whole point: the floor the estate stands on is a consented artifact."""
    root = _root(tmp_path)
    key = _key(tmp_path)

    assert ratified_profile(root) is None, "no default: an unconsented floor reads as absent"

    present(root, HARNESS, estate_id=ESTATE, kernel_version=KERNEL)
    assert HARNESS.stipulation_id() in pending(root)

    ratify(root, HARNESS.stipulation(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert pending(root) == ()

    got = ratified_profile(root)
    assert got is not None and got.profile_id == "existing-agent-harness"
    assert got.current, "the ratified bytes ARE the current definition — this must say so"


def test_presenting_twice_is_the_ceremonys_refusal_not_ours(tmp_path: Path) -> None:
    root = _root(tmp_path)
    present(root, HARNESS, estate_id=ESTATE, kernel_version=KERNEL)
    with pytest.raises(RatificationError):
        present(root, HARNESS, estate_id=ESTATE, kernel_version=KERNEL)


def test_a_redefined_profile_supersedes_and_the_stale_consent_is_reported(tmp_path: Path) -> None:
    """A changed definition is a NEW stipulation; the old consent is never stretched to cover it.

    Ratify the profile, then a redefinition (same profile_id, different terms — a new id, so
    consent-once holds). The latest row is the floor in effect, and because the redefinition here
    is NOT the registry's current definition, `current` must be False: the operator consented to
    terms the code no longer mints, and that is reported, not repaired.
    """
    root = _root(tmp_path)
    key = _key(tmp_path)
    ratify_path = present(root, HARNESS, estate_id=ESTATE, kernel_version=KERNEL)
    assert ratify_path is not None
    ratify(root, HARNESS.stipulation(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)
    assert ratified_profile(root).current

    redefined = replace(HARNESS, tradeoffs=HARNESS.tradeoffs + ("a new term",))
    assert redefined.stipulation_id() != HARNESS.stipulation_id(), (
        "fixture premise: new terms must mint a new stipulation id"
    )
    present(root, redefined, estate_id=ESTATE, kernel_version=KERNEL)
    ratify(root, redefined.stipulation(), key_path=key, estate_id=ESTATE, kernel_version=KERNEL)

    got = ratified_profile(root)
    assert got.profile_id == "existing-agent-harness"
    assert not got.current, (
        "the ratified bytes are not what the current definition mints — say so, never repair it"
    )
