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
  * THE WELL-ORDERING IS INHERITED LAW. The receipt spine's phase-legality already forbids a
    MEASURED_PROBE row ahead of any later STIPULATION_RATIFY row (phases never regress), so a
    ceremony that probed before consenting fails chain verification — and this gate verifies
    the chain before trusting any row in it. Post-probe rotation is chain-illegal for the same
    reason; egress consent is exactly-once per ceremony, which is what "first consent" means.

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

from bootstrap_receipt import (
    BootstrapAct,
    BootstrapPhase,
    BootstrapReceipt,
    load_chain,
    verify_chain_at,
)

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


def _ratified_rows(root: Path) -> list[BootstrapReceipt]:
    """Every ratified egress-allowlist row, in chain order."""
    return [
        receipt
        for receipt in load_chain(root)
        if receipt.act is BootstrapAct.RATIFIED
        and _id_of(receipt).startswith("egress-allowlist.")
    ]


def _ratified_row(root: Path) -> tuple[str, BootstrapReceipt] | None:
    """The latest ratified egress-allowlist row, as (stipulation_id, receipt), or None."""
    found = _ratified_rows(root)
    if not found:
        return None
    receipt = found[-1]
    return _id_of(receipt), receipt


def ratified_allowlist(root: Path) -> EgressAllowlist | None:
    """The consented allowlist, verified against the chain pin at read time. None renders
    dark — but a CORRUPTED consent artifact refuses, because silent-deny would hide tampering
    inside the safe-looking answer. The chain itself is verified first: load_chain does not
    check hashes, and a gate that trusts an unverified chain is a gate over nothing."""
    if not verify_chain_at(root).ok:
        raise EgressConsentError(
            "the bootstrap chain fails verification — no row in it may ground an egress decision",
            Refusal(
                gate="egress.chain-integrity",
                why="an unverifiable chain cannot prove what was consented to",
                legal_next="run verify_chain to find the break, restore from backup, then re-run",
            ),
        )
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
        parsed = json.loads(raw.decode("utf-8"))
        hosts = parsed["hosts"] if isinstance(parsed, dict) else None
        if not isinstance(hosts, list) or not all(isinstance(h, str) for h in hosts):
            raise TypeError("hosts is not a list of hostnames")
        return EgressAllowlist(hosts=tuple(hosts))
    except (UnicodeDecodeError, ValueError, KeyError, TypeError):
        raise EgressConsentError(
            f"{sid}: the consented allowlist is not decodable as an allowlist",
            Refusal(
                gate=gate,
                why="the consented bytes are unusable",
                legal_next="restore the body from backup, or elicit and accept a fresh allowlist",
            ),
        ) from None


def egress_decision(root: Path, host: str) -> bool:
    """May the estate transmit to `host`? Default-DENY, and the well-ordering is INHERITED law.

    False when no allowlist is ratified (dark, not an error) or the host is not named. The
    ordering guarantee — no model call before egress consent — is not re-implemented here: the
    receipt spine's phase-legality law already makes a MEASURED_PROBE row before any later
    STIPULATION_RATIFY row chain-illegal (phases never regress), and `ratified_allowlist`
    verifies the chain before trusting a row in it. A ceremony that probed before consenting
    fails verification, and this gate is loud through exactly that path.
    """
    allowlist = ratified_allowlist(root)
    if allowlist is None:
        return False
    return host in allowlist.hosts
