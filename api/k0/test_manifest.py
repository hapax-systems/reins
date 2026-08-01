"""The fixed point must REJECT, not merely describe. Each test breaks one half and expects refusal.

Self-contained per the repo testing convention (no shared conftest fixtures).
"""

from __future__ import annotations

import dataclasses

import pytest

from k0.manifest import (
    BOOTSTRAP_ACTS,
    K0,
    K0_DRIFT_PIN,
    KernelManifest,
    KernelManifestError,
    KernelMember,
    verify,
)


def _member(**over) -> KernelMember:
    base = dict(
        id="probe-member",
        lever_class="normative-constraint",
        tier="core",
        presupposed_by=BOOTSTRAP_ACTS,
        circularity_witness="installing it presupposes it",
    )
    base.update(over)
    return KernelMember(**base)


def _manifest(members) -> KernelManifest:
    return dataclasses.replace(K0, members=tuple(members))


def test_the_canonical_kernel_verifies():
    assert verify(K0) == K0_DRIFT_PIN


def test_pin_is_stable_across_member_order():
    """Declaration order must not move the pin, or every reshuffle reads as kernel drift."""
    reversed_k0 = _manifest(tuple(reversed(K0.members)))
    assert reversed_k0.drift_pin() == K0.drift_pin()


def test_drift_is_detected():
    mutated = _manifest(K0.members + (_member(id="smuggled"),))
    assert mutated.drift_pin() != K0_DRIFT_PIN
    with pytest.raises(KernelManifestError, match="DRIFT"):
        verify(mutated, expect_pin=K0_DRIFT_PIN)


# --- half (a): presupposed by EVERY bootstrap act ------------------------------------------
def test_member_presupposed_by_only_some_acts_is_rejected():
    """'Early' is not 'kernel'. This is the half that would otherwise admit anything convenient."""
    partial = _member(id="merely-early", presupposed_by=("mint", "flip"))
    with pytest.raises(KernelManifestError, match="not presupposed by"):
        verify(_manifest([partial]))


def test_unknown_bootstrap_act_is_rejected():
    with pytest.raises(KernelManifestError, match="unknown bootstrap acts"):
        verify(_manifest([_member(presupposed_by=BOOTSTRAP_ACTS + ("teleport",))]))


# --- half (b): non-installability ---------------------------------------------------------
def test_member_without_circularity_witness_is_rejected():
    """Awkward-to-install is not the test; circular-to-install is."""
    with pytest.raises(KernelManifestError, match="no circularity witness"):
        verify(_manifest([_member(circularity_witness="   ")]))


# --- structural invariants ----------------------------------------------------------------
def test_empty_kernel_is_rejected():
    with pytest.raises(KernelManifestError, match="empty"):
        verify(_manifest([]))


def test_duplicate_member_is_rejected():
    with pytest.raises(KernelManifestError, match="duplicate"):
        verify(_manifest([_member(), _member()]))


def test_unknown_lever_class_is_rejected():
    with pytest.raises(KernelManifestError, match="unknown lever class"):
        verify(_manifest([_member(lever_class="vibes")]))


def test_kernel_without_normative_constraint_is_rejected():
    """The design claims normative-constraint is wholly contained in K0. If none appears, the
    containment claim is vacuous and the manifest is not the kernel it says it is."""
    only_state = _member(id="receipt-ish", lever_class="state-context")
    with pytest.raises(KernelManifestError, match="normative-constraint"):
        verify(_manifest([only_state]))


def test_pre_k0_fact_cannot_be_smuggled_in_as_a_member():
    """Install-time preconditions are established by the distribution and host, not the kernel."""
    smuggled = _member(id="durable-root-declared")
    with pytest.raises(KernelManifestError, match="pre-K0"):
        verify(_manifest([smuggled]))


# --- the canonical members actually satisfy the criterion ---------------------------------
@pytest.mark.parametrize("m", K0.members, ids=lambda m: m.id)
def test_every_canonical_member_meets_its_tier_test(m):
    """Non-installability is required of BOTH tiers; universal presupposition only of core."""
    assert m.circularity_witness.strip(), m.id
    if m.tier == "core":
        assert set(m.presupposed_by) == set(BOOTSTRAP_ACTS), m.id
    else:
        assert m.constitutive_of.strip(), m.id


def test_the_four_seeded_lever_classes_are_present():
    """§2: normative-constraint wholly, plus minimal seeds of state/context, elicitation and
    loop/control. If a seed class vanishes, the kernel no longer spans what boot presupposes."""
    got = {m.lever_class for m in K0.members}
    assert got == {"normative-constraint", "state-context", "elicitation", "loop-control"}, got


# --- minimality, made falsifiable ---------------------------------------------------------
def test_the_closed_world_is_consistent():
    from k0.manifest import EXCLUDED, verify_minimality
    n = verify_minimality(K0)
    assert n == len(K0.members) + len(EXCLUDED)


def test_a_mechanism_cannot_be_both_kernel_and_installable(monkeypatch):
    import k0.manifest as km
    monkeypatch.setitem(km.EXCLUDED, "ratification-act", "installed by `ratify`")
    with pytest.raises(km.KernelManifestError, match="classified twice"):
        km.verify_minimality(K0)


def test_exclusion_without_an_installer_is_a_minimality_violation(monkeypatch):
    """The whole point: non-installable + excluded means it BELONGS in K0."""
    import k0.manifest as km
    monkeypatch.setitem(km.EXCLUDED, "mystery-mechanism", "   ")
    with pytest.raises(km.KernelManifestError, match="belongs in K0|BELONGS IN K0"):
        km.verify_minimality(K0)


def test_exclusion_must_name_an_actual_bootstrap_act(monkeypatch):
    import k0.manifest as km
    monkeypatch.setitem(km.EXCLUDED, "hand-wave", "it just shows up somehow")
    with pytest.raises(km.KernelManifestError, match="does not name an installing act"):
        km.verify_minimality(K0)


# --- option C: the two tiers, and the guards that stop `seed` becoming a loophole ------------
def test_the_ratified_tiers():
    """5 core + 1 seed, ratified 2026-08-01."""
    core = [m for m in K0.members if m.tier == "core"]
    seed = [m for m in K0.members if m.tier == "seed"]
    assert len(core) == 5 and len(seed) == 1
    assert seed[0].id == "ratification-act"


def test_the_seed_member_is_honestly_not_universally_presupposed():
    """The whole reason option C exists: reconcile/probe precede STIPULATION_RATIFY."""
    seed = next(m for m in K0.members if m.tier == "seed")
    missing = set(BOOTSTRAP_ACTS) - set(seed.presupposed_by)
    assert missing == {"probe", "reconcile"}, missing
    assert seed.constitutive_of.strip()


def test_seed_without_a_constitutive_witness_is_rejected():
    """Otherwise 'seed' is where anything that failed the core test goes to hide."""
    bad = _member(id="pretender", tier="seed", presupposed_by=("mint",), constitutive_of="  ")
    with pytest.raises(KernelManifestError, match="no constitutive_of witness"):
        verify(_manifest(list(K0.members) + [bad]))


def test_a_universally_presupposed_member_cannot_hide_in_seed():
    bad = _member(id="actually-core", tier="seed", constitutive_of="claims to seed something")
    with pytest.raises(KernelManifestError, match="it is core, not seed"):
        verify(_manifest(list(K0.members) + [bad]))


def test_core_member_may_not_carry_a_constitutive_witness():
    bad = _member(id="softened", tier="core", constitutive_of="hand-wave")
    with pytest.raises(KernelManifestError, match="must not be used to soften"):
        verify(_manifest(list(K0.members) + [bad]))


def test_seed_tier_may_not_outgrow_core():
    seeds = [
        _member(id=f"s{i}", tier="seed", presupposed_by=("mint",), constitutive_of="a regime")
        for i in range(5)
    ]
    with pytest.raises(KernelManifestError, match="not smaller than core"):
        verify(_manifest(list(K0.members) + seeds))


def test_install_is_not_a_governed_act():
    """It precedes K0; quantifying the kernel over it would be incoherent."""
    from k0.manifest import PRE_K0_ACTS
    assert "install" in PRE_K0_ACTS
    assert "install" not in BOOTSTRAP_ACTS


def test_dispositions_are_not_acts():
    from k0.manifest import DISPOSITIONS
    assert not set(DISPOSITIONS) & set(BOOTSTRAP_ACTS)
    assert set(DISPOSITIONS) == {"refused", "held", "escaped"}


# --- the pin must be RATIFIED, not merely self-consistent -----------------------------------
def test_the_pin_is_the_ratified_literal_not_a_recomputation():
    """An earlier draft computed the pin from current state, so any membership edit moved it and
    every check still passed. A drift pin that recomputes itself is not a pin."""
    from k0.manifest import RATIFIED_PIN
    assert K0_DRIFT_PIN == RATIFIED_PIN
    assert RATIFIED_PIN == "b604b52bfdd9e267b7a5b68f42d020f233065f3c6d77eeb9f244de2d78ee6d59"


def test_an_unratified_manifest_will_not_verify():
    import dataclasses
    from k0.manifest import RATIFIED_PIN
    for change in ({"kernel_version": "9.9.9"}, {"pre_k0": ("something-else",)}):
        with pytest.raises(KernelManifestError, match="DRIFT"):
            verify(dataclasses.replace(K0, **change), expect_pin=RATIFIED_PIN)


def test_the_kernel_can_attest_itself():
    """Before this the kernel had to be TOLD its own manifest hash, so it could be made to attest
    a manifest that was not its own."""
    from k0.manifest import kernel_identity
    ident = kernel_identity()
    assert ident["kernel_manifest_sha256"] == K0_DRIFT_PIN
    assert ident["kernel_version"] == K0.kernel_version
