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

from conftest import TOKENS_ENV, estate_tokens, scan_tree_for_tokens

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
    """
    monkeypatch.setenv(TOKENS_ENV, str(tmp_path / "does-not-exist"))
    with pytest.raises(RuntimeError, match="worse than one that is undeclared"):
        estate_tokens()


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
    import pathlib as _p

    hints = scan_tree_for_tokens.__annotations__.get("return")
    assert hints is not None, "the return type must be stated, since it is the security property"
    assert "int" in str(hints) and "str" not in str(hints), (
        f"scan_tree_for_tokens returns {hints}; it must yield an index, never the token"
    )
    assert _p is not None


def test_a_clean_tree_yields_nothing(tmp_path) -> None:
    (tmp_path / "a.py").write_text("x = 1\n", encoding="utf-8")
    assert scan_tree_for_tokens(tmp_path, (FAKE,)) == []


def test_the_scan_only_reads_the_requested_suffix(tmp_path) -> None:
    """Scoping is explicit. A silently-widened scan would read files it was never asked to."""
    (tmp_path / "a.txt").write_text(f"{FAKE}\n", encoding="utf-8")
    assert scan_tree_for_tokens(tmp_path, (FAKE,)) == []
    assert len(scan_tree_for_tokens(tmp_path, (FAKE,), suffix=".txt")) == 1
