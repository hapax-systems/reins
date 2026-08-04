"""The portable disclosure guard: what may never leave, and where it may never go. (R0.10)

## This is NOT a kernel member, deliberately

The ratified K0 manifest (pin b604b52b, 2026-08-01) classifies this mechanism in its EXCLUSION
ledger, and the classification is correct:

    "pii-consent-scanner": R0.10 — installed by `install`, its patterns by `ratify`. Mandatory
    before the first model call (a MEASURED_PROBE precondition), which is later than K0.

A scanner is installable; it is not presupposed by every bootstrap act. So it lives here, outside
`k0/`, as ordinary code that DEPENDS on the kernel (it raises a kernel `Refusal`) rather than
being part of it. The first draft of this module was written into `k0/` under a heading arguing
for its membership — which would have moved the drift pin as a side effect of an edit. Kernel
membership changes only under enforce-flip (P5) with a KERNEL_UPGRADE receipt, never because a new
file looked kernel-shaped.

## Why it exists at all, given the estate already has a guard

The estate already has `pii-guard.sh`. It is fail-closed and it is good, and it did not prevent the
incident that motivated this module, for two structural reasons:

1. **Its patterns are tuned to one operator.** A guard whose contents are one estate's secrets
   cannot ship to a stranger, so a stranger's first-init has no guard at all.
2. **It gates FILE WRITES, not DESTINATIONS.** Every write was to a legitimate file. The harm came
   from where those files were then *published*.

Recorded plainly because it is the whole reason this exists: operator-personal material — a health
diagnosis, family content, mortality planning — was pushed to a PUBLIC repository. Nothing in the
estate objected. The content was correct, the file was correct, the branch was correct; the
DESTINATION was not, and destination was the one thing no guard modelled.

## The split R0.10 asks for

    guard-as-law            -> here, portable, no estate content
    pattern-contents        -> supplied by the caller, per estate, never in the kernel

The LAW is that four classes must never be transmitted beyond their permitted disclosure: operator
PII, raw host fingerprints, transcripts, and credentials. That law is universal and ships. WHICH
strings match those classes is entirely an estate's own data and must not be in a kernel a stranger
runs — that is the same error as the purview engine carrying one estate's census, and the same fix.

## Disclosure classes are ordered, and a payload carries its own ceiling

`SECRET < OPERATOR_PRIVATE < ESTATE_INTERNAL < PUBLIC`, widening left to right. A payload's ceiling
is the LEAST-disclosing class among everything it contains: one credential in an otherwise public
document makes the whole document SECRET. Transmission is permitted only when the destination's
class does not exceed the payload's ceiling.

This is the check that was missing. "Is this file safe?" has no answer on its own. "Is this file
safe **to send there**?" does.

## Fail-closed, and honest about what it cannot know

An unclassified destination is REFUSED, not assumed private — the estate's absence-into-zero defect
is exactly what turns "I don't know where this goes" into "it's fine". And a scan with no patterns
supplied does not return "clean"; it returns UNSCANNED, which is not a pass. A guard that cannot
see must not report safety.
"""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass, field
from enum import StrEnum
from types import MappingProxyType

from k0.refusal import Refusal


class Disclosure(StrEnum):
    """Where something may go. Ordered by how widely it may be disclosed."""

    SECRET = "secret"
    OPERATOR_PRIVATE = "operator_private"
    ESTATE_INTERNAL = "estate_internal"
    PUBLIC = "public"


#: Explicit order, as data — never derived from declaration order. Deriving it would make the law an
#: artifact of source layout, which an alphabetising refactor could silently redefine.
LADDER: tuple[Disclosure, ...] = (
    Disclosure.SECRET,
    Disclosure.OPERATOR_PRIVATE,
    Disclosure.ESTATE_INTERNAL,
    Disclosure.PUBLIC,
)


def width(level: Disclosure) -> int:
    """How widely `level` may be disclosed. SECRET is 0; PUBLIC is widest."""
    return LADDER.index(level)


class Sensitivity(StrEnum):
    """THE LAW: the four classes that may never exceed their permitted disclosure.

    These are named here because they are universal. What MATCHES them is estate data and lives
    outside the kernel — see `PatternSet`.
    """

    OPERATOR_PII = "operator_pii"
    HOST_FINGERPRINT = "host_fingerprint"
    TRANSCRIPT = "transcript"
    CREDENTIAL = "credential"


#: The ceiling each class may never exceed. This mapping IS the law and ships with the kernel.
CEILING: dict[Sensitivity, Disclosure] = {
    Sensitivity.CREDENTIAL: Disclosure.SECRET,
    Sensitivity.OPERATOR_PII: Disclosure.OPERATOR_PRIVATE,
    Sensitivity.TRANSCRIPT: Disclosure.OPERATOR_PRIVATE,
    Sensitivity.HOST_FINGERPRINT: Disclosure.ESTATE_INTERNAL,
}


class DisclosureError(RuntimeError):
    """Refusal to transmit, carrying a kernel `Refusal` so any surface can project it.

    A refusal is DATA, not a message: it names the gate, says why, and states a legal next move, so
    a caller can render it, log it, or act on it without parsing prose. (The estate tracks this as
    requirement RX.1; a stranger needs only the property, which is why it is spelled out here
    rather than left as a reference to a document they do not have.)
    """

    def __init__(self, refusal: Refusal) -> None:
        # Same shape as the kernel's RefusalError: the refusal IS the message, so the two can
        # never disagree. The previous signature took a separate summary string, which invited a
        # caller to write one thing in the exception and another in the refusal a surface renders.
        super().__init__(refusal.render())
        self.refusal = refusal


@dataclass(frozen=True)
class PatternSet:
    """ESTATE DATA, supplied by the caller. Never shipped in the kernel.

    Maps a sensitivity class to the regexes that identify it *for this estate*. A stranger supplies
    their own; the kernel supplies none, so it can carry no one's secrets.
    """

    #: Accepted as any mapping; STORED as an immutable copy. See __post_init__.
    patterns: Mapping[Sensitivity, tuple[str, ...]] = field(default_factory=dict)

    def __post_init__(self) -> None:
        for cls, pats in self.patterns.items():
            if not isinstance(cls, Sensitivity):
                # The key is not echoed: it is caller-supplied data, and by this module's own
                # rule caller data does not appear in messages that reach logs. Its TYPE plus the
                # valid names is enough to fix a wrong key.
                raise ValueError(
                    f"a pattern key of type {type(cls).__name__} is not a Sensitivity class (the "
                    f"key itself is not repeated here). Use one of: "
                    f"{', '.join(s.value for s in Sensitivity)}"
                )
            for index, p in enumerate(pats):
                # THE RAISE IS OUTSIDE THE `except` BLOCK, DELIBERATELY.
                #
                # `re.error` carries the FULL PATTERN on its `.pattern` attribute, and an estate's
                # patterns are often literal secrets. Two weaker versions of this were caught in
                # review, in order:
                #
                #   `raise ... from exc`  -- keeps the re.error as `err.__cause__`.
                #   `raise ... from None` -- suppresses only the DISPLAY. `__suppress_context__`
                #                            is set, but `err.__context__` still references the
                #                            re.error, so `err.__context__.pattern` still reads it.
                #
                # Both leave the object reachable to anything that walks the chain and prints
                # attributes: Sentry, pytest --showlocals, rich tracebacks, a custom handler. The
                # default traceback shows none of it, which is what made this easy to miss twice.
                #
                # Raising after the handler has exited means there is no exception being handled,
                # so no context is attached and no reference survives.
                #
                # THE BOUNDARY OF THIS GUARANTEE, stated exactly, because a reviewer asked and the
                # honest answer is narrower than "the pattern cannot be recovered":
                #
                #   guaranteed clean   the exception's message; its __cause__/__context__ chain;
                #                      the DEFAULT formatted traceback.
                #   NOT guaranteed     frame locals. `self`, `pats` and `p` are bound in this
                #                      frame, so --showlocals, cgitb, or any locals-capturing
                #                      reporter will display the PatternSet.
                #
                # The second row is not fixable here and implying otherwise would be dishonest:
                # this is the constructor that HOLDS the patterns, so they are in its frame by
                # definition, and every function that touches a PatternSet has the same property.
                # What is in scope is what the exception CARRIES, because that is what propagates
                # to a caller, a log line, or a reporter that did not choose to capture locals.
                #
                # Only the clean message text
                # crosses the boundary; the caller holds the pattern list, so a position is enough.
                # NOTHING FROM THE EXCEPTION CROSSES THIS BOUNDARY EXCEPT AN INTEGER.
                #
                # Measured, because the third review pass turned on exactly this: `str(re.error)`
                # echoes pattern content for some error classes and not others.
                #
                #   "(?P<s>a)(?P<s>b)"  -> "redefinition of group name 's' ..."      LEAKS
                #   "(?P=s)"            -> "unknown group name 's' at position 4"    LEAKS
                #   "s("                -> "missing ), unterminated subpattern ..."  clean
                #   r"\ps"              -> "bad escape \p at position 0"             clean
                #
                # An earlier version interpolated str(exc) and was verified against ONE malformed
                # pattern that happened to fall in the clean half. That is an existential check
                # standing in for a universal one: "this message did not leak" is not "no message
                # leaks". Only `.pos` is taken, and an int cannot carry pattern content.
                bad_at: int | None = None
                failed = False
                try:
                    re.compile(p)
                except re.error as exc:
                    failed, bad_at = True, exc.pos
                if failed:
                    # Raised OUTSIDE the handler: `from None` suppresses only the DISPLAY, leaving
                    # `err.__context__.pattern` readable. Raising after the handler exits means no
                    # exception is being handled, so no reference survives at all.
                    where = "" if bad_at is None else f", at offset {bad_at} within it"
                    raise ValueError(
                        f"{cls.value}: the pattern at index {index} does not compile{where}. "
                        f"Neither the pattern nor the regex engine's message is repeated here: an "
                        f"estate's patterns are often literal secrets and some of those messages "
                        f"quote the text they failed on. A pattern that does not compile matches "
                        f"nothing, which would make this guard silently pass everything in that "
                        f"class. Correct the regex at that index, or remove it and let the class "
                        f"report as unscanned."
                    )

        # FREEZE THE CONTENTS, NOT JUST THE ATTRIBUTE.
        #
        # `frozen=True` stops `self.patterns = ...`; it does nothing about mutating the dict it
        # points at. Two ways a caller bypassed every check above, both measured:
        #
        #   ps.patterns[Sensitivity.OPERATOR_PII] = ("([unclosed",)   direct mutation
        #   d = {...}; ps = PatternSet(patterns=d); d[...] = ...      aliasing the caller's dict
        #
        # Either one puts an unvalidated -- possibly uncompilable -- pattern into a PatternSet that
        # was validated at construction, so the guard silently never applies it. And the failure
        # then surfaces at USE time as a bare re.error, which carries `.pattern`: the bypass
        # reopens the disclosure route this module spent six commits closing.
        #
        # dict() breaks the alias; MappingProxyType blocks the direct write. Validation now holds
        # for the life of the object rather than for the instant of construction.
        object.__setattr__(self, "patterns", MappingProxyType(dict(self.patterns)))

    @property
    def covered(self) -> frozenset[Sensitivity]:
        """Classes this estate can actually detect. The rest are UNSCANNED, not clean."""
        return frozenset(c for c, p in self.patterns.items() if p)


@dataclass(frozen=True)
class Finding:
    """What was found, in terms that disclose nothing.

    NEITHER THE MATCHED TEXT NOR THE PATTERN IS STORED.

    The matched text is obvious: a report quoting the credential it found is itself a disclosure,
    and reports travel further than the payloads they describe.

    The PATTERN is the subtler one, and an earlier version of this class got it wrong. Patterns are
    estate data, and an estate's patterns are routinely LITERAL — a hostname, an operator referent,
    a real key. So storing the pattern meant a findings report republished the denylist, which is
    the same defect as writing the denylist inline in the source, one layer up. It survived the
    matched-text test because that test used a generic regex, where the pattern is not itself the
    secret; with a literal pattern the same code discloses.

    `pattern_index` refers back into the caller's own `PatternSet` for that sensitivity class. The
    caller already holds the patterns, so they lose nothing; anyone reading a leaked report gets an
    integer.
    """

    sensitivity: Sensitivity
    #: Position in the caller's own patterns tuple for `sensitivity`. Never the pattern itself.
    pattern_index: int
    count: int


@dataclass(frozen=True)
class Verdict:
    findings: tuple[Finding, ...]
    unscanned: frozenset[Sensitivity]

    @property
    def ceiling(self) -> Disclosure:
        """The widest disclosure this payload may receive, given what was FOUND.

        Not a claim about what is present — only about what was detected. `unscanned` says where
        that claim is blind.
        """
        if not self.findings:
            return Disclosure.PUBLIC
        return min((CEILING[f.sensitivity] for f in self.findings), key=width)

    @property
    def scanned_everything(self) -> bool:
        return not self.unscanned


def scan(text: str, patterns: PatternSet) -> Verdict:
    """Find sensitive content. Reports what it could NOT look for as well as what it found."""
    findings: list[Finding] = []
    for sensitivity in Sensitivity:
        for index, pattern in enumerate(patterns.patterns.get(sensitivity, ())):
            hits = len(re.findall(pattern, text, re.IGNORECASE))
            if hits:
                findings.append(
                    Finding(sensitivity=sensitivity, pattern_index=index, count=hits)
                )
    return Verdict(
        findings=tuple(findings),
        unscanned=frozenset(set(Sensitivity) - patterns.covered),
    )


def assert_transmittable(
    text: str,
    *,
    patterns: PatternSet,
    destination: Disclosure | None,
    destination_name: str,
) -> Verdict:
    """THE CHECK THAT WAS MISSING. Refuse unless this payload may go THERE.

    "Is this content safe?" is unanswerable alone. "Is this content safe to send to a PUBLIC
    repository?" is decidable, and it is the question whose absence caused the incident.

    Fail-closed on both unknowns:
      * an UNCLASSIFIED destination is refused — not assumed private. Turning "I do not know where
        this goes" into "it is fine" is the estate's absence-into-zero defect at its most costly.
      * a class with NO PATTERNS is refused for any destination wider than that class's ceiling.
        A guard that cannot see must never report safety; "we found nothing" and "we did not look"
        are different answers and only one of them is a pass.
    """
    verdict = scan(text, patterns)

    if destination is None:
        raise DisclosureError(
            Refusal(
                gate="k0.disclosure",
                why=(
                    f"{destination_name} is not classified, so whether this payload may go there is "
                    "unknown. An unclassified destination is refused rather than assumed private — "
                    "treating 'I cannot tell' as 'it is fine' is how private material reaches a "
                    "public place."
                ),
                legal_next=(
                    f"classify {destination_name} as one of "
                    f"{', '.join(d.value for d in LADDER)} and retry"
                ),
                teaches="k0.disclosure: destination is part of the payload's safety, not context for it",
            ),
        )

    blind = [
        s for s in verdict.unscanned if width(destination) > width(CEILING[s])
    ]
    if blind:
        names = ", ".join(sorted(s.value for s in blind))
        raise DisclosureError(
            Refusal(
                gate="k0.disclosure",
                why=(
                    f"no patterns were supplied for {names}, so this payload was never checked for "
                    f"them — and {destination_name} ({destination.value}) is wider than those "
                    "classes permit. 'We found nothing' and 'we did not look' are different answers."
                ),
                legal_next=(
                    f"supply patterns for {names}, or send to a destination no wider than "
                    f"{min((CEILING[s] for s in blind), key=width).value}"
                ),
                teaches="k0.disclosure: an unscanned class is not a clean one",
            ),
        )

    if width(destination) > width(verdict.ceiling):
        classes = ", ".join(sorted({f.sensitivity.value for f in verdict.findings}))
        raise DisclosureError(
            Refusal(
                gate="k0.disclosure",
                why=(
                    f"this payload contains {classes}, so it may go no wider than "
                    f"{verdict.ceiling.value}; {destination_name} is {destination.value}. "
                    "Publication is not reversible: deleting a branch does not unpublish it."
                ),
                legal_next=(
                    f"send to a {verdict.ceiling.value} destination, or remove the "
                    f"{classes} content and re-check"
                ),
                teaches="k0.disclosure: the least-disclosing thing inside sets the ceiling for all of it",
            ),
        )

    return verdict
