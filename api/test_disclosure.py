"""R0.10 — the portable disclosure guard, tested against the incident that motivated it."""

from __future__ import annotations

import pytest

from conftest import UNARMED, estate_tokens

from disclosure import (
    CEILING,
    Disclosure,
    DisclosureError,
    PatternSet,
    Sensitivity,
    assert_transmittable,
    scan,
    width,
)

#: ESTATE DATA, supplied by the caller — the kernel ships none. These are invented for the test.
FULL = PatternSet(
    patterns={
        Sensitivity.OPERATOR_PII: (r"\bADHD\b", r"\bautis(m|tic)\b", r"\bmortality\b"),
        Sensitivity.HOST_FINGERPRINT: (r"\bexample-host-\w+\b",),
        Sensitivity.TRANSCRIPT: (r"^\s*(operator|assistant):", ),
        Sensitivity.CREDENTIAL: (r"\bsk-[A-Za-z0-9]{8,}\b",),
    }
)

BENIGN = "A document about scheduling and build systems.\n"
PERSONAL = "Operator telos: family, ADHD and autism, mortality planning.\n"
#: A DECOY, not a secret. It must have credential SHAPE or it cannot exercise the scanner, which
#: means the estate's own gitleaks job correctly flags it (generic-api-key, entropy 3.91) — a true
#: positive on a fixture whose whole job is to look like the thing being detected. The inline
#: annotation below is gitleaks' declared mechanism for exactly this case: it says "deliberate
#: decoy" in a form both a reader and the scanner's audit can see. Suppressing the rule repo-wide,
#: or assembling the string at runtime to slip past the scan, would each have hidden the decoy
#: instead of declaring it — and a guard you route around is a guard you no longer have.
CREDENTIAL = "token: sk-NOTAREALKEY-testfixture-0000\n"  # gitleaks:allow


def test_the_incident_is_refused(tmp_path) -> None:
    """THE CASE THIS EXISTS FOR.

    On 2026-08-04 a document containing operator health and mortality content was pushed to a
    PUBLIC repository. Every file write was legitimate; the destination was not. Nothing in the
    estate objected, because nothing modelled the destination.
    """
    with pytest.raises(DisclosureError) as exc:
        assert_transmittable(
            PERSONAL, patterns=FULL, destination=Disclosure.PUBLIC, destination_name="a public repo"
        )
    r = exc.value.refusal
    assert "operator_pii" in r.why
    assert r.legal_next, "INV-3: a refusal must leave a legal next move"
    assert "not reversible" in r.why, "the operator must be told deletion does not unpublish"


def test_the_same_payload_to_a_permitted_destination_passes(tmp_path) -> None:
    """The guard must not simply forbid everything; it decides against a DESTINATION."""
    v = assert_transmittable(
        PERSONAL,
        patterns=FULL,
        destination=Disclosure.OPERATOR_PRIVATE,
        destination_name="the private repo",
    )
    assert v.ceiling is Disclosure.OPERATOR_PRIVATE


def test_an_unclassified_destination_is_refused_not_assumed_private() -> None:
    """Absence-into-zero, at its most costly. 'I cannot tell' must never become 'it is fine'."""
    with pytest.raises(DisclosureError) as exc:
        assert_transmittable(
            BENIGN, patterns=FULL, destination=None, destination_name="somewhere"
        )
    assert "not classified" in exc.value.refusal.why


def test_an_unscanned_class_is_not_a_clean_one() -> None:
    """A guard that cannot see must not report safety.

    With no credential patterns supplied, a payload could carry a credential and scan 'clean'. For
    any destination wider than that class's ceiling, that is refused — 'we found nothing' and 'we
    did not look' are different answers.
    """
    partial = PatternSet(patterns={Sensitivity.OPERATOR_PII: (r"\bADHD\b",)})
    with pytest.raises(DisclosureError) as exc:
        assert_transmittable(
            BENIGN, patterns=partial, destination=Disclosure.PUBLIC, destination_name="public repo"
        )
    assert "did not look" in exc.value.refusal.why
    assert "credential" in exc.value.refusal.why


def test_the_least_disclosing_element_sets_the_ceiling() -> None:
    """One credential in an otherwise public document makes the whole document SECRET.

    THE PAYLOAD MIXES TWO CLASSES DELIBERATELY. An earlier version used a credential alone, where
    min and max over a single element are equal — so a mutant taking the WIDEST ceiling instead of
    the least survived the whole suite. With OPERATOR_PII (ceiling operator_private) beside a
    CREDENTIAL (ceiling secret), the two disagree and only the correct rule gives secret.
    """
    v = scan(BENIGN + PERSONAL + CREDENTIAL, FULL)
    classes = {f.sensitivity for f in v.findings}
    assert Sensitivity.OPERATOR_PII in classes and Sensitivity.CREDENTIAL in classes, (
        "fixture precondition: the payload must carry TWO classes with different ceilings, or this "
        "test cannot distinguish least-disclosing from widest"
    )
    assert v.ceiling is Disclosure.SECRET, (
        "a document containing a credential AND operator PII was rated at the wider of the two. "
        "The ceiling is the LEAST-disclosing thing inside — one secret makes the whole payload "
        f"secret. got {v.ceiling}"
    )


def test_findings_never_quote_what_they_found() -> None:
    """A findings report that quotes the credential is itself a disclosure.

    Reports travel further than the payloads they describe — into logs, tickets, and chat.
    """
    v = scan(CREDENTIAL, FULL)
    assert v.findings, "fixture precondition: the credential must be detected"
    for f in v.findings:
        assert "NOTAREALKEY" not in repr(f), "the matched secret leaked into the finding"


def test_a_benign_payload_reaches_public_when_fully_scanned() -> None:
    """The guard must permit the ordinary case, or it will be routed around."""
    v = assert_transmittable(
        BENIGN, patterns=FULL, destination=Disclosure.PUBLIC, destination_name="public repo"
    )
    assert v.findings == ()
    assert v.scanned_everything


def test_the_kernel_ships_no_patterns() -> None:
    """guard-as-law / patterns-as-estate-data. A kernel carrying one estate's secrets cannot ship."""
    empty = PatternSet()
    assert empty.covered == frozenset(), "the kernel must supply no patterns of its own"
    assert set(CEILING) == set(Sensitivity), "every class in the LAW must carry a ceiling"


def test_the_ladder_is_ordered_as_data() -> None:
    assert width(Disclosure.SECRET) < width(Disclosure.OPERATOR_PRIVATE) < width(
        Disclosure.ESTATE_INTERNAL
    ) < width(Disclosure.PUBLIC)


def test_a_malformed_pattern_is_refused_at_construction() -> None:
    """A pattern that does not compile would silently match nothing — a guard that always passes."""
    with pytest.raises(ValueError, match="does not compile"):
        PatternSet(patterns={Sensitivity.CREDENTIAL: (r"([unclosed",)})


def test_the_guard_and_its_own_support_files_are_estate_independent() -> None:
    """THE SCAN MUST COVER ITSELF AND ITS EXPLANATION.

    k0/test_k0.py scans the kernel package. This module, the scanner it tests, and the conftest
    that supplies the tokens all live OUTSIDE that package and were therefore unscanned — and the
    first draft of conftest.py quoted the very denylist it was written to remove. Unscanned is not
    clean; that is this module's own law, and it applies to this module.
    """
    import pathlib

    tokens = estate_tokens()
    if tokens is None:
        pytest.skip(UNARMED)

    here = pathlib.Path(__file__).parent
    for name in ("disclosure.py", "conftest.py", "test_disclosure.py"):
        src = (here / name).read_text()
        for token in tokens:
            assert token not in src, (
                f"estate fingerprint in {name}, which ships to strangers. The token is not quoted "
                f"here: a failure report travels further than the file it describes."
            )
