"""The two normative-constraint members. Each test breaks the law and expects refusal."""

from __future__ import annotations

import pytest

from conftest import UNARMED, estate_tokens, scan_tree_for_tokens

from k0 import (
    DeadEndRefusalError,
    Evaluation,
    Refusal,
    RefusalError,
    decide,
    evaluate_optional,
)


# --- W1: refusal-as-data ------------------------------------------------------------------
def test_a_refusal_carries_why_and_legal_next():
    r = Refusal(gate="durable-root", why="tmpfs is volatile", legal_next="declare a durable root")
    assert "tmpfs is volatile" in r.render()
    assert "legal next" in r.render()


def test_refusal_without_a_legal_next_is_a_dead_end_and_is_refused():
    """INV-3 applied to the kernel: BLOCKED always escapes. A trap is not a gate."""
    with pytest.raises(DeadEndRefusalError, match="no legal next move"):
        Refusal(gate="g", why="because", legal_next="   ")


def test_refusal_without_a_reason_is_refused():
    with pytest.raises(ValueError, match="no reason"):
        Refusal(gate="g", why="", legal_next="do the thing")


def test_refusal_must_name_its_gate():
    with pytest.raises(ValueError, match="name the gate"):
        Refusal(gate="  ", why="w", legal_next="n")


def test_receipt_fields_project_act_refused_and_omit_the_refused_value():
    r = Refusal(gate="air", why="field denied", legal_next="request an allowlist entry")
    f = r.receipt_fields()
    assert f["act"] == "refused"
    assert f["gate"] == "air" and f["why"] and f["legal_next"]
    assert "value" not in f  # a refusal never carries the thing it refused


def test_teaches_is_optional_but_projected_when_present():
    assert "teaches" not in Refusal(gate="g", why="w", legal_next="n").receipt_fields()
    r = Refusal(gate="g", why="w", legal_next="n", teaches="doctrine/fail-closed")
    assert r.receipt_fields()["teaches"] == "doctrine/fail-closed"
    assert "doctrine/fail-closed" in r.render()


# --- W2: fail-closed-default --------------------------------------------------------------
def test_satisfied_admits():
    assert decide("g", Evaluation.SATISFIED, legal_next="n") is None


def test_violated_refuses():
    with pytest.raises(RefusalError) as e:
        decide("g", Evaluation.VIOLATED, legal_next="fix it", violated_why="predicate false")
    assert e.value.refusal.why == "predicate false"


def test_UNEVALUABLE_REFUSES_this_is_the_whole_law():
    """The arm systems forget. declare_durable_root forgot it and accepted /dev/shm."""
    with pytest.raises(RefusalError) as e:
        decide("durable-root", Evaluation.UNEVALUABLE, legal_next="make /proc/mounts readable")
    assert "could not be evaluated" in e.value.refusal.why
    assert e.value.refusal.legal_next


def test_the_two_why_strings_are_not_interchangeable():
    """'we checked and it failed' and 'we could not check' are different facts."""
    with pytest.raises(RefusalError) as v:
        decide("g", Evaluation.VIOLATED, legal_next="n", violated_why="V", unevaluable_why="U")
    with pytest.raises(RefusalError) as u:
        decide("g", Evaluation.UNEVALUABLE, legal_next="n", violated_why="V", unevaluable_why="U")
    assert v.value.refusal.why == "V"
    assert u.value.refusal.why == "U"


def test_a_refusing_arm_with_no_legal_next_cannot_be_constructed():
    """The law cannot be used to build a trap."""
    with pytest.raises(DeadEndRefusalError):
        decide("g", Evaluation.UNEVALUABLE, legal_next="")


def test_none_is_unevaluable_not_false():
    """The trap: `if value:` collapses None into False, turning 'could not observe' into
    'observed a negative' — or worse, into a pass."""
    assert evaluate_optional(None) is Evaluation.UNEVALUABLE
    assert evaluate_optional(False) is Evaluation.VIOLATED
    assert evaluate_optional(True) is Evaluation.SATISFIED
    assert evaluate_optional("") is Evaluation.VIOLATED
    assert evaluate_optional("x") is Evaluation.SATISFIED


def test_the_kernel_package_is_estate_independent():
    """K0 must carry no estate. If this fails, the extraction has leaked.

    THE TOKENS ARE NOT WRITTEN HERE, AND THE SCAN NO LONGER SKIPS ITS OWN FILE.

    Both of those are the same fix for the same defect. The previous version of this test held the
    denylist inline — the estate's home path, both host nicknames, and the operator's referent —
    and then excluded itself from the scan (`if p.name != "test_k0.py"`). So the guard against
    publishing estate fingerprints WAS a published estate fingerprint, in the one file it could
    not see. It was live on the public repository and could never have caught itself.

    A denylist names what it forbids. That makes an inline denylist unexportable by construction,
    which is precisely R0.10's split: the guard is law and ships; what it matches is estate data
    and is supplied from outside the tree (see api/conftest.py). With the tokens external, the
    scan can now cover every file in the package including this one.
    """
    import pathlib

    tokens = estate_tokens()
    if tokens is None:
        pytest.skip(UNARMED)

    hits = scan_tree_for_tokens(pathlib.Path(__file__).parent, tokens)
    assert not hits, (
        f"estate fingerprints in exportable kernel files: {sorted({p.name for p, _ in hits})}. "
        f"K0 ships to strangers; it must carry none of this estate. The tokens are not quoted "
        f"here — a failure report travels further than the files it describes."
    )


def test_the_kernel_package_is_uncoupled_from_the_substrate_it_was_extracted_from():
    """A SEPARATE CHECK FROM THE ONE ABOVE, on purpose.

    `council_root` is a config key and `hapax-council` is a public repository name. Neither
    discloses anything, so neither belongs in the estate-fingerprint list — a denylist padded with
    non-secrets produces findings nobody reads, and a guard nobody reads does not work. But K0 must
    still be free of them, for a different reason: the kernel was extracted FROM that substrate and
    must not have carried its vocabulary out. That is a coupling defect, not a disclosure one, and
    the two have different remedies (rename vs. redact), so they are asserted apart and named apart.

    These literals are safe to write inline precisely because they are not secrets.
    """
    import pathlib

    for path in sorted(pathlib.Path(__file__).parent.glob("*.py")):
        src = path.read_text()
        if path.name == "test_k0.py":
            src = ""  # this docstring names them by necessity; see above for why that is sound
        for token in ("council_root", "hapax-council"):
            assert token not in src, (
                f"{path.name} references {token!r}: the kernel still carries the vocabulary of the "
                f"substrate it was extracted from. Generalize the name."
            )


# --- the loop closes: the law reproduces the worked example --------------------------------
_VOLATILE = frozenset({"tmpfs", "ramfs", "devtmpfs", "overlay"})


def _durable_root_guard(fstype: str | None) -> None:
    """declare_durable_root's guard, rewritten against the extracted law.

    The original carried all three arms inline and got one wrong: it returned the literal
    "unknown" for the unevaluable case and then tested only membership of the volatile set, so
    the unevaluable arm fell through to admit. Expressed against `decide`, that arm cannot be
    dropped — there is nowhere for it to fall through TO.
    """
    if fstype is None:
        ev = Evaluation.UNEVALUABLE
    elif fstype in _VOLATILE:
        ev = Evaluation.VIOLATED
    else:
        ev = Evaluation.SATISFIED
    decide(
        "durable-root",
        ev,
        legal_next="declare a root on durable media whose filesystem can be observed",
        violated_why=f"root sits on volatile filesystem {fstype!r}",
        unevaluable_why="could not determine the filesystem (is /proc/mounts readable?)",
        teaches="doctrine/fail-closed-default",
    )


def test_the_law_reproduces_the_hardened_durable_root_guard():
    _durable_root_guard("ext4")  # admits

    with pytest.raises(RefusalError) as vol:
        _durable_root_guard("tmpfs")
    assert "volatile" in vol.value.refusal.why

    # the arm the original dropped
    with pytest.raises(RefusalError) as unk:
        _durable_root_guard(None)
    assert "could not determine" in unk.value.refusal.why
    assert unk.value.refusal.legal_next


def test_the_dropped_arm_is_structurally_unreachable_as_a_pass():
    """The original bug in one line: 'unknown' was not in _VOLATILE, so it passed. Against the
    law there is no such fall-through — UNEVALUABLE has its own arm and that arm refuses."""
    assert Evaluation.UNEVALUABLE not in (Evaluation.SATISFIED,)
    with pytest.raises(RefusalError):
        decide("g", Evaluation.UNEVALUABLE, legal_next="n")
