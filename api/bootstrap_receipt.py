"""bootstrap-receipt-v1 — the first-init receipt spine (design doc §7; R0.5/R0.6/R0.7).

Append-only, hash-chained receipts for the first-init ratification ceremony. Engine code:
no estate paths baked in; the durable root and every identity value arrive as arguments.
Receipts carry refs, never secrets. ``may_authorize`` is pinned False (never-mint) and
``transmit_class`` is pinned local_only — a receipt can witness, it can never grant.
"""
from __future__ import annotations

import contextlib
import fcntl
import hashlib
import secrets
import json
import os
import re
from collections.abc import Iterator
from datetime import UTC, datetime
from enum import StrEnum
from pathlib import Path
from typing import Literal, Self

from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator

#: A payload ref carries a scheme; a bare token is a value, and values never enter receipts.
_REF_GRAMMAR = re.compile(r"^[a-z][a-z0-9._-]*:")

BOOTSTRAP_RECEIPT_SCHEMA = 1
RECEIPT_CHAIN_FILENAME = "bootstrap-receipts.jsonl"
LOCK_FILENAME = "bootstrap.lock"
#: filesystem types that cannot hold the genesis chain (MDLC Stage-0 durable-media lesson).
_VOLATILE_FSTYPES = frozenset({"tmpfs", "ramfs", "devtmpfs", "overlay"})
#: sentinel for "could not determine" — treated as DENY, never as durable (fail-closed).
_UNKNOWN_FSTYPE = "unknown"


class BootstrapPhase(StrEnum):
    """Ceremony phases, in ladder order (phase-legality law carried as data)."""

    K0_ACTIVE = "K0_ACTIVE"
    HOST_RECONCILE = "HOST_RECONCILE"
    STIPULATION_RATIFY = "STIPULATION_RATIFY"
    SURFACE_OBSERVE = "SURFACE_OBSERVE"
    AUTH_MATERIALIZE = "AUTH_MATERIALIZE"
    MEASURED_PROBE = "MEASURED_PROBE"
    CAPABILITY_MINT = "CAPABILITY_MINT"
    SDLC_GATE_SHADOW = "SDLC_GATE_SHADOW"
    ENFORCE_FLIP = "ENFORCE_FLIP"
    KERNEL_DEMOTE = "KERNEL_DEMOTE"
    COMPLETE = "COMPLETE"


#: Ladder-as-data: index = the earliest position a phase may occupy relative to the others.
#: SURFACE_OBSERVE is deliberately BEFORE AUTH_MATERIALIZE (auth-free observation is legal);
#: MEASURED_PROBE (the first model call) is AFTER it — the trust-chain well-ordering theorem.
#:
#: HARDENING (R0.4/R0.6): the ladder is written out explicitly rather than derived from
#: ``tuple(BootstrapPhase)``. Deriving it made the phase-legality LAW an artifact of the
#: order the enum members happen to appear in the source: any reordering — an alphabetising
#: refactor, a merge — silently redefined the law with no test failing. The law is data here,
#: and LADDER_DIGEST drift-pins it so a change must be made deliberately.
PHASE_LADDER: tuple[BootstrapPhase, ...] = (
    BootstrapPhase.K0_ACTIVE,
    BootstrapPhase.HOST_RECONCILE,
    BootstrapPhase.STIPULATION_RATIFY,
    BootstrapPhase.SURFACE_OBSERVE,
    BootstrapPhase.AUTH_MATERIALIZE,
    BootstrapPhase.MEASURED_PROBE,
    BootstrapPhase.CAPABILITY_MINT,
    BootstrapPhase.SDLC_GATE_SHADOW,
    BootstrapPhase.ENFORCE_FLIP,
    BootstrapPhase.KERNEL_DEMOTE,
    BootstrapPhase.COMPLETE,
)

#: sha256 over the ladder as declared. Changing the ceremony's phase law must be a deliberate
#: act that updates this pin (CEILING_DENOTATION drift-pin precedent).
LADDER_DIGEST = "d8416cb35335fc0802bda019757f990874f1e0556dbcaf8ca7b158de40a878e1"


def ladder_digest() -> str:
    """Digest of the declared phase ladder (drift-pin input)."""
    return hashlib.sha256("\n".join(p.value for p in PHASE_LADDER).encode("utf-8")).hexdigest()


def assert_ladder_undrifted() -> None:
    """Fail-closed: the declared ladder must still cover the enum and match its pin."""
    missing = set(BootstrapPhase) - set(PHASE_LADDER)
    if missing:
        raise ValueError(
            f"phase ladder does not cover {sorted(p.value for p in missing)}: "
            "every phase must be placed in the law explicitly"
        )
    if len(PHASE_LADDER) != len(set(PHASE_LADDER)):
        raise ValueError("phase ladder contains duplicates")
    actual = ladder_digest()
    if actual != LADDER_DIGEST:
        raise ValueError(
            f"phase-legality law drifted: ladder digest {actual} != pinned {LADDER_DIGEST}; "
            "update LADDER_DIGEST deliberately if the ceremony's phase law really changed"
        )


class BootstrapAct(StrEnum):
    ELICITED = "elicited"
    RATIFIED = "ratified"
    REFUSED = "refused"
    PROBED = "probed"
    MINTED = "minted"
    RECONCILED = "reconciled"
    FLIPPED = "flipped"
    HELD = "held"
    ESCAPED = "escaped"


class EvidenceStatus(StrEnum):
    """PlatformCapabilityReceipt vocabulary, incl. the honest absence states."""

    OBSERVED = "observed"
    ASSERTED = "asserted"
    MISSING = "missing"
    UNOBSERVABLE = "unobservable"


class BootstrapReceipt(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    receipt_schema: Literal[1] = BOOTSTRAP_RECEIPT_SCHEMA
    receipt_id: str
    estate_id: str  # locally minted, non-PII
    kernel_version: str
    registry_schema_versions: dict[str, int] = Field(default_factory=dict)
    phase: BootstrapPhase
    act: BootstrapAct
    payload_refs: list[str] = Field(default_factory=list)  # refs, never secret values
    evidence_status: EvidenceStatus = EvidenceStatus.OBSERVED
    operator_ratification: str | None = None  # ptr to the ratifier-key signature record
    prev_receipt_hash: str | None = None  # None ONLY for the genesis self-attest
    observed_at: datetime
    stale_after: datetime | None = None
    may_authorize: Literal[False] = False  # never-mint: a receipt witnesses, it never grants
    transmit_class: Literal["local_only"] = "local_only"

    @model_validator(mode="after")
    def _genesis_shape(self) -> Self:
        if self.prev_receipt_hash is None and self.phase is not BootstrapPhase.K0_ACTIVE:
            raise ValueError(
                f"receipt {self.receipt_id!r}: only the K0_ACTIVE genesis self-attest may "
                "omit prev_receipt_hash"
            )
        return self

    @model_validator(mode="after")
    def _payload_refs_are_refs(self) -> Self:
        """HARDENING: "refs, never secrets" was a COMMENT on an unvalidated list[str].

        Measured: a receipt with payload_refs=["sk-ant-...", "password=hunter2"] was accepted.
        A ref must carry a scheme (``kernel-manifest:sha256:...``), which is a structural
        control rather than a semantic one — it cannot detect every secret, but it refuses the
        bare-token shapes that credentials actually take."""
        for ref in self.payload_refs:
            if not _REF_GRAMMAR.match(ref):
                raise ValueError(
                    f"receipt {self.receipt_id!r}: payload_ref {ref[:12]!r}… is not a ref. "
                    "Refs carry a scheme (e.g. 'kernel-manifest:sha256:…'); receipts carry "
                    "references, never values."
                )
        return self

    @model_validator(mode="after")
    def _timestamps_are_absolute(self) -> Self:
        """A naive datetime is not a moment. It reads like one, compares like one, and means a
        different instant on every host that loads it -- so a chain carrying naive timestamps
        orders differently depending on who reads it, and ratifications verified at observed_at
        (see ratifier.verify_time) land at the wrong instant. Absolute or refused."""
        for name in ("observed_at", "stale_after"):
            value = getattr(self, name)
            if value is not None and value.tzinfo is None:
                raise ValueError(
                    f"receipt {self.receipt_id!r}: {name} is timezone-naive. A receipt records WHEN "
                    "something happened; without an offset that is unanswerable."
                )
        return self

    @model_validator(mode="after")
    def _freshness_coherent(self) -> Self:
        """HARDENING (R1.3): a receipt may not expire before it was observed.

        Freshness and TTL semantics presuppose a sane clock; the chain gives ordering, never
        duration. An incoherent pair silently poisons every downstream freshness check."""
        if self.stale_after is not None and self.stale_after < self.observed_at:
            raise ValueError(
                f"receipt {self.receipt_id!r}: stale_after {self.stale_after.isoformat()} "
                f"precedes observed_at {self.observed_at.isoformat()} (clock-sanity violation)"
            )
        return self

    def receipt_hash(self) -> str:
        payload = self.model_dump(mode="json")
        canonical = json.dumps(payload, sort_keys=True, separators=(",", ":"))
        return hashlib.sha256(canonical.encode("utf-8")).hexdigest()


def genesis_self_attest(
    *,
    estate_id: str,
    kernel_version: str,
    kernel_manifest_sha256: str,
    observed_at: datetime | None = None,
) -> BootstrapReceipt:
    """The first chain link: the kernel attests its own gate set (hash + version)."""
    digest = kernel_manifest_sha256.strip().lower()
    if len(digest) != 64 or any(c not in "0123456789abcdef" for c in digest):
        raise ValueError(
            f"kernel_manifest_sha256 {kernel_manifest_sha256!r} is not a sha256 digest. The genesis\n"
            "receipt is the chain's root claim about WHICH kernel ran; an unparseable digest makes\n"
            "every drift check downstream compare against nothing."
        )
    return BootstrapReceipt(
        receipt_id=f"genesis-{estate_id}",
        estate_id=estate_id,
        kernel_version=kernel_version,
        phase=BootstrapPhase.K0_ACTIVE,
        act=BootstrapAct.MINTED,
        payload_refs=[f"kernel-manifest:sha256:{digest}"],
        prev_receipt_hash=None,
        observed_at=observed_at or datetime.now(UTC),
    )


class ChainVerdict(BaseModel):
    model_config = ConfigDict(extra="forbid")

    ok: bool
    length: int
    errors: list[str] = Field(default_factory=list)


def _phase_legal(seen: list[BootstrapPhase], nxt: BootstrapPhase) -> str | None:
    """Phase-legality law (data-driven). Returns an error string or None.

    Phases may repeat and may only move forward on the ladder, with two well-ordering
    guards on top: MEASURED_PROBE (the first model call) requires AUTH_MATERIALIZE
    earlier in the chain, and KERNEL_DEMOTE requires ENFORCE_FLIP.
    """
    if seen:
        current = seen[-1]
        if PHASE_LADDER.index(nxt) < PHASE_LADDER.index(current):
            return f"phase regression: {current.value} -> {nxt.value}"
        if current is BootstrapPhase.COMPLETE:
            return "receipt after COMPLETE: the chain is terminal"
    if nxt is BootstrapPhase.MEASURED_PROBE and BootstrapPhase.AUTH_MATERIALIZE not in seen:
        return "MEASURED_PROBE before AUTH_MATERIALIZE violates the model-call well-ordering"
    if nxt is BootstrapPhase.KERNEL_DEMOTE and BootstrapPhase.ENFORCE_FLIP not in seen:
        return "KERNEL_DEMOTE before ENFORCE_FLIP breaks the no-gap handoff"
    return None


#: Milestones a chain must actually contain before it may claim COMPLETE.
#: HARDENING: verify_chain previously accepted [K0_ACTIVE, COMPLETE] as ok=True — a two-link
#: chain certifying a ceremony in which nothing was ratified, nothing was minted, and no
#: enforce-flip occurred. The phase ladder forbids going BACKWARD but said nothing about
#: skipping forward, so the verifier certified an empty ceremony as a complete one. For a
#: product whose proposition is "the receipts prove it", a false green here is the deepest
#: possible defect.
REQUIRED_BEFORE_COMPLETE: tuple[BootstrapPhase, ...] = (
    BootstrapPhase.STIPULATION_RATIFY,  # the ceremony's own act
    BootstrapPhase.CAPABILITY_MINT,     # F7: nothing is supply until measured
    BootstrapPhase.ENFORCE_FLIP,        # the handoff
    BootstrapPhase.KERNEL_DEMOTE,       # the no-gap proof's second half
)


def verify_chain(receipts: list[BootstrapReceipt]) -> ChainVerdict:
    """Fail-closed chain verification: hash links, genesis shape, phase legality,
    duplicate receipt ids. A prefix of a valid chain verifies (resume-by-projection)."""
    errors: list[str] = []
    if not receipts:
        return ChainVerdict(ok=False, length=0, errors=["empty chain: no genesis self-attest"])
    genesis = receipts[0]
    if genesis.prev_receipt_hash is not None or genesis.phase is not BootstrapPhase.K0_ACTIVE:
        errors.append("chain does not begin with the K0_ACTIVE genesis self-attest")
    seen_ids: set[str] = set()
    seen_phases: list[BootstrapPhase] = []
    prev: BootstrapReceipt | None = None
    for index, receipt in enumerate(receipts):
        if receipt.receipt_id in seen_ids:
            errors.append(f"[{index}] duplicate receipt_id {receipt.receipt_id!r}")
        seen_ids.add(receipt.receipt_id)
        if prev is not None and receipt.prev_receipt_hash != prev.receipt_hash():
            errors.append(f"[{index}] hash-chain break at {receipt.receipt_id!r}")
        phase_error = _phase_legal(seen_phases, receipt.phase)
        if phase_error is not None:
            errors.append(f"[{index}] {phase_error}")
        seen_phases.append(receipt.phase)
        if receipt.estate_id != receipts[0].estate_id:
            # Two estates spliced into one chain would let a receipt from elsewhere inherit this
            # chain's hash-linkage and read as locally attested.
            errors.append(
                f"[{index}] estate_id {receipt.estate_id!r} does not match the chain's "
                f"{receipts[0].estate_id!r} — one chain belongs to exactly one estate"
            )
        prev = receipt

    if BootstrapPhase.COMPLETE in seen_phases:
        missing = [p.value for p in REQUIRED_BEFORE_COMPLETE if p not in seen_phases]
        if missing:
            errors.append(
                "chain claims COMPLETE without evidencing the ceremony: missing "
                f"{missing}. A complete first-init must have ratified, minted, flipped and "
                "demoted; a chain that skips them certifies that nothing happened."
            )
    return ChainVerdict(ok=not errors, length=len(receipts), errors=errors)


class DurableRootError(RuntimeError):
    pass


def _mount_fstype(path: Path) -> str:
    """fstype of the mount containing ``path`` (longest-prefix match over /proc/mounts)."""
    resolved = path.resolve()
    best: tuple[int, str] = (-1, _UNKNOWN_FSTYPE)
    try:
        mounts = Path("/proc/mounts").read_text(encoding="utf-8").splitlines()
    except OSError:
        return _UNKNOWN_FSTYPE
    for line in mounts:
        parts = line.split()
        if len(parts) < 3:
            continue
        mount_point, fstype = parts[1], parts[2]
        try:
            if resolved.is_relative_to(mount_point) and len(mount_point) > best[0]:
                best = (len(mount_point), fstype)
        except ValueError:
            continue
    return best[1]


def declare_durable_root(root: Path) -> dict[str, str]:
    """Pre-K0 install-time fact: the receipt chain must land durable from the FIRST write.

    Fail-closed on BOTH arms: a volatile filesystem raises, AND an filesystem we could not
    determine raises.

    HARDENING (K0.1 applied to K0.7's sibling): the previous version returned the literal
    string "unknown" when /proc/mounts was unreadable or matched nothing, and "unknown" is not
    in _VOLATILE_FSTYPES — so the guard PASSED whenever it could not evaluate. Measured: with
    /proc/mounts unreadable, declare_durable_root() accepted /dev/shm. A durability guard that
    fails open is worse than none, because it reports a durable root that is not one. The
    kernel's first law is "unevaluable predicate ⇒ DENY"; this member was violating it.
    """
    fstype = _mount_fstype(root)
    if fstype == _UNKNOWN_FSTYPE:
        raise DurableRootError(
            f"durable root {root}: could not determine the filesystem (is /proc/mounts "
            "readable?). An unevaluable durability predicate DENIES — declare a root whose "
            "filesystem can be observed."
        )
    if fstype in _VOLATILE_FSTYPES:
        raise DurableRootError(
            f"durable root {root} sits on volatile filesystem {fstype!r}; "
            "the genesis receipt must land on durable media (declare a different root)"
        )
    return {"root": str(root.resolve()), "fstype": fstype}


class BootstrapLockError(RuntimeError):
    pass


class BootstrapLock:
    """Single-instance lock: two concurrent first-inits must not fork the chain.

    Exclusive-create semantics; fail-closed on any existing lock, including a stale one
    (a dead pid is reported, never silently stolen — takeover is an explicit human act)."""

    def __init__(self, root: Path) -> None:
        self._path = root / LOCK_FILENAME
        self._held = False
        self._token: str | None = None

    @property
    def path(self) -> Path:
        return self._path

    def acquire(self) -> None:
        self._path.parent.mkdir(parents=True, exist_ok=True)
        try:
            fd = os.open(self._path, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
        except FileExistsError:
            holder = self._path.read_text(encoding="utf-8").strip() or "unknown"
            raise BootstrapLockError(
                f"bootstrap lock held ({holder}) at {self._path}; a concurrent or "
                "interrupted first-init owns the chain — inspect, then remove the lock "
                "explicitly to take over"
            ) from None
        # pid + timestamp is NOT unique: pids are reused, and across pid namespaces two live
        # processes can share one. release() treats an equal token as proof of ownership, so a
        # collision lets the wrong process delete a lock it does not hold. A random nonce makes
        # the token unforgeable by coincidence.
        token = f"pid:{os.getpid()} at:{datetime.now(UTC).isoformat()} nonce:{secrets.token_hex(8)}"
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(token)
        self._held = True
        self._token = token

    def release(self) -> None:
        """Release only a lock we still own.

        HARDENING: the previous release() unlinked unconditionally. After an explicit human
        takeover (remove the lock, start a new first-init) the crashed-out holder's release
        would delete the NEW holder's lock, silently readmitting concurrency at exactly the
        moment the operator had intervened to prevent it."""
        if not self._held:
            return
        self._held = False
        try:
            current = self._path.read_text(encoding="utf-8")
        except OSError:
            self._token = None
            return
        if current == self._token:
            self._path.unlink(missing_ok=True)
        self._token = None

    def __enter__(self) -> BootstrapLock:
        self.acquire()
        return self

    def __exit__(self, *_exc: object) -> None:
        self.release()


def _fsync_dir(directory: Path) -> None:
    """fsync the DIRECTORY so the chain file's entry itself survives a crash.

    HARDENING (R0.7): fsyncing only the file leaves the very first write — the creation of
    bootstrap-receipts.jsonl — recoverable-as-absent after power loss. The genesis receipt is
    the one receipt with no predecessor to prove it existed."""
    fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


@contextlib.contextmanager
def _exclusive_chain(root: Path) -> Iterator[int]:
    """Open the chain O_NOFOLLOW and hold an exclusive advisory lock across the whole
    read-modify-write.

    HARDENING (R0.6): append_receipt previously did load_chain() -> open("a"), an unguarded
    TOCTOU window. BootstrapLock existed but nothing on the write path consulted it, so it
    could not prevent what it was built to prevent. Measured before this change: 60/60 trials
    with 4 concurrent writers admitted every writer and produced a chain that verify_chain
    then rejected — and because the chain is append-only, that corruption is terminal.

    O_NOFOLLOW additionally refuses a chain path that has been replaced by a symlink."""
    root.mkdir(parents=True, exist_ok=True)
    chain_path = root / RECEIPT_CHAIN_FILENAME
    fd = os.open(chain_path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW | os.O_APPEND, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)
        yield fd
    finally:
        os.close(fd)


def append_receipt(root: Path, receipt: BootstrapReceipt) -> Path:
    """Append to the JSONL chain, enforcing the hash link and id uniqueness against the
    stored tail (fail-closed: a mismatched prev hash or duplicate id refuses the write).

    The validate-then-write sequence runs under an exclusive lock on the chain file, so
    concurrent first-inits serialise and the losers are refused rather than forking it."""
    chain_path = root / RECEIPT_CHAIN_FILENAME
    with _exclusive_chain(root) as fd:
        existing = _load_chain_fd(fd)
        if existing:
            tail_hash = existing[-1].receipt_hash()
            if receipt.prev_receipt_hash != tail_hash:
                raise ValueError(
                    f"append refused: prev_receipt_hash does not match the stored tail "
                    f"({receipt.receipt_id!r})"
                )
            if receipt.receipt_id in {r.receipt_id for r in existing}:
                raise ValueError(f"append refused: duplicate receipt_id {receipt.receipt_id!r}")
        elif receipt.prev_receipt_hash is not None:
            raise ValueError(
                "append refused: an empty chain accepts only the genesis self-attest"
            )
        line = json.dumps(receipt.model_dump(mode="json"), sort_keys=True) + "\n"
        # os.write may write FEWER bytes than asked. On an append-only ledger a short write is a
        # truncated JSON line -- the chain would fail to parse forever after, with no way to tell
        # a torn write from tampering. Write to completion or raise.
        _write_all(fd, line.encode("utf-8"))
        os.fsync(fd)
    _fsync_dir(chain_path.parent)
    return chain_path


def _write_all(fd: int, data: bytes) -> None:
    """Write every byte or raise. A partial append is indistinguishable from tampering later."""
    written = 0
    while written < len(data):
        n = os.write(fd, data[written:])
        if n <= 0:
            raise OSError(
                f"short write on the receipt chain: {written} of {len(data)} bytes. The chain is "
                "append-only and must not carry a truncated record."
            )
        written += n


def _load_chain_fd(fd: int) -> list[BootstrapReceipt]:
    """Read the chain from an already-open, already-locked descriptor."""
    os.lseek(fd, 0, os.SEEK_SET)
    chunks: list[bytes] = []
    while chunk := os.read(fd, 65536):
        chunks.append(chunk)
    try:
        text = b"".join(chunks).decode("utf-8")
    except UnicodeDecodeError as e:
        # The chain is written as UTF-8 JSON lines, so undecodable bytes mean the ledger itself is
        # damaged. Say that, rather than surfacing a codec error the caller must interpret.
        raise ValueError(
            f"receipt chain is corrupt: not valid UTF-8 at byte {e.start} ({e.reason}). "
            "The chain is append-only; a damaged chain is restored from backup, never repaired "
            "in place."
        ) from e
    return _parse_chain(text)


def _parse_chain(text: str) -> list[BootstrapReceipt]:
    return [
        BootstrapReceipt.model_validate(json.loads(line))
        for line in text.splitlines()
        if line.strip()
    ]


def verify_chain_at(root: Path) -> ChainVerdict:
    """Load and verify in one fail-closed step.

    HARDENING: load_chain() raises on a malformed line, so the fail-closed verifier could not
    report on the one thing it most needs to report on — a corrupted ledger. Callers got a
    traceback instead of ``ok=False``. Corruption is now a verdict."""
    try:
        receipts = load_chain(root)
    except (json.JSONDecodeError, ValidationError, OSError) as exc:
        return ChainVerdict(
            ok=False, length=0, errors=[f"chain at {root} is unreadable or corrupt: {exc}"]
        )
    return verify_chain(receipts)


def load_chain(root: Path) -> list[BootstrapReceipt]:
    """Read the chain under a shared lock, so a reader never observes a partial append."""
    chain_path = root / RECEIPT_CHAIN_FILENAME
    try:
        fd = os.open(chain_path, os.O_RDONLY | os.O_NOFOLLOW)
    except FileNotFoundError:
        return []
    try:
        fcntl.flock(fd, fcntl.LOCK_SH)
        return _load_chain_fd(fd)
    finally:
        os.close(fd)
