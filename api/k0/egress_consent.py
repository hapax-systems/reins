"""R2.4 — first consent and the AIR allowlist.

The renderer already lives under AIR default-deny (reins_context: an absent air decision is a
deny). What the ceremony lacked until now is the CONSENT side: nothing the estate transmits was
ever consented to as a stipulation, so the first model call could be made against an allowlist
nobody signed. The graph gap is verbatim: "egress-consent stipulation ratified BEFORE first
model call; allowlist elicitation as ceremony step + consent receipt".

The shape here is the K0 form applied to egress:

  * THE ALLOWLIST IS A STIPULATION. Exact hosts, canonical bytes, digest-keyed id. Elicited as a
    ceremony step (an ELICITED row), ratified through the same signature path as every other
    act of consent — the consent receipt is the RATIFIED row carrying the operator's signature
    ref.
  * THE BODY IS PERSISTED BEFORE THE CONSENT THAT POINTS AT IT (the R2.6 write-order law), and
    verified against the chain pin at every read. A missing, unreadable, or edited allowlist
    body is not an empty allowlist — it is a refusal, because a corrupted consent artifact that
    reads as "deny everything" would hide tampering inside the safe-looking answer.
  * THE GATE IS DEFAULT-DENY. `egress_decision` allows only an exact host in the ratified
    allowlist. No wildcards: a pattern language is a way to consent to more than was named.
  * THE WELL-ORDERING IS CHECKED, not assumed: if any MEASURED_PROBE-phase row predates the
    ratified allowlist row, the first model call happened before consent and the gate refuses —
    the violation is loud, because the chain itself is telling on the ceremony.

Identity note: the graph's third clause ("ceremony-elicited identity lands AIR-classed") belongs
to the identity-elicitation node; k0 has no operator-identity elicitation surface yet, and this
module does not invent one.
"""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from bootstrap_receipt import BootstrapAct, BootstrapPhase, BootstrapReceipt, load_chain

from .degradation import _write_body_durably
from .ratification import (
    SIGNATURE_DIRNAME,
    Stipulation,
    _id_of,
    artifact_digest,
    propose,
    ratify,
)
from .refusal import Refusal

HOST_GRAMMAR = re.compile(r"^[a-z0-9][a-z0-9.-]{0,252}[a-z0-9]$")


class EgressConsentError(RuntimeError):
    """A refusal-carrying error: corrupted consent is loud, never a silent deny."""

    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


@dataclass(frozen=True)
class EgressAllowlist:
    """The exact hosts the estate may transmit to, as one consented artifact."""

    hosts: tuple[str, ...]

    def __post_init__(self) -> None:
        if not self.hosts:
            raise ValueError(
                "an empty allowlist consents to nothing — decline egress instead of signing "
                "an artifact that says nothing"
            )
        for host in self.hosts:
            if not HOST_GRAMMAR.match(host):
                raise ValueError(
                    f"{host!r} is not an exact hostname — the allowlist names hosts, never "
                    "patterns; a wildcard would consent to more than was named"
                )

    def body(self) -> bytes:
        """The exact bytes consented to. Canonical JSON so the digest is stable across readers."""
        return json.dumps(
            {"hosts": sorted(self.hosts)},
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def digest_short(self) -> str:
        return hashlib.sha256(self.body()).hexdigest()[:8]

    def stipulation_id(self) -> str:
        return f"egress-allowlist.{self.digest_short()}"

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"EGRESS CONSENT: the estate may transmit to exactly {len(self.hosts)} named hosts",
            self.body(),
        )


def elicit_allowlist(
    root: Path,
    allowlist: EgressAllowlist,
    *,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The ceremony step: the allowlist is put to the sovereign, on the record."""
    return propose(
        root,
        allowlist.stipulation(),
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def accept(
    root: Path,
    allowlist: EgressAllowlist,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The sovereign consents to the allowlist. The body is durable BEFORE the row that pins
    it — a crash leaves at worst an orphan body, never a consent without an artifact."""
    stip = allowlist.stipulation()
    _write_body_durably(root / SIGNATURE_DIRNAME / f"{stip.stipulation_id}.body", allowlist.body())
    return ratify(
        root,
        stip,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def _ratified_row(root: Path) -> tuple[str, BootstrapReceipt] | None:
    """The latest ratified egress-allowlist row, as (stipulation_id, receipt), or None."""
    found = [
        receipt
        for receipt in load_chain(root)
        if receipt.act is BootstrapAct.RATIFIED
        and _id_of(receipt).startswith("egress-allowlist.")
    ]
    if not found:
        return None
    receipt = found[-1]
    return _id_of(receipt), receipt


def ratified_allowlist(root: Path) -> EgressAllowlist | None:
    """The consented allowlist, verified against the chain pin at read time. None renders
    dark — but a CORRUPTED consent artifact refuses, because silent-deny would hide tampering
    inside the safe-looking answer."""
    found = _ratified_row(root)
    if found is None:
        return None
    sid, receipt = found
    gate = "egress.allowlist-integrity"
    pinned = artifact_digest(receipt)
    path = root / SIGNATURE_DIRNAME / f"{sid}.body"
    try:
        raw = path.read_bytes()
    except OSError as exc:
        raise EgressConsentError(
            f"{sid}: the ratified allowlist body cannot be read ({exc.strerror or 'missing'})",
            Refusal(
                gate=gate,
                why="the chain consents to an artifact that is gone or unreadable",
                legal_next=(
                    "restore the allowlist body from backup, or elicit and accept a fresh "
                    "allowlist — a deleted consent artifact is not an empty allowlist"
                ),
            ),
        ) from None
    if pinned is None or hashlib.sha256(raw).hexdigest() != pinned:
        raise EgressConsentError(
            f"{sid}: the stored allowlist does not hash to the digest the chain pins — the "
            "artifact changed after consent",
            Refusal(
                gate=gate,
                why="the stored allowlist is not the artifact the operator ratified",
                legal_next=(
                    "restore the body whose sha256 is the pinned digest, or elicit and accept "
                    "a fresh allowlist carrying the changed terms"
                ),
            ),
        )
    try:
        hosts = json.loads(raw.decode("utf-8"))["hosts"]
    except (UnicodeDecodeError, ValueError, KeyError, TypeError):
        raise EgressConsentError(
            f"{sid}: the consented allowlist is not decodable as an allowlist",
            Refusal(
                gate=gate,
                why="the consented bytes are unusable",
                legal_next="restore the body from backup, or elicit and accept a fresh allowlist",
            ),
        ) from None
    return EgressAllowlist(hosts=tuple(hosts))


def egress_decision(root: Path, host: str) -> bool:
    """May the estate transmit to `host`? Default-DENY, and the well-ordering is law.

    False when no allowlist is ratified (dark, not an error) or the host is not named. LOUD when
    the consent artifact is corrupted, and loud when a MEASURED_PROBE-phase row predates the
    ratification — the first model call happened before consent, and the chain is telling on
    the ceremony.
    """
    found = _ratified_row(root)
    if found is None:
        return False
    sid, receipt = found
    chain = load_chain(root)
    ratified_index = max(i for i, r in enumerate(chain) if r.receipt_id == receipt.receipt_id)
    for earlier in chain[:ratified_index]:
        if earlier.phase == BootstrapPhase.MEASURED_PROBE:
            raise EgressConsentError(
                "a MEASURED_PROBE-phase receipt predates the ratified egress allowlist — the "
                "first model call happened before consent",
                Refusal(
                    gate="egress.well-ordering",
                    why="consent after the fact is not consent",
                    legal_next=(
                        "the ceremony order was violated on this host; verify_chain, establish "
                        "what transmitted, and re-run the ceremony cleanly — do not ratify over "
                        "the gap"
                    ),
                ),
            )
    allowlist = ratified_allowlist(root)
    if allowlist is None:
        return False
    return host in allowlist.hosts
