"""Tests for the R2.2 deterministic pre-model segment manifest.

Form mirrors api/k0/test_k0.py: every clause gets a test that breaks it and asserts refusal.
The pin test is non-tautological: it re-executes the module source in a fresh namespace with
the import-time check neutralized and compares digests, so a coordinated data+pin edit fails.
"""

from __future__ import annotations

import re
from dataclasses import replace
from pathlib import Path
from typing import Any

import pytest

import deterministic_segment as ds
from deterministic_segment import (
    DETERMINISTIC_SEGMENT,
    EXCLUSIONS,
    R22_DRAFT_PIN,
    SEGMENT_MEMBERS,
    REQUIRED_MEMBER_IDS,
    SegmentViolation,
    verify,
    verify_minimality,
)

API_ROOT = Path(__file__).resolve().parent

#: Model-call surface markers for the segment's code paths. Heuristic first line of defense:
#: literal + dynamic imports, vendor call shapes (incl. Anthropic's messages.create), raw HTTP
#: clients to completion endpoints, and shell-outs. The runtime backstop is the chain-order
#: receipt law, not regex; the scan's surface is pinned and non-vacuous (see below).
MODEL_CLIENT_PATTERNS = (
    r"import\s+(litellm|openai|anthropic)",
    r"from\s+(litellm|openai|anthropic)[\s.a-z]*\s+import",
    r"\.chat\.completions\.create",
    r"\.messages\.create",
    r"litellm\.completion",
    r"importlib\.import_module\([^)]*(litellm|openai|anthropic|model)",
    r"__import__\([^)]*(litellm|openai|anthropic|model)",
    r"\beval\s*\(|\bexec\s*\(",
    r"/v1/(chat/)?completions",
)

#: The modules the segment's execution currently flows through. When a member builds out
#: (key capture, consent ceremony), its module JOINS this set in the same act that changes
#: the member's substrate_state — never silently. test_segment_code_paths_cover_evidence_
#: cited_modules ties this set to the membership data.
SEGMENT_CODE_PATHS = (
    "bootstrap_receipt.py",
    "reins_command.py",
    "deterministic_segment.py",
    "k0",
)

#: Per-path non-vacuity floors (review: claude PR#10 r2): a renamed or emptied path fails
#: structurally instead of passing unscanned. k0/ holds the kernel package modules.
SCAN_FILE_FLOORS = {
    "bootstrap_receipt.py": 1,
    "reins_command.py": 1,
    "deterministic_segment.py": 1,
    "k0": 6,
}


def test_canonical_segment_verifies() -> None:
    assert verify(DETERMINISTIC_SEGMENT, expect_pin=R22_DRAFT_PIN) == R22_DRAFT_PIN


#: The expected pin, anchored HERE as its own literal (review: claude PR#10 r2 blocker).
#: A coordinated data edit must now also edit this file — two files, both diff-visible.
EXPECTED_PIN = "f90a85643c46a9052b4c6adb72d2f026ea8a00bab25bb5ef009ba8adbf11a895"


def test_canonical_content_matches_pinned_shape() -> None:
    """A silent data edit must also rewrite these literals to pass."""
    seg = DETERMINISTIC_SEGMENT
    assert [m.id for m in sorted(seg.members, key=lambda m: m.id)] == [
        "durable-root-declaration",
        "first-consent",
        "first-stipulations",
        "host-reconcile",
        "identity",
        "install-verify",
        "k0-arm-genesis-lock",
        "key-capture",
    ]
    assert {m.id: m.substrate_state for m in seg.members} == {
        "install-verify": "unbuilt",
        "durable-root-declaration": "built",
        "k0-arm-genesis-lock": "built",
        "host-reconcile": "partial",
        "identity": "built",
        "key-capture": "unbuilt",
        "first-consent": "partial",
        "first-stipulations": "unbuilt",
    }
    assert seg.terminal_r_node == "R2.15"
    assert seg.ratified is False
    assert seg.mandatory_act_count is None


def test_pin_is_recomputed_independently_of_import() -> None:
    """Exec the module source with the import-time check neutralized and compare digests,
    so a coordinated data+pin edit cannot hide behind the import-time check."""
    source = (API_ROOT / "deterministic_segment.py").read_text(encoding="utf-8")
    assert "expect_pin=R22_DRAFT_PIN" in source
    neutralized = source.replace("expect_pin=R22_DRAFT_PIN", "expect_pin=None")
    namespace: dict[str, Any] = {}
    exec(compile(neutralized, "deterministic_segment.py", "exec"), namespace)
    recomputed = namespace["DETERMINISTIC_SEGMENT"].digest()
    assert recomputed == EXPECTED_PIN
    assert R22_DRAFT_PIN == EXPECTED_PIN


def test_act_set_agrees_with_k0_manifest() -> None:
    from k0.manifest import BOOTSTRAP_ACTS as K0_ACTS

    assert ds.BOOTSTRAP_ACTS == K0_ACTS


def test_phase_vocabulary_agrees_with_the_receipt_spine() -> None:
    from bootstrap_receipt import PHASE_LADDER

    ladder_values = tuple(phase.value for phase in PHASE_LADDER)
    assert ds.PHASES == ("pre-K0",) + ladder_values


def test_membership_is_exactly_the_eight_elements() -> None:
    assert {m.id for m in SEGMENT_MEMBERS} == set(REQUIRED_MEMBER_IDS)


def test_a_missing_member_refuses() -> None:
    broken = replace(
        DETERMINISTIC_SEGMENT,
        members=tuple(m for m in SEGMENT_MEMBERS if m.id != "identity"),
    )
    with pytest.raises(SegmentViolation, match="missing"):
        verify(broken)


def test_a_surplus_member_refuses() -> None:
    surplus = replace(SEGMENT_MEMBERS[0], id="convenience-member")
    with pytest.raises(SegmentViolation, match="surplus"):
        verify(replace(DETERMINISTIC_SEGMENT, members=SEGMENT_MEMBERS + (surplus,)))


def test_a_duplicate_member_refuses() -> None:
    broken = replace(
        DETERMINISTIC_SEGMENT, members=SEGMENT_MEMBERS + (SEGMENT_MEMBERS[0],)
    )
    with pytest.raises(SegmentViolation, match="duplicate"):
        verify(broken)


def test_a_non_ladder_phase_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[1], phase="pre-K0+HOST_RECONCILE")
    with pytest.raises(SegmentViolation, match="pinned ladder vocabulary"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_an_unknown_substrate_state_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[0], substrate_state="mostly")
    with pytest.raises(SegmentViolation, match="unknown substrate state"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_on_branch_evidence_cannot_license_partial() -> None:
    """On-branch work is not substrate: it may only accompany 'unbuilt'."""
    member = replace(
        SEGMENT_MEMBERS[7], substrate_state="partial", evidence_class="on_branch"
    )
    with pytest.raises(SegmentViolation, match="cannot license"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_vague_evidence_class_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[1], evidence_class="pinky_swear")
    with pytest.raises(SegmentViolation, match="unknown evidence class"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_absent_evidence_cannot_license_built() -> None:
    member = replace(
        SEGMENT_MEMBERS[0], substrate_state="built", evidence_class="absent"
    )
    with pytest.raises(SegmentViolation, match="cannot license"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_evidence_free_state_claim_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[1], evidence="")
    with pytest.raises(SegmentViolation, match="false green"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_empty_acts_only_for_self_attesting_members() -> None:
    member = replace(SEGMENT_MEMBERS[1], acts=())
    with pytest.raises(SegmentViolation, match="self-attesting"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_a_ungoverned_act_refuses() -> None:
    member = replace(SEGMENT_MEMBERS[1], acts=("vibe",))
    with pytest.raises(SegmentViolation, match="not a governed act"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(member,)))


def test_dishonest_act_tally_refuses() -> None:
    with pytest.raises(SegmentViolation, match="contradicts the data"):
        verify(replace(DETERMINISTIC_SEGMENT, mandatory_act_count=2))


def test_tally_is_pending_ratification_today() -> None:
    """The mandatory-act tally is an open ratification question (spec §8.3); freezing a
    guessed number into the pin would be a false precision."""
    assert DETERMINISTIC_SEGMENT.mandatory_act_count is None


def test_ratified_flag_is_false_and_pinned() -> None:
    assert DETERMINISTIC_SEGMENT.ratified is False
    assert "ratified" in DETERMINISTIC_SEGMENT.canonical()


def test_terminal_act_is_outside_the_segment() -> None:
    assert DETERMINISTIC_SEGMENT.terminal_act_id == "R2.15-crow-cold-start"
    member_nodes = {node for m in SEGMENT_MEMBERS for node in m.r_nodes}
    assert "R2.15" not in member_nodes


def test_missing_terminal_act_refuses() -> None:
    with pytest.raises(SegmentViolation, match="terminal act"):
        verify(replace(DETERMINISTIC_SEGMENT, terminal_act_id=""))


def test_missing_transmit_law_refuses() -> None:
    with pytest.raises(SegmentViolation, match="transmit-class law"):
        verify(replace(DETERMINISTIC_SEGMENT, transmit_class_law=()))


def test_transmit_law_order_is_semantic_and_preserved() -> None:
    assert DETERMINISTIC_SEGMENT.transmit_class_law == (
        "local_only>=K0_ACTIVE",
        "transmitting>=AUTH_MATERIALIZE",
    )
    assert (
        '"transmit_class_law":["local_only>=K0_ACTIVE","transmitting>=AUTH_MATERIALIZE"]'
        in (DETERMINISTIC_SEGMENT.canonical())
    )


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


def test_no_model_client_in_segment_code_paths() -> None:
    """The segment's execution paths must not import or call a model client."""
    offenders: list[str] = []
    for rel in SEGMENT_CODE_PATHS:
        path = API_ROOT / rel
        assert path.exists(), (
            f"scan path vanished: {rel} — the scan would pass vacuously"
        )
        files = sorted(path.rglob("*.py")) if path.is_dir() else [path]
        assert len(files) >= SCAN_FILE_FLOORS[rel], (
            f"scan path {rel} yielded {len(files)} files (< {SCAN_FILE_FLOORS[rel]})"
        )
        for file in files:
            text = file.read_text(encoding="utf-8")
            for pattern in MODEL_CLIENT_PATTERNS:
                if re.search(pattern, text):
                    offenders.append(f"{file.name}: {pattern}")
            # A deterministic segment has no need of a general HTTP client at all
            # (review: claude PR#10 r2 — cross-line patterns cannot see this).
            if re.search(
                r"^\s*(import|from)\s+(httpx|aiohttp|requests|urllib)\b", text, re.M
            ):
                offenders.append(
                    f"{file.name}: general HTTP client import in a segment path"
                )
    assert not offenders, f"model client surface in segment paths: {offenders}"


def test_segment_code_paths_cover_evidence_cited_modules() -> None:
    """The scan's path set must cover every module a landed/receipted member cites."""
    covered = SEGMENT_CODE_PATHS + ("k0/",)
    for member in SEGMENT_MEMBERS:
        if member.evidence_class not in ("landed", "receipted"):
            continue
        for cited in re.findall(r"api/([A-Za-z0-9_./]+\.py)", member.evidence):
            assert any(cited == rel or cited.startswith(rel) for rel in covered), (
                f"{member.id} cites api/{cited} but SEGMENT_CODE_PATHS does not cover it"
            )


def test_substrate_accounting_is_honest_today() -> None:
    """The declaration must not claim the segment is more built than main is."""
    states = {m.id: m.substrate_state for m in SEGMENT_MEMBERS}
    assert states["key-capture"] == "unbuilt"
    assert states["install-verify"] == "unbuilt"
    assert (
        states["first-stipulations"] == "unbuilt"
    )  # R2.8/R2.6 are on-branch, not substrate
    assert states["first-consent"] == "partial"
    assert states["host-reconcile"] == "partial"
    assert states["identity"] == "built"
    assert sum(1 for s in states.values() if s == "unbuilt") == 3
    assert sum(1 for s in states.values() if s == "partial") == 2
    assert sum(1 for s in states.values() if s == "built") == 3


def test_terminal_r_node_shape_and_membership() -> None:
    """The half-open check uses a pinned r-node field and can never go vacuous."""
    assert DETERMINISTIC_SEGMENT.terminal_r_node == "R2.15"
    broken_shape = replace(DETERMINISTIC_SEGMENT, terminal_r_node="crow-cold-start")
    with pytest.raises(SegmentViolation, match="vacuous"):
        verify(broken_shape)
    inside = replace(DETERMINISTIC_SEGMENT, terminal_r_node="R0.3")
    with pytest.raises(SegmentViolation, match="half-open"):
        verify(inside)


def test_tally_floor_counts_act_instances() -> None:
    instances = sum(len(m.acts) for m in SEGMENT_MEMBERS)
    assert instances == 11
    with pytest.raises(SegmentViolation, match="act instances"):
        verify(replace(DETERMINISTIC_SEGMENT, mandatory_act_count=10))
    assert verify(replace(DETERMINISTIC_SEGMENT, mandatory_act_count=11))


def test_verify_composes_minimality() -> None:
    """verify() alone must check the exclusion ledger (review: claude PR#10 r2)."""
    with pytest.raises(SegmentViolation, match="exclusion ledger is empty"):
        verify(replace(DETERMINISTIC_SEGMENT, exclusions=()))
    broken = replace(EXCLUSIONS[0], installed_by="")
    with pytest.raises(SegmentViolation, match="minimality violation"):
        verify(replace(DETERMINISTIC_SEGMENT, exclusions=(broken,) + EXCLUSIONS[1:]))


def test_member_and_exclusion_ids_are_disjoint() -> None:
    overlap = replace(EXCLUSIONS[0], id="identity")
    with pytest.raises(SegmentViolation, match="both member and exclusion"):
        verify(replace(DETERMINISTIC_SEGMENT, exclusions=(overlap,) + EXCLUSIONS[1:]))


def test_evidence_legality_matrix_is_complete() -> None:
    assert set(ds.EVIDENCE_LEGALITY) == set(ds.SUBSTRATE_STATES)


def test_phase_act_product_is_checked() -> None:
    pre_k0_governed = replace(SEGMENT_MEMBERS[1], phase="pre-K0")
    with pytest.raises(SegmentViolation, match="cannot execute pre-K0"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(pre_k0_governed,)))
    install_at_ladder = replace(SEGMENT_MEMBERS[0], phase="K0_ACTIVE")
    with pytest.raises(SegmentViolation, match="cannot execute at K0_ACTIVE"):
        verify(replace(DETERMINISTIC_SEGMENT, members=(install_at_ladder,)))
