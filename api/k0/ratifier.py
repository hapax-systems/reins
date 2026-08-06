"""K0 — the ratifier key: binding a ratification to the sovereign. (R0.11, second clause)

ACCEPTED 2026-08-01: SSHSIG via OpenSSH `ssh-keygen -Y sign|verify`.

WHY THIS AND NOT A LIBRARY
--------------------------
The kernel adds ZERO Python dependencies by using it. K0 is stdlib-only and the api project
declares only fastapi and uvicorn; `cryptography` would be the largest dependency the kernel ever
acquired, for a package whose entire purpose is to be minimal and estate-independent.

It also rides the OS floor R1.4 already declares (shell, init, git, secret store) — git-over-ssh
means OpenSSH is present in practice. And critically, **a stranger can verify without our
software**: `ssh-keygen -Y verify` is a standard tool on any box. Verification that requires the
kernel is weak verification. git itself signs commits this way (gpg.format=ssh) since 2.34.

An HMAC was the tempting stdlib answer and is the wrong one: any key holder can forge a
ratification, so it cannot bind an act to a sovereign. A fake signature is worse than an absent
one, because it reads as binding.

NAMESPACE SEPARATION IS LOAD-BEARING
------------------------------------
Every signature is namespaced `hapax-ratification`. Measured: verifying a correct signature under
a different namespace fails with "namespace does not match". Without it, any SSHSIG the sovereign
ever produced for another purpose — a signed commit, a signed file — could be replayed as a
ratification. This is the property a hand-rolled scheme would omit.

THE SUBPROCESS BOUNDARY IS THE COST, AND IT IS CONFINED HERE
------------------------------------------------------------
K0 makes no other subprocess calls. This module is the single exception and it is deliberately the
only one. Measured exit-code contract for `-Y verify`:

    0    good signature
    255  EVERY failure path — tampered payload, wrong principal, wrong namespace,
         missing signers file, missing signature file

So exit 0 is the ONLY success, and anything else refuses. A verification that could not RUN
(binary absent, timeout) is UNEVALUABLE and therefore also refuses — the K0 law applied rather
than restated: this module calls `decide()` directly.

The payload is signed over stdin and never written to disk. A ratification payload may name the
thing being ratified, and temp files leak.

The declared floor is OpenSSH 9.1, not the 8.0 that SSHSIG alone would need: verify-time and
allowed_signers validity windows arrived in 8.7, and the UTC 'Z' suffix this module emits arrived in
9.1. See host_floor for the release-note citations.

KEY LOSS AND ROTATION are handled in recovery.py (R0.11's third clause). Note the coupling: once a
key is rotated its allowed_signers window closes, so verifying a HISTORICAL ratification requires
passing that receipt's own `observed_at` as `verify_time`. Verified at "now" a retired key looks
exactly like tampering.
"""

from __future__ import annotations

import shutil
import subprocess
from datetime import UTC, datetime
from pathlib import Path

from .fail_closed import Evaluation, decide
from .refusal import Refusal, RefusalError

#: Pinned. Changing it invalidates every prior ratification, so it changes only under enforce-flip.
RATIFICATION_NAMESPACE = "hapax-ratification"

#: ssh-keygen is not interactive here; anything slower than this is a hang, not slowness.
_TIMEOUT_S = 20

_LEGAL_NEXT = (
    "install OpenSSH >= 9.1 (see host_floor) and register the sovereign's public key in the allowed-signers file "
    "under the durable root"
)


class RatifierError(RuntimeError):
    """Raised when signing cannot be performed. Verification refuses instead (see verify)."""


def _ssh_keygen() -> str:
    path = shutil.which("ssh-keygen")
    if not path:
        raise RatifierError(
            "ssh-keygen not found. The ratifier key requires OpenSSH >= 9.1, which R1.4's "
            "supported-host matrix must declare explicitly."
        )
    return path


def sign_ratification(payload: bytes, key_path: Path) -> str:
    """Sign `payload` with the sovereign's private key. Returns the armored SSHSIG.

    Signing raises rather than refusing: an operator who cannot sign has hit a configuration
    fault, not a governed denial. Verification is the arm that must refuse.
    """
    if not key_path.is_file():
        raise RatifierError(f"ratifier key {key_path} is not a file")
    try:
        proc = subprocess.run(  # noqa: S603 — argv form, no shell, fixed binary
            [_ssh_keygen(), "-Y", "sign", "-f", str(key_path), "-n", RATIFICATION_NAMESPACE, "-"],
            input=payload,
            capture_output=True,
            timeout=_TIMEOUT_S,
            check=False,
        )
    except subprocess.TimeoutExpired as e:
        raise RatifierError(f"ssh-keygen timed out after {_TIMEOUT_S}s signing") from e
    if proc.returncode != 0:
        # stderr only — never echo the payload, which may name what is being ratified.
        raise RatifierError(
            f"ssh-keygen sign failed (exit {proc.returncode}): "
            f"{proc.stderr.decode('utf-8', 'replace').strip()[:200]}"
        )
    sig = proc.stdout.decode("utf-8")
    if "BEGIN SSH SIGNATURE" not in sig:
        raise RatifierError("ssh-keygen returned no signature")
    return sig


def verify_ratification(
    payload: bytes,
    signature: str,
    *,
    allowed_signers: Path,
    principal: str,
    scratch_dir: Path,
    verify_time: datetime | None = None,
) -> None:
    """Verify a ratification. Returns None if bound to `principal`; REFUSES otherwise.

    `scratch_dir` holds only the signature (already public); the payload goes over stdin.

    `verify_time` MUST be supplied when checking a HISTORICAL ratification — pass the receipt's
    own `observed_at`. ssh-keygen validates against the current time by default, so once a key is
    rotated (recovery.rotate closes its window with valid-before) every ratification it ever made
    fails to verify at "now". Verifying history at the present moment is the wrong question, and
    getting it wrong looks exactly like tampering.
    """
    sig_path = scratch_dir / "ratification.sig"
    try:
        sig_path.write_text(signature, encoding="utf-8")
    except OSError as e:
        raise RefusalError(
            Refusal(
                gate="ratifier-verify",
                why=f"could not stage the signature for verification: {e}",
                legal_next=_LEGAL_NEXT,
                teaches="doctrine/ratifier-key",
            )
        ) from e

    try:
        binary = _ssh_keygen()
    except RatifierError as e:
        # Cannot evaluate -> DENY. This is the arm a naive implementation turns into a pass.
        decide(
            "ratifier-verify",
            Evaluation.UNEVALUABLE,
            legal_next=_LEGAL_NEXT,
            unevaluable_why=str(e),
            teaches="doctrine/ratifier-key",
        )
        return  # pragma: no cover — decide() always raises on UNEVALUABLE

    try:
        proc = subprocess.run(  # noqa: S603 — argv form, no shell, fixed binary
            [
                binary, "-Y", "verify",
                "-f", str(allowed_signers),
                "-I", principal,
                "-n", RATIFICATION_NAMESPACE,
                "-s", str(sig_path),
                *(("-O", f"verify-time={verify_time.astimezone(UTC).strftime('%Y%m%d%H%M%SZ')}")
                  if verify_time is not None else ()),
            ],
            input=payload,
            capture_output=True,
            timeout=_TIMEOUT_S,
            check=False,
        )
    except subprocess.TimeoutExpired:
        decide(
            "ratifier-verify",
            Evaluation.UNEVALUABLE,
            legal_next=_LEGAL_NEXT,
            unevaluable_why=f"ssh-keygen timed out after {_TIMEOUT_S}s; the ratification is unverified",
            teaches="doctrine/ratifier-key",
        )
        return  # pragma: no cover

    # Exit 0 is the ONLY success. 255 covers tampered payload, wrong principal, wrong namespace,
    # and missing files alike — all of which are VIOLATED, none of which may pass.
    decide(
        "ratifier-verify",
        Evaluation.SATISFIED if proc.returncode == 0 else Evaluation.VIOLATED,
        legal_next=_LEGAL_NEXT,
        violated_why=(
            f"ratification not bound to {principal!r} under namespace "
            f"{RATIFICATION_NAMESPACE!r} (ssh-keygen exit {proc.returncode}): "
            f"{proc.stderr.decode('utf-8', 'replace').strip()[:160]}"
        ),
        teaches="doctrine/ratifier-key",
    )


def write_allowed_signers(path: Path, principal: str, public_key: str) -> None:
    """Write the principal -> key mapping verification reads.

    One line per principal, which is the shape multi-ratifier will need later without a rewrite.
    """
    if not principal.strip() or " " in principal.strip():
        raise RatifierError(f"principal {principal!r} must be non-empty and contain no spaces")
    key = public_key.strip()
    if not key.startswith(("ssh-", "sk-", "ecdsa-")):
        raise RatifierError("public_key does not look like an OpenSSH public key")
    path.write_text(f"{principal.strip()} {key}\n", encoding="utf-8")
