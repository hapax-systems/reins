"""Test-time access to ESTATE DATA that must never be committed to an exportable repository.

## Why this exists

`test_k0.py` guarded the kernel's estate-independence with a denylist written inline in the
source — a tuple of literal strings naming this estate's home path, its host nicknames, and the
operator's referent — and asserted none of them appeared in the package.

That guard was correct in intent and self-defeating in form. **A denylist names the things it
forbids.** So the check that kept the estate out of the exportable kernel WAS an estate
fingerprint, published on a PUBLIC repository, sitting in the one file the scan skipped over
(`if p.name != "test_k0.py"`). It could not have caught itself.

This docstring does not quote that tuple. The first draft of this file did, which reintroduced the
whole defect inside the explanation of why not to — a fix for a fail-quiet defect is a likely
source of the next one, and prose is not exempt from the rule it describes.

This is the same split R0.10 draws for the disclosure guard, and it is drawn here for the same
reason: **the guard is law and ships; what it matches is estate data and does not.**

## How an estate arms it

Write one token per line to `.estate-tokens` at the repository root (gitignored, same class as
`config.toml` — "instance config, never commit real paths/secrets"), or point
`REINS_ESTATE_TOKENS_FILE` at a file elsewhere. Blank lines and `#` comments are ignored.

## What happens when it is not armed

The scan does not run, and it says so loudly — a skip, never a pass. A stranger cloning this repo
has their own estate and none of our tokens, so the check is meaningless for them; what would NOT
be acceptable is reporting "estate-independent: PASSED" after looking for nothing. That is the
absence-into-zero defect this whole kernel exists to remove.

A declared-but-missing file is a hard ERROR, not a skip. An estate that says where its tokens live
and is wrong has a guard it believes is running; that is worse than no guard at all.
"""

from __future__ import annotations

import os
import pathlib

DEFAULT_TOKENS_FILE = pathlib.Path(__file__).resolve().parent.parent / ".estate-tokens"
TOKENS_ENV = "REINS_ESTATE_TOKENS_FILE"

#: Repo-relative on purpose: this string is a pytest SKIP REASON, so it is printed into the CI log
#: of a PUBLIC repository on every unarmed run. An earlier version interpolated the resolved
#: absolute path of the tokens file — which begins with the operator's home directory, and is
#: therefore an estate fingerprint. The guard's own diagnostic was publishing the thing the guard
#: exists to keep unpublished, which is this defect's fourth appearance in this change alone:
#: inline denylist, stored pattern, returned token, and now the skip reason. The lesson is not
#: about any one of them. Every artifact a guard emits is a candidate disclosure channel.
TOKENS_FILE_NAME = ".estate-tokens"

#: Told to the operator when the scan cannot run, so "skipped" is never mistaken for "clean".
UNARMED = (
    f"estate-independence NOT CHECKED: no estate tokens supplied. This is not a pass — nothing was "
    f"scanned. To arm it, write one token per line to {TOKENS_FILE_NAME} at the repository root "
    f"(gitignored) or set {TOKENS_ENV} to a file elsewhere. A stranger's clone has no tokens of "
    f"ours and may leave this unarmed."
)


def estate_tokens() -> tuple[str, ...] | None:
    """The strings that identify THIS estate. `None` means unsupplied — which is not `()`.

    `()` would mean "an estate with no identifying strings", and would make every scan pass. `None`
    means "we did not look", and callers must report that as an unchecked outcome.
    """
    declared = os.environ.get(TOKENS_ENV)
    if declared:
        path = pathlib.Path(declared)
        if not path.is_file():
            raise RuntimeError(
                f"{TOKENS_ENV}={declared!r} names a file that does not exist. A guard that is "
                f"declared and absent is worse than one that is undeclared: the estate believes it "
                f"is being checked and it is not. Create the file or unset {TOKENS_ENV}."
            )
    else:
        path = DEFAULT_TOKENS_FILE
        if not path.is_file():
            return None

    tokens = tuple(
        line.strip()
        for line in path.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.lstrip().startswith("#")
    )
    if not tokens:
        raise RuntimeError(
            f"the tokens file ({path.name}) exists but contains no tokens. An empty denylist "
            f"passes everything, so an "
            f"empty file would silently disarm the scan while looking armed. Remove it to declare "
            f"the check unarmed, or write the tokens."
        )
    return tokens


#: Substrate vocabulary K0 must not carry. NOT secrets -- a config key and a public repository
#: name -- so they are written inline, unlike the estate fingerprints. They live HERE, outside the
#: k0 package, for the same structural reason the fingerprints live outside the tree: a guard's
#: data must not sit in what the guard scans, or the guard needs an exemption to avoid finding
#: itself, and that exemption is the original defect.
SUBSTRATE_TOKENS: tuple[str, ...] = ("council_root", "hapax-council")


def scan_tree_for_tokens(
    directory: pathlib.Path, tokens: tuple[str, ...], *, suffix: str = ".py"
) -> list[tuple[pathlib.Path, int]]:
    """Every (file, token-index) pair in `directory`. EXCLUDES NOTHING, including its own file.

    The exclusion is the whole reason this is a function instead of a loop inside one test. The
    version this replaces skipped the file it lived in, which is exactly where the tokens were —
    so a scan that excludes any file by name is a scan that cannot see its own denylist.

    Factored out so it can be tested against a temporary tree with SYNTHETIC tokens. Testing it
    only against the real package would mean the regression witness depended on a gitignored file,
    and would vanish into a skip in any clone that does not have it.

    THE TOKEN IS NOT RETURNED — ITS INDEX IS.

    A returned token lands in a pytest assertion's rendered repr, a CI log, a pasted failure. So
    the report saying "an estate fingerprint is present here" would itself publish the fingerprint.
    That is the same defect as a Finding that stores its pattern, and it was found the same way: by
    a reviewer reading what the failure path would actually print rather than what its message
    claimed. The caller supplied the list and can map an index back; a log reader gets an integer.
    """
    return [
        (path, index)
        for path in sorted(directory.glob(f"*{suffix}"))
        for index, token in enumerate(tokens)
        if token in path.read_text(encoding="utf-8")
    ]
