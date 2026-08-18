"""The fixed point must REJECT, not merely describe. Each test breaks one half and expects refusal.

Self-contained per the repo testing convention (no shared conftest fixtures).
"""

from __future__ import annotations

import ast
import dataclasses
import importlib.util
import sys
from pathlib import Path
from typing import Any

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

K0_ROOT = Path(__file__).resolve().parent

#: The ratified pin as an independent literal, used only by the arming test below.
#:
#: `test_the_pin_is_the_ratified_literal_not_a_recomputation` already anchors `RATIFIED_PIN` this
#: way, so this is not a new anchor and does not pretend to be one. It exists so the arming test
#: can compare a digest recomputed with the import-time check DISABLED against a value that did
#: not come through that check.
EXPECTED_RATIFIED_PIN = "b604b52bfdd9e267b7a5b68f42d020f233065f3c6d77eeb9f244de2d78ee6d59"


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


def test_the_import_time_drift_check_is_still_armed(tmp_path: Path) -> None:
    """`test_the_pin_is_the_ratified_literal_not_a_recomputation` already anchors the literal.

    What nothing asserted is that the ARMING survives: `RATIFIED_PIN` could keep its value and
    stay pinned by a test, while `K0_DRIFT_PIN = verify(K0, expect_pin=RATIFIED_PIN)` was quietly
    softened to `expect_pin=None`. Every existing test would still pass — the constant is
    unchanged and the recomputation still agrees — and the kernel would no longer be checked at
    import by anything.

    So this reads the source and requires the arming expression to be present, then re-execs with
    it neutralised to prove the digest is what the anchored literal says independently of the
    import-time path. Mirrors
    `api/test_deterministic_segment.py::test_pin_is_recomputed_independently_of_import`, which the
    module this one mirrors already had.
    """
    source = (K0_ROOT / "manifest.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    pin_assign = None
    for node in tree.body:
        if not isinstance(node, ast.Assign):
            continue
        if any(isinstance(t, ast.Name) and t.id == "K0_DRIFT_PIN" for t in node.targets):
            pin_assign = node
            break
    assert pin_assign is not None, (
        "K0_DRIFT_PIN assignment was removed or is no longer a module-level Assign; the kernel "
        "is no longer pinned at import and this test can no longer prove anything about it"
    )
    call = pin_assign.value
    assert isinstance(call, ast.Call), "K0_DRIFT_PIN must be bound to a verify(...) call"
    expect = next((kw.value for kw in call.keywords if kw.arg == "expect_pin"), None)
    assert isinstance(expect, ast.Name) and expect.id == "RATIFIED_PIN", (
        "the import-time drift check no longer passes expect_pin=RATIFIED_PIN; a comment or "
        "other call matching the old token would have hidden this"
    )

    # A REAL MODULE LOAD, not a differently-spelled dynamic evaluation.
    #
    # The original called the dynamic-evaluation builtin, which tripped this repo's own
    # model-client scanner in api/test_deterministic_segment.py, so the test failed CI while
    # testing nothing about model clients.
    #
    # The tempting fix was to switch to the sibling builtin: same act, different token, scanner
    # silenced. That is evasion, and blunting a live guard to accommodate a test is the failure
    # mode this module exists to prevent. The scanner was doing its job.
    #
    # So the neutralised copy is written out and loaded through the import machinery instead. It
    # is a closer model of the real thing besides — the behaviour under test IS an import-time
    # side effect. (This comment deliberately does not quote the scanner's pattern: the first
    # version did, and the comment then matched it, which is the same defect one layer up.)
    #
    # Neutralise only the K0_DRIFT_PIN assignment's expect_pin, not every occurrence of the
    # token — a later comment or unrelated call must not keep this test green.
    neutralized = source
    for kw in call.keywords:
        if kw.arg == "expect_pin":
            lines = source.splitlines(keepends=True)
            start = sum(len(line) for line in lines[: kw.value.lineno - 1]) + kw.value.col_offset
            end = (
                sum(len(line) for line in lines[: kw.value.end_lineno - 1])
                + kw.value.end_col_offset
            )
            neutralized = source[:start] + "None" + source[end:]
            break
    neutralized_path = tmp_path / "manifest_neutralized.py"
    neutralized_path.write_text(neutralized, encoding="utf-8")

    spec = importlib.util.spec_from_file_location("k0_manifest_neutralized", neutralized_path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    # Registered before loading: the module defines dataclasses, and the decorator resolves the
    # defining module out of sys.modules while the class body runs.
    sys.modules[spec.name] = module
    try:
        spec.loader.exec_module(module)
    finally:
        sys.modules.pop(spec.name, None)

    assert module.K0.drift_pin() == EXPECTED_RATIFIED_PIN
    assert module.RATIFIED_PIN == EXPECTED_RATIFIED_PIN


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


# --- the second closed-world source: the superseded draft ------------------------------------
#: Every member of the 11-member vault draft (k0-manifest.yaml, 0.1.0-draft), under the name this
#: file classifies it by. Frozen here because the draft is a superseded VAULT artifact: once it is
#: retired there is no other record that these were kernel candidates, and an unrecorded candidate
#: is precisely what a closed-world check exists to make impossible.
DRAFT_MEMBER_DISPOSITIONS = {
    "K0.1 fail-closed default": "fail-closed-default",
    "K0.2 refusal-as-data": "refusal-as-data",
    "K0.3 receipt primitive": "receipt-primitive",
    "K0.4 ratification act": "ratification-act",
    "K0.5 sovereign-only mutation": "sovereign-only-mutation",
    "K0.6 signed EscapeGrant verify": "escape-grant-verify",
    "K0.7 PII/consent scanner": "pii-consent-scanner",
    "K0.8 identity seed": "identity-seed",
    "K0.8 ratifier signing key": "ratifier-signing-key",
    "K0.9 FSM phase-legality law": "fsm-phase-legality-law",
    "K0.10 single-instance lock": "single-instance-lock",
    "K0.11 bounded-affordance law": "bounded-affordance-law",
}


def test_every_draft_candidate_is_classified_exactly_once():
    """The check that was missing on 2026-08-10, when two draft members were in NEITHER list.

    verify_minimality could not catch it: an unclassified candidate is invisible to a world
    closed against R0.1-R0.12 alone. K0.5 and K0.6 sat outside both lists with real circularity
    witnesses and byte-pinned artifacts, so the minimality claim held only over a world that
    omitted them.

    Note K0.8: the draft bundled the identity seed and the ratifier key into ONE member, and this
    kernel splits them -- seed is kernel, key is minted. A test keyed to draft members has to
    represent that split, or it re-hides the sharper answer.
    """
    from k0.manifest import EXCLUDED
    member_ids = {m.id for m in K0.members}

    unclassified = [
        draft_name
        for draft_name, name in DRAFT_MEMBER_DISPOSITIONS.items()
        if name not in member_ids and name not in EXCLUDED
    ]
    assert not unclassified, (
        f"draft candidates in neither K0 nor EXCLUDED: {unclassified}. A non-installable "
        f"candidate belongs in K0; an installable one belongs in EXCLUDED with its installer "
        f"named. Silence is the third option, and it is the one that is wrong."
    )


def test_the_harvested_candidates_record_their_provenance():
    """Regression pin for the harvest itself, not merely for its presence.

    An exclusion whose reason omits a bootstrap act already fails verify_minimality. These three
    are the entries a later editor is most likely to prune as redundant, because they are the
    only ones not traceable to an R0.x id. K0.11 is the contestable one; dropping its draft
    identifier would hide that it was ever a kernel candidate.
    """
    from k0.manifest import EXCLUDED
    expected_draft_members = {
        "sovereign-only-mutation": "K0.5",
        "escape-grant-verify": "K0.6",
        "bounded-affordance-law": "K0.11",
    }
    for name, draft_member in expected_draft_members.items():
        assert name in EXCLUDED, f"{name} was harvested from the draft; do not drop it"
        assert f"draft {draft_member}" in EXCLUDED[name], (
            f"{name} must record which draft member it was"
        )
