"""R2.14 — the ceremony mints the sovereign identity and the role registry.

The estate's dispatch machinery fail-closes without a role marker (the governed-push lesson):
a packet with no registered role is refused, and an UNREGISTERED-but-real role fails the same
way as an intruder's — illegibly. The graph gap: "ceremony mints sovereign identity + role
registry rows so P4 dispatch never fail-closes illegibly".

Two consented artifacts, same K0 ceremony discipline as every narrowing in this kernel:

  * THE SOVEREIGN IDENTITY. The ratifier principal is how the ledger names who consents. Today
    that principal lives in the operator's key comment and the allowed-signers file — asserted,
    never ratified. Here it becomes a stipulation: principal + the key's own fingerprint, so
    the ledger's consent rows are legible against the identity that made them.
  * THE ROLE REGISTRY. The initial role set is ELICITED content, never hardcoded — this kernel
    carries no estate's roster (the estate-independence scan enforces exactly that). The set is
    ratified as one stipulation; `role_known` reads it. An unknown role fails with the registry
    in view, so a legitimate-but-unregistered role and a nonsense one read differently.

Nothing here grants authority: the registry names roles; what they may do is the dispatch
layer's law, not this module's.
"""

from __future__ import annotations

import hashlib
import json
import subprocess
import re
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from bootstrap_receipt import BootstrapAct, load_chain, verify_chain_at

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

ROLE_GRAMMAR = re.compile(r"[a-z][a-z0-9-]{1,31}")
PRINCIPAL_GRAMMAR = re.compile(r"[a-z][a-z0-9._-]{1,63}@[a-z0-9][a-z0-9.-]{1,62}")


class RoleRegistryError(RuntimeError):
    def __init__(self, message: str, refusal: Refusal | None = None) -> None:
        super().__init__(message)
        self.refusal = refusal


@dataclass(frozen=True)
class SovereignIdentity:
    """Who signs, as a consented artifact: the principal and the ratifier key's fingerprint."""

    principal: str
    key_fingerprint: str  # SHA256:<base64> from ssh-keygen -l — identity, not material

    def __post_init__(self) -> None:
        if not PRINCIPAL_GRAMMAR.fullmatch(self.principal):
            raise ValueError(
                f"{self.principal!r}: a principal is name@host-shaped, lowercase — it becomes "
                "a ledger referent"
            )
        if not self.key_fingerprint.startswith("SHA256:"):
            raise ValueError("the fingerprint is the ssh-keygen -l SHA256 form, never the key")

    def body(self) -> bytes:
        return json.dumps(
            {"principal": self.principal, "key_fingerprint": self.key_fingerprint},
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def stipulation_id(self) -> str:
        return f"sovereign-identity.{hashlib.sha256(self.body()).hexdigest()[:16]}"

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"SOVEREIGN IDENTITY: the ratifier principal is {self.principal}",
            self.body(),
        )


@dataclass(frozen=True)
class RoleSet:
    """The initial role registry, as one consented artifact. Elicited content — never ours."""

    roles: tuple[str, ...]

    def __post_init__(self) -> None:
        # frozen blocks reassignment, not construction with a mutable: normalize first
        # (CodeRabbit), so the registry's bytes cannot drift after minting.
        object.__setattr__(self, "roles", tuple(self.roles))
        if not self.roles:
            raise ValueError("an empty registry dispatches nothing — elicit the roles first")
        for role in self.roles:
            if not ROLE_GRAMMAR.fullmatch(role):
                raise ValueError(
                    f"{role!r}: a role name is lowercase kebab, 2-32 chars — it becomes a "
                    "dispatch referent"
                )
        if len(set(self.roles)) != len(self.roles):
            raise ValueError("duplicate roles: one canonical registry, one entry per role")

    def body(self) -> bytes:
        return json.dumps(
            {"roles": sorted(self.roles)},
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")

    def stipulation_id(self) -> str:
        return f"role-registry.{hashlib.sha256(self.body()).hexdigest()[:16]}"

    def stipulation(self) -> Stipulation:
        return Stipulation.over(
            self.stipulation_id(),
            f"ROLE REGISTRY: {len(self.roles)} roles minted at genesis",
            self.body(),
        )


def _fingerprint_of(key_path: Path) -> str:
    """The SHA256 fingerprint of the signing key's public half, derived in a temp dir —
    nothing is written beside the operator's key. A missing/broken ssh-keygen is a governed
    refusal, never a bare CalledProcessError (CodeRabbit)."""
    import tempfile

    try:
        pub_text = subprocess.run(
            ["ssh-keygen", "-y", "-f", str(key_path)],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
        with tempfile.TemporaryDirectory() as td:
            pub = Path(td) / "key.pub"
            pub.write_text(pub_text + "\n", encoding="utf-8")
            return subprocess.run(
                ["ssh-keygen", "-l", "-f", str(pub)],
                check=True,
                capture_output=True,
                text=True,
            ).stdout.split()[1]
    except (OSError, subprocess.CalledProcessError, IndexError) as exc:
        raise RoleRegistryError(
            f"the signing key's fingerprint could not be derived ({type(exc).__name__})",
            Refusal(
                gate="identity.signer-binding",
                why="key inspection failed — the identity cannot be bound to the signer",
                legal_next="install OpenSSH and confirm the ratifier key is readable, then re-run",
            ),
        ) from None


def _present_and_accept(
    root: Path,
    stip: Stipulation,
    body: bytes,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None,
) -> Path:
    propose(root, stip, estate_id=estate_id, kernel_version=kernel_version, observed_at=observed_at)
    _write_body_durably(root / SIGNATURE_DIRNAME / f"{stip.stipulation_id}.body", body)
    return ratify(
        root,
        stip,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def mint_sovereign_identity(
    root: Path,
    identity: SovereignIdentity,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The ceremony's identity act: the principal is consented, key fingerprint bound —
    and the fingerprint must be the SIGNING key's own (codex r2): an identity naming a
    different key would make the ledger's consent rows legible against a key that never
    signed them."""
    signer_fingerprint = _fingerprint_of(key_path)
    if identity.key_fingerprint != signer_fingerprint:
        raise RoleRegistryError(
            f"the identity names {identity.key_fingerprint[:20]}… but the signing key is "
            f"{signer_fingerprint[:20]}… — the sovereign identity must be the signer's own",
            Refusal(
                gate="identity.signer-binding",
                why="an identity minted for a different key than the one signing is a false "
                "witness, however it came to be",
                legal_next="mint the identity for the key that signs, or sign with the key the "
                "identity names",
            ),
        )
    return _present_and_accept(
        root,
        identity.stipulation(),
        identity.body(),
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def mint_role_registry(
    root: Path,
    roles: RoleSet,
    *,
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> Path:
    """The registry's mint: the initial role set, ratified as one artifact."""
    return _present_and_accept(
        root,
        roles.stipulation(),
        roles.body(),
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def mint_genesis_identity(
    root: Path,
    *,
    principal: str,
    roles: tuple[str, ...],
    key_path: Path,
    estate_id: str,
    kernel_version: str,
    observed_at: datetime | None = None,
) -> None:
    """The ceremony's identity composition (codex r3 critical): identity first, registry
    second — the registry's mint is signed BY the identity the first act just bound, so the
    order is the meaning. This is the entry point the external ceremony driver calls; the
    driver itself arrives with the P2 executor and is not this kernel's to invent here."""
    # Construct BOTH artifacts first (CodeRabbit): their constructors carry the shape laws,
    # so validating up front means a bad role set cannot leave a minted identity behind it.
    identity = SovereignIdentity(principal, _fingerprint_of(key_path))
    role_set = RoleSet(roles)
    mint_sovereign_identity(
        root,
        identity,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )
    mint_role_registry(
        root,
        role_set,
        key_path=key_path,
        estate_id=estate_id,
        kernel_version=kernel_version,
        observed_at=observed_at,
    )


def _read_consented_body(
    root: Path,
    prefix: str,
    gate: str,
    *,
    allowed_signers: Path | None = None,
    principal: str | None = None,
    scratch_dir: Path | None = None,
) -> dict | None:
    """The verified-read half of the ceremony shape, for this module's two artifacts.

    With the signing materials supplied, the ratification row must also authenticate against
    the sovereign's key (CodeRabbit: hash links alone cannot catch an appended forged row).
    Without them the read is hash-only — callers needing the authenticated answer pass all
    three."""
    if not verify_chain_at(root).ok:
        raise RoleRegistryError(
            "the bootstrap chain fails verification",
            Refusal(
                gate=gate,
                why="an unverifiable chain cannot prove what was minted",
                legal_next="run verify_chain to find the break, restore from backup, then re-run",
            ),
        )
    rows = [
        r
        for r in load_chain(root)
        if r.act is BootstrapAct.RATIFIED and _id_of(r).startswith(prefix)
    ]
    if not rows:
        return None
    receipt = rows[-1]
    sid = _id_of(receipt)
    if not re.fullmatch(rf"{re.escape(prefix)}[0-9a-f]{{16}}", sid):
        raise RoleRegistryError(
            f"{sid}: not a canonical id for {prefix}<digest16> — the prefix is reserved",
            Refusal(
                gate=gate,
                why="a malformed id in a reserved namespace is not this artifact",
                legal_next="verify_chain; establish how the row was minted",
            ),
        )
    if allowed_signers is not None and principal is not None and scratch_dir is not None:
        from .ratification import verify_ratifications
        from .refusal import RefusalError

        try:
            verdict = verify_ratifications(
                root, allowed_signers=allowed_signers, principal=principal, scratch_dir=scratch_dir
            )
            ok = sid in verdict.verified and verdict.ok
        except RefusalError:
            ok = False
        if not ok:
            raise RoleRegistryError(
                f"{sid}: the ratification does not authenticate against the sovereign's key",
                Refusal(
                    gate=f"{gate}.signature",
                    why="a consent row that does not verify is not consent",
                    legal_next="verify_ratifications() shows every unverified row with its reason",
                ),
            )
    pinned = artifact_digest(receipt)
    if pinned is None:
        raise RoleRegistryError(
            f"{sid}: the ratified row pins no artifact digest",
            Refusal(gate=gate, why="a mint that names nothing minted nothing", legal_next="verify_chain"),
        )
    try:
        raw = (root / SIGNATURE_DIRNAME / f"{sid}.body").read_bytes()
    except OSError as exc:
        raise RoleRegistryError(
            f"{sid}: the consented body cannot be read ({exc.strerror or 'missing'})",
            Refusal(
                gate=gate,
                why="the chain consents to an artifact that is gone or unreadable (claude r2)",
                legal_next="restore the body from backup, or re-mint with the same terms",
            ),
        ) from None
    actual = hashlib.sha256(raw).hexdigest()
    if actual != pinned or sid.rsplit(".", 1)[1] != actual[:16]:
        raise RoleRegistryError(
            f"{sid}: the stored body is not the artifact the chain pins",
            Refusal(
                gate=gate,
                why="the artifact changed after consent",
                legal_next="restore the pinned body from backup, or re-mint with the changed terms",
            ),
        )
    try:
        return json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, ValueError):
        raise RoleRegistryError(
            f"{sid}: the consented body is not decodable JSON",
            Refusal(
                gate=gate,
                why="the consented bytes are unusable",
                legal_next="restore the body from backup, or re-mint with the same terms",
            ),
        ) from None


def sovereign_principal(
    root: Path,
    *,
    allowed_signers: Path | None = None,
    principal: str | None = None,
    scratch_dir: Path | None = None,
) -> str | None:
    """The ratified principal, or None — dark, never a default identity. With the signing
    materials supplied, the consent row is authenticated first."""
    body = _read_consented_body(
        root,
        "sovereign-identity.",
        "identity.integrity",
        allowed_signers=allowed_signers,
        principal=principal,
        scratch_dir=scratch_dir,
    )
    if body is None:
        return None
    if set(body) != {"principal", "key_fingerprint"}:
        raise RoleRegistryError(
            "the consented identity carries unexpected fields",
            Refusal(
                gate="identity.integrity",
                why="the consented bytes are not the canonical shape",
                legal_next="restore the body from backup, or re-mint the identity",
            ),
        )
    try:
        # Re-run the construction laws on the read-back (codex r3): a body that parses but
        # violates the grammars is not canonical, however it got signed.
        SovereignIdentity(body["principal"], body["key_fingerprint"])
    except ValueError as exc:
        raise RoleRegistryError(
            f"the consented identity violates the construction laws: {exc}",
            Refusal(
                gate="identity.integrity",
                why="parses but is not canonical",
                legal_next="restore the body from backup, or re-mint the identity",
            ),
        ) from None
    return body["principal"]


def role_known(
    root: Path,
    role: str,
    *,
    allowed_signers: Path | None = None,
    principal: str | None = None,
    scratch_dir: Path | None = None,
) -> bool:
    """Is this role registered? The fail-close must be LEGIBLE (the graph's whole point):
    an unregistered-but-plausible role and a malformed one are different answers."""
    if not ROLE_GRAMMAR.fullmatch(role):
        raise RoleRegistryError(
            f"{role!r} is not a role name — nothing to look up",
            Refusal(
                gate="role-registry.shape",
                why="a malformed role string is a caller bug, not an unregistered role",
                legal_next="role names are lowercase kebab, 2-32 chars",
            ),
        )
    body = _read_consented_body(
        root,
        "role-registry.",
        "role-registry.integrity",
        allowed_signers=allowed_signers,
        principal=principal,
        scratch_dir=scratch_dir,
    )
    if body is None:
        raise RoleRegistryError(
            "no role registry is minted — dispatch cannot fail-close legibly without one",
            Refusal(
                gate="role-registry.absent",
                why="the registry does not exist yet; every role is equally unknown",
                legal_next="mint_role_registry with the elicited role set",
            ),
        )
    if (
        set(body) != {"roles"}
        or not isinstance(body["roles"], list)
        or not body["roles"]
        or len(set(body["roles"])) != len(body["roles"])
        or not all(isinstance(r, str) and ROLE_GRAMMAR.fullmatch(r) for r in body["roles"])
    ):
        raise RoleRegistryError(
            "the consented registry is not the canonical shape (exactly {'roles': [valid names]})",
            Refusal(
                gate="role-registry.integrity",
                why="the consented bytes parse but are not a role registry (codex r1 major)",
                legal_next="restore the body from backup, or re-mint the registry",
            ),
        )
    return role in body["roles"]
