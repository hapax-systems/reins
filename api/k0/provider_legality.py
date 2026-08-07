"""R2.16 — provider ToS conformance, carried as data the ceremony is shaped by.

The estate already holds the determination (claude-code-tos-good-faith-determination-2026-06-20):
direct OAuth subscription usage is within Anthropic's terms, and the fingerprint-normalizing
gateway class was considered and REJECTED as a violation. What the graph names as the gap is
that this lived in a document, not in the ceremony: "per-provider BYOK-vs-OAuth legality carried
as elicitation-shaping data in the auth ceremony; kit must not architect forbidden wiring for
strangers".

Two properties, both machine-checked:

  * THE FORBIDDEN CLASS IS UNREPRESENTABLE. The acquisition-mode vocabulary has no gateway,
    relay, proxy, or shared-entitlement member — there is no value to construct, so the kit
    cannot architect that wiring for anyone. The absence is the control, and a scan test pins it.
  * LEGALITY SHAPES ELICITATION. `capturable_providers()` is the set of providers whose key may
    be offered for capture (BYOK-legal AND probeable); `validate_key` refuses a BYOK capture for
    a provider whose legal path is entitlement-OAuth, naming the legal alternative. An unknown
    provider fails closed — legality is never assumed.
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum
from types import MappingProxyType
from typing import Mapping


class AcquisitionMode(StrEnum):
    """How a capability's access may legally be acquired.

    The negative space IS the clause: no gateway, relay, proxy, or shared-entitlement member
    exists, so the wiring the ToS determination rejected is unrepresentable in this vocabulary.
    """

    BYOK = "byok"  # the operator's own API key, captured per R2.3
    OAUTH_ENTITLEMENT = "oauth_entitlement"  # the sanctioned harness's own provider sign-in
    LOCAL_RUNTIME = "local_runtime"  # a detected local model runtime; supplementary, never the floor


@dataclass(frozen=True)
class ProviderLegality:
    """One provider's legal acquisition modes, with the determination's basis on record."""

    provider: str
    legal_modes: frozenset[AcquisitionMode]
    basis: str


#: IMMUTABLE (MappingProxyType) — the legality table is determination data, not a config a
#: caller can loosen. A provider enters only with a determination on record.
PROVIDER_LEGALITY: Mapping[str, ProviderLegality] = MappingProxyType(
    {
        "anthropic": ProviderLegality(
            "anthropic",
            frozenset({AcquisitionMode.BYOK, AcquisitionMode.OAUTH_ENTITLEMENT}),
            basis=(
                "claude-code-tos-good-faith-determination-2026-06-20: direct OAuth subscription "
                "and first-party API keys are within the terms; the fingerprint-normalizing "
                "gateway class is rejected as a violation"
            ),
        ),
        "openai": ProviderLegality(
            "openai",
            frozenset({AcquisitionMode.BYOK}),
            basis=(
                "first-party API-key offering; no entitlement-OAuth determination on record, "
                "so the harness path is not asserted for this provider"
            ),
        ),
    }
)


def legal_acquisition_modes(provider: str) -> frozenset[AcquisitionMode]:
    """The legal acquisition modes for a provider. Unknown fails closed: no determination on
    record means no asserted legality, and none is inferred."""
    entry = PROVIDER_LEGALITY.get(provider)
    if entry is None:
        raise KeyError(
            f"{provider!r}: no ToS determination on record — legality is never assumed; add the "
            "provider only with its determination cited"
        )
    return entry.legal_modes


def key_capture_legal(provider: str) -> bool:
    """May the ceremony offer key capture for this provider? Only when BYOK is a legal mode."""
    try:
        return AcquisitionMode.BYOK in legal_acquisition_modes(provider)
    except KeyError:
        return False


def capturable_providers() -> tuple[str, ...]:
    """THE ELICITATION-SHAPING DATA: providers the ceremony may offer key capture for —
    BYOK-legal AND backed by a sanctioned probe endpoint (an offered key must be provable).
    """
    from .key_capture import PROVIDER_PROBE_ENDPOINTS

    return tuple(
        provider
        for provider in sorted(PROVIDER_PROBE_ENDPOINTS)
        if key_capture_legal(provider)
    )
