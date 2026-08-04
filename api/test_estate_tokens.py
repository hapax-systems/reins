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

from conftest import TOKENS_ENV, UNARMED, estate_tokens, scan_tree_for_tokens

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


def test_the_scan_only_reads_the_requested_suffix(tmp_path) -> None:
    """Scoping is explicit. A silently-widened scan would read files it was never asked to."""
    (tmp_path / "a.txt").write_text(f"{FAKE}\n", encoding="utf-8")
    assert scan_tree_for_tokens(tmp_path, (FAKE,)) == []
    assert len(scan_tree_for_tokens(tmp_path, (FAKE,), suffix=".txt")) == 1


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
    assert "/" not in UNARMED.split("write one token per line to ")[1].split(" ")[0], (
        "the unarmed message names a PATH; it must name only a filename, since this string is "
        "printed into public CI logs"
    )
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
