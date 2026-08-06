"""Tests for the R2.2 deterministic pre-model segment manifest.

Form mirrors api/k0/test_k0.py: every clause gets a test that breaks it and asserts refusal.
The two evidence-bearing checks are the no-LLM source scan over segment code paths and the
act-set agreement with the ratified K0 manifest.
"""

from __future__ import annotations

import re
from dataclasses import replace
from pathlib import Path

import pytest

import deterministic_segment as ds
from deterministic_segment import (
    DETERMINISTIC_SEGMENT,
    EXCLUSIONS,
    SEGMENT_MEMBERS,
    R22_DRAFT_PIN,
    SegmentViolation,
    verify,
    verify_minimality,
)

API_ROOT = Path(__file__).resolve().parent

#: Model-client markers that must never appear in segment code paths. Kept deliberately
#: conservative: these match imports/calls, never prose in comments about models.
MODEL_CLIENT_PATTERNS = (
    r"import\s+(litellm|openai|anthropic|requests\.post\(.*(api|completions))",
    r"from\s+(litellm|openai|anthropic)\s+import",
    r"\.chat\.completions\.create",
    r"litellm\.completion",
)

#: The modules the segment's execution currently flows through. When a member builds out
#: (key capture, consent ceremony), its module JOINS this set in the same ratification act
#: that changes the member's substrate_state — never silently.
SEGMENT_CODE_PATHS = (
    "bootstrap_receipt.py",
    "reins_command.py",
    "k0",
)


def test_canonical_segment_verifies() -> None:
    assert verify(DETERMINISTIC_SEGMENT, expect_pin=R22_DRAFT_PIN) == R22_DRAFT_PIN


def test_act_set_agrees_with_k0_manifest() -> None:
    from k0.manifest import BOOTSTRAP_ACTS as K0_ACTS

    assert ds.BOOTSTRAP_ACTS == K0_ACTS


def test_membership_is_exactly_the_eight_elements() -> None:
    assert {m.id for m in SEGMENT_MEMBERS} == {
        "install-verify",
        "durable-root-declaration",
        "k0-arm-genesis-lock",
        "host-reconcile",
        "identity",
        "key-capture",
        "first-consent",
        "first-stipulations",
    }


def test_a_missing_member_refuses() -> None:
    broken = replace(
        DETERMINISTIC_SEGMENT,
        members=tuple(m for m in SEGMENT_MEMBERS if m.id != "identity"),
    )
    with pytest.raises(SegmentViolation, match="closed-world"):
        verify(broken)


def test_a_duplicate_member_refuses() -> None:
    broken = replace(
        DETERMINISTIC_SEGMENT, members=SEGMENT_MEMBERS + (SEGMENT_MEMBERS[0],)
    )
    with pytest.raises(SegmentViolation, match="duplicate"):
        verify(broken)


def test_an_unknown_substrate_state_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[0], substrate_state="mostly")
    with pytest.raises(SegmentViolation, match="unknown substrate state"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_built_with_absent_evidence_refuses() -> None:
    """A false green must not verify: 'built' cannot carry an absent: evidence class."""
    member = replace(
        SEGMENT_MEMBERS[0], substrate_state="built", evidence="absent: nothing exists"
    )
    with pytest.raises(SegmentViolation, match="declared built with absent evidence"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_evidence_free_state_claim_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[1], evidence="")
    with pytest.raises(SegmentViolation, match="false green"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_a_ungoverned_act_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[1], acts=("vibe",))
    with pytest.raises(SegmentViolation, match="not a governed act"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_dishonest_act_tally_refuses() -> None:
    broken = replace(DETERMINISTIC_SEGMENT, mandatory_act_count=3)
    with pytest.raises(SegmentViolation, match="tally is dishonest"):
        verify(broken)


def test_missing_terminal_act_refuses() -> None:
    broken = replace(DETERMINISTIC_SEGMENT, terminal_act_id="")
    with pytest.raises(SegmentViolation, match="terminal act"):
        verify(broken)


def test_missing_transmit_law_refuses() -> None:
    broken = replace(DETERMINISTIC_SEGMENT, transmit_class_law=())
    with pytest.raises(SegmentViolation, match="transmit-class law"):
        verify(broken)


def test_drift_refuses() -> None:
    with pytest.raises(SegmentViolation, match="drift pin mismatch"):
        verify(DETERMINISTIC_SEGMENT, expect_pin="0" * 64)


def test_minimality_exclusion_without_installer_refuses() -> None:
    broken = replace(EXCLUSIONS[0], installed_by="")
    with pytest.raises(SegmentViolation, match="minimality violation"):
        verify_minimality(replace(DETERMINISTIC_SEGMENT, exclusions=(broken,)))


def test_exclusion_ledger_names_the_near_misses() -> None:
    assert {e.id for e in EXCLUSIONS} >= {
        "crow-narration",
        "measured-probe",
        "enforce-flip",
        "kit-signature-verification",
    }


def test_terminal_act_is_the_crow_cold_start() -> None:
    assert DETERMINISTIC_SEGMENT.terminal_act_id == "R2.15-crow-cold-start"


def test_no_model_client_in_segment_code_paths() -> None:
    """The segment's execution paths must not import or call a model client."""
    offenders: list[str] = []
    for rel in SEGMENT_CODE_PATHS:
        path = API_ROOT / rel
        files = sorted(path.rglob("*.py")) if path.is_dir() else [path]
        for file in files:
            text = file.read_text(encoding="utf-8")
            for pattern in MODEL_CLIENT_PATTERNS:
                if re.search(pattern, text):
                    offenders.append(f"{file.name}: {pattern}")
    assert not offenders, f"model client surface in segment paths: {offenders}"


def test_substrate_accounting_is_honest_today() -> None:
    """The declaration must not claim the segment is more built than it is."""
    states = {m.id: m.substrate_state for m in SEGMENT_MEMBERS}
    assert states["key-capture"] == "unbuilt"
    assert states["install-verify"] == "unbuilt"
    assert states["first-consent"] == "partial"
    assert states["first-stipulations"] == "partial"
    assert states["identity"] == "built"
