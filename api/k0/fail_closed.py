"""K0 member `fail-closed-default` — the law, extracted. (R0.3 / W2)

RATIFIED AS KERNEL 2026-08-01, tier `core`, lever class normative-constraint. The circularity
witness: a governed act installing the fail-closed default would itself run either fail-closed
(presupposing it) or fail-open (violating it before it exists).

THE LAW: an unevaluable predicate DENIES.

Not "is hard to evaluate" and not "evaluated false" — *could not be evaluated at all*. That third
outcome is the one systems forget, and forgetting it is how a guard silently inverts. A predicate
has three outcomes, not two:

    SATISFIED   -> admit
    VIOLATED    -> refuse (the obvious arm)
    UNEVALUABLE -> refuse (the arm that gets dropped)

WHY THIS IS EXTRACTED RATHER THAN PORTED. R0.3's gap describes genericising 63 estate-tuned hook
scripts across 4 runtimes. Porting the fleet would carry the estate's tuning into a kernel that is
supposed to be estate-independent. The kernel needs the LAW, not the scripts; each estate then
writes its own predicates against it.

THE WORKED EXAMPLE IS ALREADY IN THIS REPO. `bootstrap_receipt.declare_durable_root` had exactly
this defect and was hardened on 2026-08-01: `_mount_fstype` returned the literal "unknown" when
/proc/mounts was unreadable, "unknown" was not in the volatile set, so the guard PASSED whenever it
could not evaluate — it accepted /dev/shm. A durability guard that fails open is worse than none,
because it reports a durable root that is not one. This module is that fix generalised so the next
guard cannot repeat it.
"""

from __future__ import annotations

from enum import StrEnum

from .refusal import Refusal, RefusalError


class Evaluation(StrEnum):
    """The three outcomes. UNEVALUABLE is not an error state -- it is a legitimate, expected
    result that the law maps to DENY."""

    SATISFIED = "satisfied"
    VIOLATED = "violated"
    UNEVALUABLE = "unevaluable"


def decide(
    gate: str,
    evaluation: Evaluation,
    *,
    legal_next: str,
    violated_why: str = "",
    unevaluable_why: str = "",
    teaches: str = "",
) -> None:
    """Apply the law. Returns None on admit; raises RefusalError on either refusing arm.

    `legal_next` is required on both refusing arms because a refusal without one is a dead end
    (see refusal.Refusal). The two `*_why` strings are separate deliberately: "we checked and it
    failed" and "we could not check" are different facts, and collapsing them is how an
    unevaluable predicate gets quietly reported as a passing one.
    """
    if evaluation is Evaluation.SATISFIED:
        return None

    if evaluation is Evaluation.VIOLATED:
        why = violated_why.strip() or f"{gate}: predicate violated"
    elif evaluation is Evaluation.UNEVALUABLE:
        why = unevaluable_why.strip() or (
            f"{gate}: predicate could not be evaluated. An unevaluable predicate DENIES — a guard "
            f"that passes when it cannot check is worse than no guard, because it reports a "
            f"condition it never established."
        )
    else:  # pragma: no cover — StrEnum makes this unreachable, kept as the fail-closed tail
        raise RefusalError(
            Refusal(
                gate=gate,
                why=f"{gate}: unknown evaluation {evaluation!r}",
                legal_next=legal_next,
                teaches=teaches,
            )
        )

    raise RefusalError(Refusal(gate=gate, why=why, legal_next=legal_next, teaches=teaches))


def evaluate_optional(value: object | None) -> Evaluation:
    """Convenience for the commonest shape: a probe that returns None when it could not observe.

    The trap this closes: `if value:` treats None and False identically, so "we could not observe"
    silently becomes "we observed a negative" — or, worse, `if value is not False:` treats None as
    a pass. None means UNEVALUABLE and nothing else.
    """
    if value is None:
        return Evaluation.UNEVALUABLE
    return Evaluation.SATISFIED if value else Evaluation.VIOLATED
