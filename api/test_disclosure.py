"""R0.10 — the portable disclosure guard, tested against the incident that motivated it."""

from __future__ import annotations

import traceback

import pytest

from conftest import UNARMED, estate_tokens, scan_tree_for_tokens

from disclosure import (
    CEILING,
    Disclosure,
    DisclosureError,
    PatternSet,
    Sensitivity,
    Verdict,
    assert_transmittable,
    scan,
    width,
)

#: ESTATE DATA, supplied by the caller — the kernel ships none. These are invented for the test.
FULL = PatternSet(
    patterns={
        Sensitivity.OPERATOR_PII: (r"\bADHD\b", r"\bautis(m|tic)\b", r"\bmortality\b"),
        Sensitivity.HOST_FINGERPRINT: (r"\bexample-host-\w+\b",),
        Sensitivity.TRANSCRIPT: (r"^\s*(operator|assistant):",),
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


def test_the_incident_is_refused() -> None:
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


def test_the_same_payload_to_a_permitted_destination_passes() -> None:
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


def test_a_finding_never_carries_the_pattern_because_patterns_are_often_literal() -> None:
    """THE CASE THAT SLIPPED THROUGH, found by two independent reviewers.

    `Finding` used to store the pattern that matched. The test above did not catch it because it
    used a GENERIC regex, where the pattern is not the secret. But an estate's patterns are
    routinely literal — this estate's own token list is literal strings — so storing the pattern
    meant a findings report republished the denylist verbatim.

    That is the same defect as writing the denylist inline in the source, one layer up: the artifact
    that DESCRIBES the secret becomes the secret's new home. It is worth being exact about why the
    original test could not see it: it asserted a property of the payload's relationship to the
    report, and the leak was in the pattern's relationship to the report.

    So this test uses a literal pattern, and checks the whole Verdict — a Finding that stayed clean
    while the Verdict's repr leaked would be no better.
    """
    literal = "zzq-secret-literal-value"
    patterns = PatternSet(patterns={Sensitivity.OPERATOR_PII: (literal,)})
    v = scan(f"the document mentions {literal} once", patterns)

    assert v.findings, "fixture precondition: the literal pattern must match"
    assert literal not in repr(v), "the pattern leaked through the Verdict"
    for f in v.findings:
        assert literal not in repr(f), "the pattern leaked through the Finding"
        assert f.pattern_index == 0, "the caller must still be able to map back to its own list"


#: Malformed patterns whose `re.error` message ECHOES the pattern's own content. Measured, not
#: assumed — `str(re.error)` is clean for some error classes and not others, and an earlier version
#: of this test used only a clean-half pattern. It passed while the code interpolated str(exc), so
#: a mutant restoring that interpolation SURVIVED the whole suite. "This message did not leak" is
#: not "no message leaks"; the same existential-for-universal slip the module's docstring warns
#: about, committed in the test meant to catch it.
LEAKING_PATTERNS = [
    "(?P<zzqsecret>a)(?P<zzqsecret>b)",  # -> "redefinition of group name 'zzqsecret' ..."
    "(?P=zzqsecret)",                    # -> "unknown group name 'zzqsecret' at position 4"
]
#: The other half, where re.error says nothing about the content. Included so the test covers both
#: behaviours of the engine rather than whichever one happened to be sampled.
QUIET_PATTERNS = ["zzqsecret(", r"\pzzqsecret"]


@pytest.mark.parametrize("pattern", LEAKING_PATTERNS + QUIET_PATTERNS)
def test_a_pattern_that_does_not_compile_is_named_by_position_not_by_content(pattern) -> None:
    """The error must disclose nothing, for EVERY class of regex error — not a sampled one.

    An estate's patterns are often literal secrets, and this message reaches logs and tickets.
    """
    with pytest.raises(ValueError) as exc:
        PatternSet(patterns={Sensitivity.CREDENTIAL: (pattern,)})
    assert "index 0" in str(exc.value)
    assert "zzqsecret" not in str(exc.value), (
        "the failing pattern's content reached the exception message"
    )

    # THE WHOLE CHAIN, not just the message. `from None` suppresses only the DISPLAY: it sets
    # __suppress_context__ while leaving __context__ pointing at the re.error, whose `.pattern`
    # attribute holds the full pattern. Sentry, pytest --showlocals and rich tracebacks all read
    # attributes. So the assertion is over every exception reachable from the one raised.
    seen, err = [], exc.value
    while err is not None:
        seen.append(err)
        err = err.__cause__ or err.__context__
    for link in seen:
        assert "zzqsecret" not in repr(vars(link) if hasattr(link, "__dict__") else {}), (
            "an exception in the chain carries the pattern in its attributes"
        )
        assert "zzqsecret" not in repr(getattr(link, "pattern", "")), (
            "the failing regex is still reachable through the exception chain"
        )

    # AND THE RENDERED TRACEBACK, which is what actually reaches a log.
    #
    # Deliberately the DEFAULT rendering, matching the guarantee the module states. Frame locals are
    # explicitly outside that guarantee: `self`, `pats` and `p` are bound in the constructor frame,
    # so --showlocals or a locals-capturing reporter WILL show the PatternSet. That is not fixable
    # in a constructor whose job is to hold the patterns, and asserting it here would encode a
    # promise the code cannot keep.
    rendered = "".join(
        traceback.format_exception(type(exc.value), exc.value, exc.value.__traceback__)
    )
    assert "zzqsecret" not in rendered, "the pattern reached the default formatted traceback"


def test_the_leaking_fixtures_really_do_leak_or_this_test_proves_nothing() -> None:
    """A GUARD ON THE GUARD.

    If a future Python changed those messages to stop quoting group names, the parametrized test
    above would keep passing while testing nothing — coverage theater with a green tick. So the
    premise is asserted directly: these patterns must still produce engine messages that echo their
    own content, or this file must be told so and updated.
    """
    import re

    for pattern in QUIET_PATTERNS:
        with pytest.raises(re.error) as exc:
            re.compile(pattern)
        assert "zzqsecret" not in str(exc.value), (
            f"{pattern!r} now DOES echo its content on this Python. That is not a failure of the "
            f"module — it is the split between the two halves moving. Reclassify it into "
            f"LEAKING_PATTERNS so the distinction this file documents stays true."
        )

    for pattern in LEAKING_PATTERNS:
        with pytest.raises(re.error) as exc:
            re.compile(pattern)
        assert "zzqsecret" in str(exc.value), (
            f"{pattern!r} no longer produces a content-echoing re.error message on this Python. "
            f"The disclosure test above is now vacuous — find a pattern that still does, or drop "
            f"the distinction if the engine no longer echoes content at all."
        )


def test_an_empty_findings_tuple_yields_public_as_a_boundary_in_its_own_right() -> None:
    """Asserted directly, not inferred from a benign-payload test.

    `ceiling` has two branches and the no-findings branch is the one that ADMITS. Reaching it only
    through a payload test means a change to the scan could stop exercising it without any test
    going red.
    """
    assert Verdict(findings=(), unscanned=frozenset()).ceiling is Disclosure.PUBLIC


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


def test_the_guard_and_everything_beside_it_is_estate_independent() -> None:
    """THE SCAN MUST COVER ITSELF, ITS EXPLANATION, AND ANYTHING THAT ARRIVES LATER.

    k0/test_k0.py scans the kernel package. This module, the scanner it tests, and the conftest
    that supplies the tokens all live OUTSIDE that package and were therefore unscanned — and the
    first draft of conftest.py quoted the very denylist it was written to remove. Unscanned is not
    clean; that is this module's own law, and it applies to this module.

    IT SCANS THE WHOLE DIRECTORY, NOT A NAMED LIST. An earlier version enumerated three filenames,
    which meant a new file beside them was silently out of scope — a by-name exemption wearing an
    allowlist's clothes, and the fourth variation on the same mistake in this change. `test_estate_tokens.py`
    was already sitting outside that triple when a reviewer pointed it out.
    """
    import pathlib

    tokens = estate_tokens()
    if tokens is None:
        pytest.skip(UNARMED)

    hits = scan_tree_for_tokens(pathlib.Path(__file__).parent, tokens)
    assert not hits, (
        f"estate fingerprints in files that ship to strangers: "
        f"{sorted({p.name for p, _ in hits})}. The tokens are not quoted here: a failure report "
        f"travels further than the files it describes."
    )

def test_a_pattern_key_that_is_not_a_sensitivity_class_is_refused() -> None:
    """An unrecognised key would silently never be scanned for.

    `scan` iterates the Sensitivity enum, so a typo'd or string key is not merely ignored — the
    patterns under it are never applied, and the class reports as UNSCANNED with no indication that
    the caller thought they had supplied it. Fail-closed at construction, with the valid names in
    the message so the caller can fix it without reading the source.
    """
    bad_key = "zzq-mistyped-key"
    with pytest.raises(ValueError, match="is not a Sensitivity class") as exc:
        PatternSet(patterns={bad_key: (r"x",)})

    for name in ("operator_pii", "host_fingerprint", "transcript", "credential"):
        assert name in str(exc.value), "the message must list the valid classes"

    # AND THE KEY ITSELF IS NOT ECHOED. It is caller-supplied data reaching a log, which is the
    # same rule the patterns follow. Its TYPE plus the valid names is enough to fix a wrong key.
    assert bad_key not in str(exc.value), "the caller's key was echoed into the error message"
    assert "str" in str(exc.value), "the type is what identifies the mistake, so it must be named"


def test_the_frame_locals_limitation_is_real_and_pinned() -> None:
    """PINS A KNOWN, DOCUMENTED LIMITATION so it cannot change unnoticed.

    The module states its boundary: the exception's message, chain, and default traceback carry no
    pattern content, but FRAME LOCALS do, because `PatternSet.__post_init__` is the constructor
    that holds the patterns. a reviewer asked for the acknowledged gap to be tested, which is right — an
    untested caveat is a comment, and comments drift.

    This asserts the limitation EXISTS, not that it is acceptable. If a future change makes frame
    locals clean, this test fails and the docstring must be corrected to claim the stronger
    property. A caveat that silently became false would leave the module under-claiming, which
    misleads a caller in the opposite direction.
    """
    literal = "zzq-frame-local-probe("
    with pytest.raises(ValueError) as exc:
        PatternSet(patterns={Sensitivity.CREDENTIAL: (literal,)})

    tb = exc.value.__traceback__
    while tb.tb_next:
        tb = tb.tb_next
    assert "zzq-frame-local-probe" in repr(tb.tb_frame.f_locals), (
        "frame locals no longer carry the pattern. That is an IMPROVEMENT, not a failure — but the "
        "module documents this as an explicit limitation, so update that docstring to claim the "
        "stronger guarantee rather than leaving it under-claimed."
    )


def test_validation_cannot_be_bypassed_by_mutating_the_pattern_dict() -> None:
    """`frozen=True` freezes the ATTRIBUTE, not the dict it points at.

    Two ways a caller got an unvalidated pattern into a validated PatternSet, both measured before
    the fix. The consequence is not merely an uncompilable regex: an unvalidated pattern is never
    applied, so the class silently reports as scanned-and-clean when it was never scanned — and the
    failure that does surface is a bare `re.error`, which carries `.pattern`, reopening the
    disclosure route the rest of this module exists to close.
    """
    ps = PatternSet(patterns={Sensitivity.CREDENTIAL: (r"a",)})
    with pytest.raises(TypeError):
        ps.patterns[Sensitivity.OPERATOR_PII] = ("([never-validated",)  # type: ignore[index]

    # AND THE ALIAS. Holding the caller's dict would let them edit it after construction, which no
    # amount of read-only wrapping on our side would prevent — the copy is what breaks it.
    caller = {Sensitivity.CREDENTIAL: (r"a",)}
    ps2 = PatternSet(patterns=caller)
    caller[Sensitivity.TRANSCRIPT] = ("([never-validated",)
    assert Sensitivity.TRANSCRIPT not in ps2.patterns, (
        "the PatternSet still points at the caller's dict, so a post-construction edit slipped an "
        "unvalidated pattern past every check in __post_init__"
    )
    assert ps2.covered == frozenset({Sensitivity.CREDENTIAL})

    # AND THE VALUES, one level deeper. The annotation says tuple; the annotation is not
    # enforcement, and a caller passing a LIST keeps a live reference to it. Freezing the mapping
    # alone left every value aliased — which is why the shallow fix looked complete, and is the
    # same lesson as the guard's six output channels: closing one level says nothing about the next.
    mutable = ["a"]
    ps3 = PatternSet(patterns={Sensitivity.CREDENTIAL: mutable})
    mutable.append("([never-validated")
    assert ps3.patterns[Sensitivity.CREDENTIAL] == ("a",), (
        "a list passed as a pattern sequence stayed aliased to the caller, so appending to it "
        "inserted an unvalidated pattern after construction"
    )
