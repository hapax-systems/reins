"""K0 member `refusal-as-data` — a refusal is a value, not a message. (R0.3 / W1)

RATIFIED AS KERNEL 2026-08-01, tier `core`, lever class normative-constraint. The circularity
witness: every act can refuse, including the act that would install refusal, and a refusal emitted
before refusal-as-data exists has no legible form and cannot be received.

WHY A TYPE AND NOT A STRING. Today the estate expresses refusal in 63 hook scripts as ad-hoc shell
output. That cannot be projected, audited, or taught from. R4.9 asks for a machine check that every
bootstrap-path gate has a why + legal-next projection; RX.1 asks that surfaces teach from the row
they just produced. Neither is possible against prose on stderr.

THE ONE INVARIANT: NO DEAD ENDS. A refusal must say what the operator may legally do next. This is
the same law the SDLC ladder already enforces as INV-3 ("BLOCKED always escapes" -- the BLOCKED
pseudo-state's transition set is deliberately non-empty), and that R4.5 restates as
"no-dead-end-holds". A refusal with no legal next move is a trap, and this module refuses to
construct one. That check is the whole reason the type exists.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime


class DeadEndRefusalError(ValueError):
    """Raised when a refusal would leave the operator with no legal next move."""


@dataclass(frozen=True)
class Refusal:
    """A refusal, as data.

    `why` and `legal_next` are both required and both non-empty. `teaches` is the doctrine
    reference the projecting surface renders (RX.1); it is optional because not every refusal is
    pedagogical, but a bootstrap-path gate should carry one.
    """

    gate: str
    why: str
    legal_next: str
    teaches: str = ""
    refused_at: datetime = field(default_factory=lambda: datetime.now(UTC))

    def __post_init__(self) -> None:
        if not self.gate.strip():
            raise ValueError("refusal must name the gate that refused")
        if not self.why.strip():
            raise ValueError(
                f"{self.gate}: refusal with no reason. An unexplained refusal cannot be received, "
                f"audited, or taught from."
            )
        if not self.legal_next.strip():
            raise DeadEndRefusalError(
                f"{self.gate}: refusal with no legal next move. Every refusal must leave the "
                f"operator somewhere to go — this is INV-3 (BLOCKED always escapes) applied to the "
                f"kernel. A dead-end refusal is a trap, not a gate."
            )

    def receipt_fields(self) -> dict[str, str]:
        """Project onto a bootstrap receipt with act=REFUSED.

        Deliberately returns only refs and prose the estate already classes as safe: a refusal
        never carries the value it refused, because that value is frequently the thing under
        restriction.
        """
        out = {
            "act": "refused",
            "gate": self.gate,
            "why": self.why,
            "legal_next": self.legal_next,
            "refused_at": self.refused_at.isoformat(),
        }
        if self.teaches.strip():
            out["teaches"] = self.teaches
        return out

    def render(self) -> str:
        """One-line legible form. why and legal-next are never separated -- separating them is how
        a refusal becomes a dead end in practice even when the data carries both."""
        tail = f"  [teaches: {self.teaches}]" if self.teaches.strip() else ""
        return f"REFUSED {self.gate}: {self.why}  -> legal next: {self.legal_next}{tail}"


class RefusalError(Exception):
    """Raised to refuse. Carries the Refusal so callers receive data, never a parsed string."""

    def __init__(self, refusal: Refusal) -> None:
        super().__init__(refusal.render())
        self.refusal = refusal
