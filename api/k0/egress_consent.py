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
  * THE WELL-ORDERING IS ENFORCED AT THE WIRE, AUDITED BY THE CHAIN. The kernel's one
    transmitting transport self-gates through `require_egress`, so no dial can precede consent
    in fact; independently, the receipt spine's phase-legality forbids a MEASURED_PROBE row
    ahead of any later STIPULATION_RATIFY row, and this gate verifies the chain before trusting
    any row in it. Post-probe rotation of the receipt record is chain-illegal; egress consent
    is exactly-once per ceremony, which is what "first consent" means.

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
        # 64 bits, not 32 (codex r17 major): a consent-artifact identifier collides by birthday
        # at ~2^16 with 8 hex chars — too thin for a governance id. (degradation.py's [:8]
        # predates this and is a recorded follow-up, not a precedent to copy.)
        return hashlib.sha256(self.body()).hexdigest()[:16]

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


@dataclass(frozen=True)
class RatifiedEgress:
    """The consent in effect, WITH its amendment count. Supersession is surfaced, never silent:
    a second ratified allowlist inside the pre-probe window is legal (phases only regress after
    it), so the latest row governs — and `amendments` tells any renderer how many earlier
    consents it overrode (claude r2 major). An operator who never amended and reads 1 knows the
    ledger is saying something they did not do."""

    allowlist: EgressAllowlist
    amendments: int
    #: Whether the consent row was authenticated against the sovereign's key at read time.
    #: False is not silent skipping — it is the answer's authentication status, as data, and
    #: anything driving a decision from this object can see it (claude r21).
    signature_verified: bool


def ratified_allowlist(
    root: Path,
    *,
    allowed_signers: Path | None = None,
    principal: str | None = None,
    scratch_dir: Path | None = None,
) -> RatifiedEgress | None:
    """The consented allowlist, verified against the chain pin at read time. None renders
    dark — but a CORRUPTED consent artifact refuses, because silent-deny would hide tampering
    inside the safe-looking answer. The chain itself is verified first: load_chain does not
    check hashes, and a gate that trusts an unverified chain is a gate over nothing.

    SIGNATURES (claude r18): hash-pinning proves the artifact matches the ROW; it does not
    prove the row was signed by the sovereign's key. When the signing materials are supplied
    (allowed_signers + principal + scratch_dir), the ratification is also verified against the
    key — and a row that does not verify is a refusal, not a consent. Callers that have the
    materials MUST pass them; the hash-only path exists for readers that genuinely lack them,
    and it says so."""
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
    materials_supplied = allowed_signers is not None and principal is not None and scratch_dir is not None
    if materials_supplied:
        from .ratification import verify_ratifications
        from .refusal import RefusalError

        try:
            verdict = verify_ratifications(
                root, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
            )
            ok = sid in verdict.verified
            why = dict(verdict.unverified).get(sid, "not present among the verified ratifications")
        except RefusalError as exc:
            ok = False
            gate = getattr(getattr(exc, "refusal", None), "gate", None) or "ratifier"
            why = f"the ratifier REFUSED ({gate}): {exc}"  # preserved, never collapsed (claude r21)
        if not ok:
            raise EgressConsentError(
                f"{sid}: the ratification does not verify against the sovereign's key: {why}",
                Refusal(
                    gate="egress.signature-verification",
                    why="a consent row that does not verify is not consent, however the bytes line up",
                    legal_next=(
                        "verify_ratifications() shows every unverified row with its reason; "
                        "establish whether the key rotated or the row is forged before trusting "
                        "anything on this chain"
                    ),
                ),
            ) from None
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
    except (UnicodeDecodeError, ValueError, KeyError, TypeError) as exc:
        raise EgressConsentError(
            f"{sid}: the consented allowlist is not decodable as an allowlist",
            Refusal(
                gate=gate,
                why="the consented bytes are unusable",
                legal_next="restore the body from backup, or elicit and accept a fresh allowlist",
            ),
        ) from None
    # Construction is OUTSIDE the try (claude r20): a bug in these constructors is a real bug
    # and must crash as itself, never masquerade as "not decodable".
    return RatifiedEgress(
        allowlist=EgressAllowlist(hosts=tuple(hosts)),
        # Count only rows that actually pin a consent artifact: a ratified egress row with
        # no parseable digest claims nothing, and must not inflate the amendment count
        # (claude r8).
        amendments=sum(1 for r in _ratified_rows(root) if artifact_digest(r) is not None) - 1,
        signature_verified=materials_supplied,
    )


def egress_decision(
    root: Path,
    host: str,
    *,
    allowed_signers: Path,
    principal: str,
    scratch_dir: Path,
) -> bool:
    """May the estate transmit to `host`? Default-DENY, and the well-ordering is INHERITED law.

    False when no allowlist is ratified (dark, not an error) or the host is not named. The
    signing materials are mandatory on anything named like a gate (claude r20): a public
    hash-only decision function is a gate that skips the sovereign's signature, and readers
    who genuinely lack the materials can say so by calling `ratified_allowlist` directly —
    which returns data, never a transmit-shaped answer. The ordering guarantee has two layers,
    stated exactly (claude r16): ACTUAL TRANSMISSIONS are
    ordered at the wire — the kernel's one transport self-gates through `require_egress`, so no
    dial can precede consent regardless of what rows exist. RECEIPT ROWS are ordered by the
    spine's phase-legality law (a MEASURED_PROBE row before a later STIPULATION_RATIFY row is
    chain-illegal), and `ratified_allowlist` verifies the chain before trusting a row in it.
    The wire is the enforcement; the chain is the audit.
    """
    ratified = ratified_allowlist(
        root, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
    )
    if ratified is None:
        return False
    return host in ratified.allowlist.hosts


def require_egress(
    root: Path,
    host: str,
    *,
    allowed_signers: Path,
    principal: str,
    scratch_dir: Path,
) -> None:
    """The enforcement primitive: transmitting callers call this FIRST, or not at all legally.

    `egress_decision` answers (hash-verified, for readers); this REFUSES, and on the enforcement
    path the consent row must also authenticate against the sovereign's key (r19, both seats):
    hash links cannot catch an appended forged row — the hashes recompute from content — so a
    gate that skips the signature trusts whatever the file says. The signing materials are
    MANDATORY here; a caller without them does not transmit.
    """
    ratified = ratified_allowlist(
        root, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
    )
    if ratified is None:
        raise EgressConsentError(
            f"{host}: no egress allowlist is ratified at all — the estate is dark, not broken",
            Refusal(
                gate="egress.default-deny",
                why="nothing is consented, so everything is denied — darkness is the honest "
                "state, and it is not the same answer as a corrupted artifact",
                legal_next=(
                    "elicit_allowlist + accept an allowlist naming the hosts the estate may "
                    "reach (the consent ceremony), then retry"
                ),
            ),
        )
    if host in ratified.allowlist.hosts:
        return
    raise EgressConsentError(
        f"{host}: the ratified egress allowlist does not name this host",
        Refusal(
            gate="egress.default-deny",
            why="the estate may not transmit where the operator has not consented — an "
            "unlisted host is a denial, never a guess",
            legal_next=(
                f"elicit_allowlist + accept a fresh allowlist adding {host} (an amendment is a "
                "new consent), or hold the act; dark is a legal state"
            ),
        ),
    )
