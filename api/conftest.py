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
    """The strings that identify THIS estate, or `None` when none were supplied.

    `None` means unsupplied — which is NOT `()`.

    `()` would mean "an estate with no identifying strings", and would make every scan pass. `None`
    means "we did not look", and callers must report that as an unchecked outcome.
    """
    declared = os.environ.get(TOKENS_ENV)
    if declared:
        path = pathlib.Path(declared)
        if not path.is_file():
            # THE PATH IS NOT ECHOED, only the variable that holds it.
            #
            # The operator supplied it and can read their own environment; a CI log, a pasted
            # traceback, or an issue comment cannot. A path under a home directory is a host
            # fingerprint by this module's own ladder, and leaving it here while removing it from
            # the skip reason would have been the same defect kept alive in a sibling branch --
            # which is exactly how it reached four channels before anyone counted them.
            raise RuntimeError(
                f"the file named by {TOKENS_ENV} does not exist (the path is not repeated here; "
                f"read the variable). A guard that is declared and absent is worse than one that "
                f"is undeclared: the estate believes it is being checked and it is not. Create the "
                f"file, correct {TOKENS_ENV}, or unset it to declare the check unarmed."
            )
    else:
        path = DEFAULT_TOKENS_FILE
        if not path.is_file():
            return None

    # Raised OUTSIDE the handler, for the reason given in _text_of: an OSError carries `.filename`,
    # and neither `from exc` nor `from None` stops it being reachable through the chain.
    body: str | None = None
    read_failure: str | None = None
    try:
        body = path.read_text(encoding="utf-8")
    except OSError as exc:
        read_failure = type(exc).__name__
    if body is None:
        raise RuntimeError(
            f"the tokens file named by {TOKENS_ENV} (or {TOKENS_FILE_NAME} at the repository root) "
            f"exists but could not be read ({read_failure}; the path is not repeated here). The "
            f"scan cannot run, and an unrunnable guard must not report a pass. Fix its "
            f"permissions, or remove it to declare the check unarmed."
        )

    tokens = tuple(
        line.strip()
        for line in body.splitlines()
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


#: Directories that are not the estate's source: caches, virtualenvs, vendored trees. Excluded by
#: NAME rather than by a depth limit, so the exclusion is legible and enumerable — the point of the
#: recursion is that nothing is skipped silently, and a skip list you can read is not a silent one.
_NOT_SOURCE = frozenset({"__pycache__", "node_modules", "site-packages", "build", "dist"})


def _readable_file(path: pathlib.Path, root: pathlib.Path) -> bool:
    """True for a plain file to scan. REFUSES on a symlinked directory instead of ignoring it.

    `rglob` does not descend directory symlinks, so anything behind one is outside the scan while
    the scan reports success — silence about a region it never entered, which is the shape this
    module exists to remove. Symlinked FILES are read normally: they resolve to bytes the tree
    genuinely contains.

    Refusing rather than following is the conservative choice: following would let a link out of
    the tree pull in an arbitrary amount of unrelated content, and could loop. The operator is told
    which entry to resolve or remove.
    """
    if path.is_symlink() and path.is_dir():
        raise RuntimeError(
            f"{path.relative_to(root)} is a symlink to a directory, and the scan does not descend "
            f"through it. Anything behind it would be unscanned while this reported success, and "
            f"unscanned is not clean. Replace it with a real directory, or move it out of the "
            f"scanned tree."
        )
    return path.is_file()


def _text_of(path: pathlib.Path) -> str:
    """File contents, or "" if the file is NOT TEXT. Unreadable is a different answer entirely.

    THESE TWO CASES WERE THE SAME LINE AND MUST NOT BE.

      UnicodeDecodeError   the bytes are not text, so they cannot contain a token AS TEXT.
                           "" is the honest answer, and a guard that crashed on the first binary
                           asset would stop running — a stopped guard reports nothing at all.
      OSError              WE COULD NOT LOOK. Permissions, a broken symlink, a vanished file.
                           Returning "" here says "scanned, clean" about a file nobody read, which
                           is the exact absence-into-zero defect this module exists to remove,
                           committed inside the module's own helper.

    So the second one fails closed. A source tree the scan cannot read is exceptional and should
    stop the guard loudly rather than be absorbed into a green run. The FILENAME is named, never
    the path, for the same reason every other message here names no path.
    """
    # The raise happens OUTSIDE the handler. `from None` suppresses only the DISPLAY: __context__
    # would still reference the OSError, which carries `.filename` — the protected path, reachable
    # again. Exactly the trap the regex validator fell into twice, and this file's own comment
    # described it while the first draft here did it anyway.
    failure: str | None = None
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return ""
    except OSError as exc:
        failure = type(exc).__name__
    raise RuntimeError(
        f"could not read {path.name} while scanning for estate fingerprints "
        f"({failure}; the path is not repeated here). A file that was not read is NOT a file that "
        f"is clean, so this refuses rather than reporting a pass over it. Make it readable, or "
        f"remove it from the scanned tree."
    )


def _skipped(relative: pathlib.Path) -> bool:
    """True for caches and vendored trees. DOTFILES ARE NOT SKIPPED — only dot-DIRECTORIES.

    An earlier version tested `part.startswith(".")` over every part including the filename, so a
    dotfile shipped in the package was invisible to the scan. Dotfiles are exactly where secrets
    tend to live, which makes that the worst possible place to be blind.
    """
    return any(
        part in _NOT_SOURCE or part.startswith(".") for part in relative.parts[:-1]
    ) or relative.parts[-1] in _NOT_SOURCE


def scan_tree_for_tokens(
    directory: pathlib.Path, tokens: tuple[str, ...], *, suffix: str = ""
) -> list[tuple[pathlib.Path, int]]:
    """Every (file, token-index) pair in `directory`. Exempts no SOURCE file, including its own.

    The claim is deliberately narrower than "excludes nothing", which is what this said before a
    reviewer pointed out that it is not true: caches, virtualenvs and vendored trees ARE skipped,
    by the enumerable `_NOT_SOURCE` list. Over-claiming in a docstring is the same defect as
    over-claiming in a guarantee, and this module has corrected three of those already.

    What it never does is exempt a file it was asked to read. The version this replaces skipped the
    file it lived in, which is exactly where the tokens were — so a scan that excludes any source
    file by name is a scan that cannot see its own denylist.

    Factored out so it can be tested against a temporary tree with SYNTHETIC tokens. Testing it
    only against the real package would mean the regression witness depended on a gitignored file,
    and would vanish into a skip in any clone that does not have it.

    IT SCANS EVERY FILE, NOT ONLY PYTHON. The default suffix is empty on purpose: a fingerprint in
    a README, a YAML fixture or a TOML file inside an exportable package is exactly as published as
    one in a module. Scanning `*.py` while the exit predicate said "every file" was a narrower
    guarantee than the words around it, which two reviewers caught. Undecodable bytes are treated
    as empty rather than raising, so one binary asset cannot take the whole guard down.

    IT RECURSES. A non-recursive glob would leave any sub-package silently out of scope: the
    package has none today, so the scan passed and would have kept passing the day someone added
    one. That is the by-name exemption again wearing a different hat — an exemption by DEPTH — and
    the rule is the same, that a scan which cannot see part of what it claims to cover is a scan
    whose green means less than it appears to.

    THE TOKEN IS NOT RETURNED — ITS INDEX IS, AND THE PATH IS RELATIVE.

    A returned token lands in a pytest assertion's rendered repr, a CI log, a pasted failure. So
    the report saying "an estate fingerprint is present here" would itself publish the fingerprint.
    That is the same defect as a Finding that stores its pattern, and it was found the same way: by
    a reviewer reading what the failure path would actually print rather than what its message
    claimed. The caller supplied the list and can map an index back; a log reader gets an integer.

    Paths are relative to `directory` for the same reason: an absolute path under a home directory
    is a host fingerprint, and these land in failing-assertion output.
    """
    return [
        (path.relative_to(directory), index)
        for path in sorted(directory.rglob(f"*{suffix}"))
        if _readable_file(path, directory) and not _skipped(path.relative_to(directory))
        for index, token in enumerate(tokens)
        if token in _text_of(path)
    ]
