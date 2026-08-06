"""The estate-token loader and the tree scan, tested with SYNTHETIC tokens only.

## Why this file exists

The first version of this change proved its negative controls by hand, in a shell, against the real
gitignored token file. Two independent reviewers blocked on the same ground and they were right:
a witness that lives in someone's terminal is not a witness. In a clean clone there is no token
file, so the estate-independence test SKIPS — meaning CI could not distinguish "no fingerprints
present" from "never looked", which is the exact confusion this whole change exists to remove.

Everything here injects its own tokens into a temporary tree. Nothing depends on estate data, so
these run identically in a stranger's clone, and none of them can publish a real token.
"""

from __future__ import annotations

import pathlib

import pytest

from conftest import (
    TOKENS_ENV,
    TOKENS_FILE_NAME,
    UNARMED,
    estate_tokens,
    scan_tree_for_tokens,
)

#: Invented for these tests. Deliberately unlike anything real.
FAKE = "zzq-token-alpha"
FAKE_2 = "zzq-token-beta"


def _tokens_file(tmp_path: pathlib.Path, body: str) -> pathlib.Path:
    path = tmp_path / "tokens"
    path.write_text(body, encoding="utf-8")
    return path


# --- the loader ---------------------------------------------------------------------------
def test_a_declared_but_missing_file_is_an_error_not_a_skip(monkeypatch, tmp_path) -> None:
    """The worst of the three outcomes: a guard the estate BELIEVES is running.

    Skipping here would be silent — the estate declared where its tokens live, got the path wrong,
    and would see a green run forever.

    AND THE ERROR MUST NOT ECHO THE PATH. A path under a home directory is a host fingerprint by
    this module's own ladder, and this message reaches CI logs and pasted tracebacks. Naming the
    ENVIRONMENT VARIABLE is fully actionable — the operator set it and can read it — while
    disclosing nothing. Removing the absolute path from the skip reason and leaving it here would
    have been the same defect kept alive in a sibling branch, which is how it reached four separate
    channels before anyone counted them.
    """
    missing = tmp_path / "does-not-exist"
    monkeypatch.setenv(TOKENS_ENV, str(missing))
    with pytest.raises(RuntimeError, match="worse than one that is undeclared") as exc:
        estate_tokens()
    assert TOKENS_ENV in str(exc.value), "the operator must be told WHICH variable to fix"
    assert str(missing) not in str(exc.value), (
        "the error echoed the caller-supplied path; it must name only the variable"
    )
    assert str(tmp_path) not in str(exc.value), "a parent directory leaked through the message"


def test_an_empty_file_is_an_error_not_an_empty_denylist(monkeypatch, tmp_path) -> None:
    """∀-over-the-empty-set. An empty denylist passes every payload, vacuously."""
    monkeypatch.setenv(TOKENS_ENV, str(_tokens_file(tmp_path, "")))
    with pytest.raises(RuntimeError, match="empty denylist passes everything"):
        estate_tokens()


def test_a_comments_only_file_is_also_empty_and_also_errors(monkeypatch, tmp_path) -> None:
    """The same vacuity, reached by a route that LOOKS armed to a reader."""
    monkeypatch.setenv(TOKENS_ENV, str(_tokens_file(tmp_path, "# nothing here\n\n  \n")))
    with pytest.raises(RuntimeError, match="empty denylist passes everything"):
        estate_tokens()


def test_comments_and_blank_lines_are_ignored_but_tokens_are_not(monkeypatch, tmp_path) -> None:
    body = f"# a heading\n\n{FAKE}\n   \n  # indented comment\n{FAKE_2}\n"
    monkeypatch.setenv(TOKENS_ENV, str(_tokens_file(tmp_path, body)))
    assert estate_tokens() == (FAKE, FAKE_2)


def test_an_absent_default_file_returns_none_which_is_not_an_empty_tuple(
    monkeypatch, tmp_path
) -> None:
    """`None` means WE DID NOT LOOK. `()` would mean an estate with no identifying strings.

    Collapsing the two is how "unchecked" becomes "clean" — so the loader keeps them distinct and
    callers must handle `None` as an unchecked outcome rather than as a pass.
    """
    monkeypatch.delenv(TOKENS_ENV, raising=False)
    monkeypatch.setattr("conftest.DEFAULT_TOKENS_FILE", tmp_path / "absent")
    assert estate_tokens() is None


def test_the_default_file_is_read_when_the_environment_is_silent(monkeypatch, tmp_path) -> None:
    """THE PATH THIS ESTATE ACTUALLY TAKES, and it had no test.

    Every other loader test sets the environment variable, so the branch that reads
    DEFAULT_TOKENS_FILE was exercised only by the ambient real run — which SKIPS in any clone that
    is unarmed. So in CI the estate's own loading path was never executed at all, and a change
    breaking it would surface as a silent skip rather than a failure: the unarmed-is-not-clean
    confusion, one level up, in the code that implements it.
    """
    monkeypatch.delenv(TOKENS_ENV, raising=False)
    default = tmp_path / TOKENS_FILE_NAME
    default.write_text(f"# heading\n{FAKE}\n\n{FAKE_2}\n", encoding="utf-8")
    monkeypatch.setattr("conftest.DEFAULT_TOKENS_FILE", default)
    assert estate_tokens() == (FAKE, FAKE_2)


def test_the_environment_declaration_wins_over_the_default_file(monkeypatch, tmp_path) -> None:
    """Otherwise a stale default silently shadows the file the estate actually named."""
    monkeypatch.setattr("conftest.DEFAULT_TOKENS_FILE", _tokens_file(tmp_path, f"{FAKE_2}\n"))
    declared = tmp_path / "declared"
    declared.write_text(f"{FAKE}\n", encoding="utf-8")
    monkeypatch.setenv(TOKENS_ENV, str(declared))
    assert estate_tokens() == (FAKE,)


# --- the scan -----------------------------------------------------------------------------
def test_the_scan_finds_a_planted_token(tmp_path) -> None:
    (tmp_path / "clean.py").write_text("x = 1\n", encoding="utf-8")
    (tmp_path / "dirty.py").write_text(f"# {FAKE}\n", encoding="utf-8")
    hits = scan_tree_for_tokens(tmp_path, (FAKE,))
    assert [p.name for p, _ in hits] == ["dirty.py"]


def test_the_scan_returns_relative_paths_so_a_failure_cannot_publish_the_checkout(
    tmp_path,
) -> None:
    """An absolute path under a home directory is a host fingerprint.

    These land in the rendered operands of a failing assertion, so returning them would make the
    report disclose where the estate keeps its checkout — the same shape as returning the token
    itself, one field over. Relative to the directory the caller passed: the caller knows what they
    passed, and a log reader learns nothing.
    """
    (tmp_path / "dirty.py").write_text(f"# {FAKE}\n", encoding="utf-8")
    hits = scan_tree_for_tokens(tmp_path, (FAKE,))

    assert hits, "fixture precondition: the planted token must be found"
    for path, _ in hits:
        assert not path.is_absolute(), f"the scan returned an absolute path: {path.name}"
    assert str(tmp_path) not in repr(hits), "the containing directory leaked through the result"


def test_the_scan_excludes_no_file_by_name_including_the_scanners_own(tmp_path) -> None:
    """THE REGRESSION. This is the defect, reproduced against a synthetic tree.

    The scan this replaces skipped `test_k0.py` by name — and `test_k0.py` was where the estate's
    tokens were written. A scan that exempts any file cannot see its own denylist, so the exemption
    is the bug, not an optimisation. Here every file that a real package would contain is planted
    with a token, including one named exactly like the old exemption, and all of them must be found.
    """
    names = ("test_k0.py", "conftest.py", "refusal.py", "manifest.py")
    for name in names:
        (tmp_path / name).write_text(f"# {FAKE}\n", encoding="utf-8")
    found = {p.name for p, _ in scan_tree_for_tokens(tmp_path, (FAKE,))}
    assert found == set(names), (
        f"the scan skipped {sorted(set(names) - found)}. Any by-name exemption reintroduces the "
        f"original defect: the file most likely to contain the denylist is the file that holds it."
    )


def test_the_scan_reports_every_token_not_merely_the_first(tmp_path) -> None:
    """A scan that stops at the first hit under-reports, and the operator fixes one of three."""
    (tmp_path / "a.py").write_text(f"# {FAKE}\n# {FAKE_2}\n", encoding="utf-8")
    assert {i for _, i in scan_tree_for_tokens(tmp_path, (FAKE, FAKE_2))} == {0, 1}


def test_the_scan_returns_indices_so_a_failure_report_cannot_publish_the_token() -> None:
    """THE REPORT MUST NOT BE THE LEAK.

    pytest renders the operands of a failing assertion, so a scan returning the matched TOKEN would
    print the estate's fingerprints into the CI log of the very run that found them — the report
    becoming the disclosure, which is the defect this whole change exists to remove, arriving by a
    third route after the inline denylist and the stored pattern.
    """
    import tempfile

    token = "zzq-fingerprint-value"
    with tempfile.TemporaryDirectory() as tmp:
        root = pathlib.Path(tmp)
        (root / "leaky.py").write_text(f"# {token}\n", encoding="utf-8")
        hits = scan_tree_for_tokens(root, (token,))

    assert hits, "fixture precondition: the planted token must be found"
    assert token not in repr(hits), (
        "the scan returned the token it found. pytest renders a failing assertion's operands, so "
        "this would print the estate's fingerprints into the log of the run that detected them."
    )
    assert [i for _, i in hits] == [0], "the index must identify WHICH token, for the caller"


def test_a_clean_tree_yields_nothing(tmp_path) -> None:
    (tmp_path / "a.py").write_text("x = 1\n", encoding="utf-8")
    assert scan_tree_for_tokens(tmp_path, (FAKE,)) == []


def test_the_scan_reads_every_file_by_default_and_narrows_only_when_asked(tmp_path) -> None:
    """THE DEFAULT IS EVERYTHING. Narrowing is opt-in, and visible at the call site.

    It used to default to `.py`, which meant a fingerprint in a README, a YAML fixture or a TOML
    file was invisible while the exit predicate said "every file" — a guarantee narrower than the
    words around it. Two reviewers caught the gap. Defaulting wide and requiring an explicit suffix
    to narrow puts the decision where a reader can see it.
    """
    (tmp_path / "a.txt").write_text(f"{FAKE}\n", encoding="utf-8")
    (tmp_path / "b.py").write_text(f"{FAKE}\n", encoding="utf-8")
    (tmp_path / "README.md").write_text(f"{FAKE}\n", encoding="utf-8")

    assert {p.name for p, _ in scan_tree_for_tokens(tmp_path, (FAKE,))} == {
        "a.txt", "b.py", "README.md",
    }
    assert [p.name for p, _ in scan_tree_for_tokens(tmp_path, (FAKE,), suffix=".py")] == ["b.py"]


def test_a_dotfile_is_scanned_even_though_a_dot_directory_is_not(tmp_path) -> None:
    """Dotfiles are exactly where secrets live, so being blind to them is the worst place to be.

    The skip rule tested every path part including the FILENAME, so `.env` beside a module was
    silently out of scope while `.venv/` was correctly excluded. Only directories are skipped now.
    """
    (tmp_path / ".env").write_text(f"{FAKE}\n", encoding="utf-8")
    (tmp_path / ".venv").mkdir()
    (tmp_path / ".venv" / "vendored.py").write_text(f"{FAKE}\n", encoding="utf-8")

    assert [str(p) for p, _ in scan_tree_for_tokens(tmp_path, (FAKE,))] == [".env"]


def test_an_undecodable_file_does_not_stop_the_scan(tmp_path) -> None:
    """A guard that crashes is a guard that reports nothing, which must never read as clean."""
    (tmp_path / "asset.bin").write_bytes(b"\xff\xfe\x00binary")
    (tmp_path / "real.py").write_text(f"# {FAKE}\n", encoding="utf-8")

    assert [p.name for p, _ in scan_tree_for_tokens(tmp_path, (FAKE,))] == ["real.py"]


def test_the_unarmed_message_is_itself_free_of_estate_fingerprints() -> None:
    """THE FOURTH APPEARANCE OF THIS DEFECT, and the reason it gets its own test.

    UNARMED is a pytest SKIP REASON, so it is printed into the CI log of a PUBLIC repository on
    every unarmed run. An earlier version interpolated the RESOLVED ABSOLUTE PATH of the tokens
    file, which begins with the operator's home directory. The guard's own diagnostic was
    publishing the thing the guard exists to keep unpublished.

    Counting the routes this change has now closed: the inline denylist, the stored pattern, the
    returned token, and this. The lesson is not about any one of them — it is that every artifact a
    guard emits (source, report, return value, diagnostic) is a disclosure channel, and each has to
    be checked separately because closing one says nothing about the others.

    Asserted against the estate's real tokens when armed, so it is this estate's actual
    fingerprints being excluded and not a stand-in.
    """
    # Asserted over the WHOLE message, not a slice of it. An earlier version located the filename
    # by splitting on a fixed phrase — so rewording the message would have made the check pass
    # while inspecting nothing, and a reviewer flagged it. Nothing in this string should ever
    # contain a path separator, so that is what is checked.
    assert "/" not in UNARMED, (
        "the unarmed message contains a path separator; it must name only a bare filename, since "
        "this string is printed into public CI logs"
    )
    assert TOKENS_FILE_NAME in UNARMED, "the operator must still be told what file to create"
    tokens = estate_tokens()
    if tokens is None:
        pytest.skip("cannot check against real fingerprints while unarmed; shape check above holds")

    # THE TOKEN IS NEVER AN ASSERTION OPERAND.
    #
    # `assert token not in UNARMED` reads correctly and republishes the token when it fails: pytest
    # rewrites assertions to render their operands, so the run that DETECTS the leak prints it.
    # Measured, not argued — planting a token in UNARMED and failing this test put it in the output.
    #
    # It is the same defect as a report that quotes its finding, arriving in the assertion idiom
    # itself, and it is why "the failure report is clean" has to be checked per assertion rather
    # than per file: an earlier pass measured the OTHER test in this module, found it clean, and
    # concluded about the file. One assertion is not all assertions.
    leaked = [index for index, token in enumerate(tokens) if token in UNARMED]
    assert not leaked, (
        f"estate fingerprint(s) at token index {leaked} appear in the skip reason, which is "
        f"printed into public CI logs. The tokens are not named here for the same reason."
    )


def test_a_failing_estate_scan_does_not_print_the_token_it_found(tmp_path) -> None:
    """RUNS PYTEST IN A SUBPROCESS AND READS THE RENDERED OUTPUT.

    Every other test here asserts on values. This one asserts on what a HUMAN AND A CI LOG actually
    see, because that is where a disclosure would happen and no in-process assertion can observe
    it. A reviewer asked for exactly this after the property had been verified twice by hand — and
    hand-verification is what let the `assert token not in UNARMED` leak survive two passes, since
    the check that was run covered a different assertion in the same file.

    THE PROBE LOADS THE TOKEN FROM A FILE rather than inlining it. The first version wrote the
    token as a literal into the probe's source, and pytest echoes the source of a failing test —
    so it failed, on its own construction rather than on the property. That is worth keeping in the
    record: it is the same rule one layer further out. The real scan has no token literal in its
    source either; the tokens arrive from outside, which is the whole design.

    Synthetic token throughout, so this is safe in any clone and proves the IDIOM rather than this
    estate's particular data.
    """
    import subprocess
    import sys

    token = "zzq-subprocess-canary"
    (tmp_path / "planted.py").write_text(f"# {token}\n", encoding="utf-8")
    (tmp_path / "token.txt").write_text(token, encoding="utf-8")
    (tmp_path / "test_probe.py").write_text(
        "import pathlib, sys\n"
        f"sys.path.insert(0, {str(pathlib.Path(__file__).parent)!r})\n"
        "from conftest import scan_tree_for_tokens\n"
        "HERE = pathlib.Path(__file__).parent\n"
        "def test_probe():\n"
        "    tok = (HERE / 'token.txt').read_text()\n"
        "    hits = scan_tree_for_tokens(HERE, (tok,))\n"
        "    assert not hits, f'found in {sorted({p.name for p, _ in hits})}'\n",
        encoding="utf-8",
    )
    proc = subprocess.run(
        [sys.executable, "-m", "pytest", str(tmp_path / "test_probe.py"), "-q", "--no-header"],
        capture_output=True,
        text=True,
        cwd=str(tmp_path),
    )

    assert proc.returncode != 0, "fixture precondition: the probe test must FAIL so output exists"
    rendered = proc.stdout + proc.stderr
    assert "planted.py" in rendered, "fixture precondition: the failure must name the guilty file"
    assert token not in rendered, (
        "the rendered pytest failure printed the token. The run that detects a leak must not be "
        "the run that publishes it."
    )


def test_the_scan_descends_into_sub_packages(tmp_path) -> None:
    """A NON-RECURSIVE SCAN IS AN EXEMPTION BY DEPTH.

    The package had no sub-directories when this guard was written, so a flat glob passed — and
    would have kept passing the day someone added one. That is the by-name exemption in a different
    hat: a scan that cannot see part of what it claims to cover has a green that means less than it
    looks like. This is the eighth variation on that theme in this change, which is why it gets a
    test rather than a comment.
    """
    nested = tmp_path / "sub" / "deeper"
    nested.mkdir(parents=True)
    (nested / "hidden.py").write_text(f"# {FAKE}\n", encoding="utf-8")
    hits = scan_tree_for_tokens(tmp_path, (FAKE,))
    assert [str(p) for p, _ in hits] == ["sub/deeper/hidden.py"], (
        "the scan did not descend; a token in a sub-package was invisible to it"
    )


def test_an_unenumerated_dot_directory_is_scanned_not_assumed_uninteresting(tmp_path) -> None:
    """`.github/` holds workflows that can name a host, and a blanket dot rule hid it.

    Two blanket `startswith(".")` rules were removed in turn, each found by review: one over every
    path part (hiding dotFILES, where secrets live) and one over the directory parts (hiding
    `.github/`, inside a scan that promised the whole package). Caches are enumerated instead, so a
    dot-directory nobody listed is SCANNED. A false positive costs a line in a skip list; a false
    negative costs a disclosure.
    """
    workflows = tmp_path / ".github" / "workflows"
    workflows.mkdir(parents=True)
    (workflows / "ci.yml").write_text(f"runs-on: {FAKE}\n", encoding="utf-8")

    assert [str(p) for p, _ in scan_tree_for_tokens(tmp_path, (FAKE,))] == [
        ".github/workflows/ci.yml"
    ]


def test_the_scan_skips_caches_and_vendored_trees_but_says_which(tmp_path) -> None:
    """The skip list is by NAME and enumerable, so nothing is skipped silently.

    A depth limit or a bare exclusion would be the same silent narrowing the recursion just fixed.
    These are directories that are not the estate's source at all; a token inside a virtualenv is
    the dependency's business, and scanning them would bury real findings in noise.
    """
    for directory in ("__pycache__", ".venv", "node_modules"):
        (tmp_path / directory).mkdir()
        (tmp_path / directory / "vendored.py").write_text(f"# {FAKE}\n", encoding="utf-8")
    (tmp_path / "real.py").write_text(f"# {FAKE}\n", encoding="utf-8")

    hits = scan_tree_for_tokens(tmp_path, (FAKE,))
    assert [str(p) for p, _ in hits] == ["real.py"], (
        f"expected only the source file; got {sorted(str(p) for p, _ in hits)}"
    )


def test_an_unreadable_file_refuses_rather_than_reporting_it_clean(tmp_path) -> None:
    """UNREADABLE AND UNDECODABLE ARE DIFFERENT ANSWERS, and they were the same line.

    Undecodable bytes cannot contain a token AS TEXT, so "" is honest for them. A file we could not
    OPEN is a file nobody looked at, and returning "" for that says "scanned, clean" about it —
    absence turned into zero, inside the helper belonging to the module written to remove that.

    A reviewer separated the two cases. The permission case now refuses, naming the FILENAME and
    never the path, because a source tree the scan cannot read is exceptional and should stop the
    guard loudly rather than dissolve into a green run.
    """
    import os

    blocked = tmp_path / "unreadable.py"
    blocked.write_text(f"# {FAKE}\n", encoding="utf-8")
    os.chmod(blocked, 0o000)
    try:
        if os.access(blocked, os.R_OK):
            pytest.skip("running as a user that bypasses file permissions (root?)")
        with pytest.raises(RuntimeError, match="NOT a file that is clean") as exc:
            scan_tree_for_tokens(tmp_path, (FAKE,))
        assert "unreadable.py" in str(exc.value), "the operator must be told which file"
        assert str(tmp_path) not in str(exc.value), "the containing path leaked into the error"
    finally:
        os.chmod(blocked, 0o644)


def test_an_unreadable_tokens_file_refuses_and_names_no_path(monkeypatch, tmp_path) -> None:
    """The same distinction one layer up: the guard's own input.

    An OSError carries `.filename`, so the error is raised WITHOUT chaining — the same reasoning as
    the regex validator, where `from exc` and `from None` both left the original reachable.
    """
    import os

    tokens_file = tmp_path / "tokens"
    tokens_file.write_text(f"{FAKE}\n", encoding="utf-8")
    os.chmod(tokens_file, 0o000)
    monkeypatch.setenv(TOKENS_ENV, str(tokens_file))
    try:
        if os.access(tokens_file, os.R_OK):
            pytest.skip("running as a user that bypasses file permissions (root?)")
        with pytest.raises(RuntimeError, match="could not be read") as exc:
            estate_tokens()
        assert str(tokens_file) not in str(exc.value), "the tokens path leaked into the error"
        assert exc.value.__cause__ is None and exc.value.__context__ is None, (
            "the OSError is still reachable, and it carries .filename"
        )
    finally:
        os.chmod(tokens_file, 0o644)


def test_a_symlinked_directory_refuses_rather_than_being_silently_unscanned(tmp_path) -> None:
    """`rglob` does not descend directory symlinks, so it would report success over a blind spot."""
    outside = tmp_path / "outside"
    outside.mkdir()
    (outside / "hidden.py").write_text(f"# {FAKE}\n", encoding="utf-8")
    root = tmp_path / "root"
    root.mkdir()
    (root / "linked").symlink_to(outside, target_is_directory=True)

    with pytest.raises(RuntimeError, match="unscanned is not clean") as exc:
        scan_tree_for_tokens(root, (FAKE,))
    assert "linked" in str(exc.value), "the operator must be told which entry to resolve"
    assert str(tmp_path) not in str(exc.value), "the containing path leaked into the error"


def test_a_symlinked_file_is_scanned_normally(tmp_path) -> None:
    """A link to a file resolves to bytes the tree genuinely contains, so it is read, not refused."""
    real = tmp_path / "real.py"
    real.write_text(f"# {FAKE}\n", encoding="utf-8")
    root = tmp_path / "root"
    root.mkdir()
    (root / "aliased.py").symlink_to(real)

    assert [str(p) for p, _ in scan_tree_for_tokens(root, (FAKE,))] == ["aliased.py"]
