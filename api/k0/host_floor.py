"""The kernel's OS-dependency floor, declared as DATA. (R1.4)

R1.4 asks for the "kernel OS-dependency floor declared (shell, init, git, secret store)" and a
"ratified supported-platform stipulation". Until now the floor was ASSUMED: the ratifier key needs
OpenSSH >= 8.0 for `ssh-keygen -Y`, and that was inferred from git-over-ssh rather than stated. An
assumed dependency is one a stranger's fresh box discovers by failing.

DECLARED, NOT DISCOVERED. Each entry names the binary, why the kernel needs it, and the minimum
version where the required behaviour exists. `probe()` reports what is actually present;
`require()` applies the K0 law to the result.

THE VERSION MINIMUMS ARE CLAIMS ABOUT BEHAVIOUR, not preferences:
  * OpenSSH 8.0 (2019) — `ssh-keygen -Y sign|verify` (SSHSIG) first appears here. The ratifier key
    is unimplementable below it.
  * OpenSSH 8.2 (2020) — allowed_signers `valid-after`/`valid-before` and the `verify-time` option,
    which the key-rotation ceremony REQUIRES: without verify-time, retiring a key invalidates every
    ratification it ever made.

UNEVALUABLE DENIES, here as everywhere. A probe that cannot determine a version does not get to
assume it is new enough — the same arm that made declare_durable_root accept /dev/shm.
"""

from __future__ import annotations

import re
import shutil
import subprocess
from dataclasses import dataclass

from .fail_closed import Evaluation, decide

_TIMEOUT_S = 10


@dataclass(frozen=True)
class FloorEntry:
    """One declared OS dependency."""

    binary: str
    why: str
    #: (major, minor). None = any version present is acceptable.
    min_version: tuple[int, int] | None
    version_argv: tuple[str, ...]
    #: Where the version appears in the tool's output; both streams are searched.
    version_re: str
    #: Binary to ASK for the version, when it differs from the one required. ssh-keygen has no
    #: version flag of its own -- `ssh -V` reports the OpenSSH release both ship in.
    version_binary: str = ""


#: THE FLOOR. Adding an entry is widening what the kernel demands of a stranger's machine and
#: should be as reluctant as adding a K0 member.
FLOOR: tuple[FloorEntry, ...] = (
    FloorEntry(
        binary="ssh-keygen",
        why=(
            "the ratifier key binds ratifications to the sovereign via SSHSIG "
            "(ssh-keygen -Y sign|verify), and key rotation needs allowed_signers validity "
            "windows plus the verify-time option"
        ),
        min_version=(8, 2),
        version_argv=("-V",),
        version_re=r"OpenSSH[_ ](\d+)\.(\d+)",
        version_binary="ssh",
    ),
    FloorEntry(
        binary="git",
        why="delivery and governance rails; the kit is distributed and versioned through it",
        min_version=None,
        version_argv=("--version",),
        version_re=r"git version (\d+)\.(\d+)",
    ),
)


def _detect(entry: FloorEntry) -> tuple[int, int] | None:
    """Observed version, or None when it cannot be determined. None means UNEVALUABLE."""
    if not shutil.which(entry.binary):
        return None
    asker = entry.version_binary or entry.binary
    if not shutil.which(asker):
        return None
    try:
        proc = subprocess.run(  # noqa: S603 — argv form, no shell, declared binary
            [asker, *entry.version_argv],
            capture_output=True,
            timeout=_TIMEOUT_S,
            check=False,
        )
    except (subprocess.TimeoutExpired, OSError):
        return None
    # `ssh -V` writes to stderr; other tools to stdout. Search both rather than guess.
    blob = (proc.stdout + b"\n" + proc.stderr).decode("utf-8", "replace")
    m = re.search(entry.version_re, blob)
    if not m:
        return None
    return int(m.group(1)), int(m.group(2))


def probe() -> dict[str, tuple[int, int] | None]:
    """What is actually present. None = present-but-unreadable, or absent."""
    return {e.binary: _detect(e) for e in FLOOR}


def require() -> None:
    """Apply the K0 law to the floor. Refuses on the first entry that is missing, too old, or
    undeterminable. Returns None when every declared dependency is satisfied."""
    for entry in FLOOR:
        found = _detect(entry)
        if found is None:
            ev = Evaluation.UNEVALUABLE
        elif entry.min_version is None or found >= entry.min_version:
            ev = Evaluation.SATISFIED
        else:
            ev = Evaluation.VIOLATED

        want = (
            f">= {entry.min_version[0]}.{entry.min_version[1]}"
            if entry.min_version
            else "any version"
        )
        decide(
            f"host-floor:{entry.binary}",
            ev,
            legal_next=f"install {entry.binary} {want} and re-run the host reconcile",
            violated_why=(
                f"{entry.binary} {found[0]}.{found[1]} is below the declared floor {want} — "
                f"{entry.why}"
            ) if found else "",
            unevaluable_why=(
                f"{entry.binary} is absent, or its version could not be read. The kernel declares "
                f"it at {want} because {entry.why}. An undeterminable dependency DENIES rather "
                f"than being assumed new enough."
            ),
            teaches="doctrine/host-floor",
        )
