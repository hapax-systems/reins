"""`scripts/reins-release` — the refusals are the subject under test.

The frontdoor API used to ExecStart from a mutable developer checkout, so the code the
operator talked to was whatever someone last checked out. `reins-release` replaces that
with an immutable, tested generation plus a `current` symlink.

The whole value of that mechanism is what it DECLINES to make current. A release script
that builds happily is worth nothing; one that refuses a dirty tree, an unmerged commit,
and a failing suite is the delivery guarantee. So these tests drive the real script
against synthetic repos and assert on `current` — not on exit codes alone, because an
exit code says what the script reported and `current` says what the operator will be
served on the next restart.

Every refusal is checked for both: it exits non-zero, AND it leaves `current` exactly
where it was. A failure path that moved `current` while reporting failure would be the
defect this file exists to catch.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parent.parent
RELEASE = REPO_ROOT / "scripts" / "reins-release"

pytestmark = pytest.mark.skipif(
    shutil.which("git") is None, reason="reins-release is a git-driven mechanism"
)


def _git(cwd: Path, *args: str) -> str:
    out = subprocess.run(
        ["git", *args], cwd=cwd, capture_output=True, text=True, check=True
    )
    return out.stdout.strip()


def _run(repo: Path, root: Path, current: Path, *args: str):
    """Invoke the real script, fully redirected off the operator's estate."""
    env = {
        **os.environ,
        "REINS_REPO": str(repo),
        "REINS_RELEASE_ROOT": str(root),
        "REINS_CURRENT_LINK": str(current),
    }
    return subprocess.run(
        ["bash", str(RELEASE), *args],
        capture_output=True,
        text=True,
        env=env,
        timeout=900,
    )


def _make_repo(tmp_path: Path, *, test_body: str) -> tuple[Path, str]:
    """A synthetic instance: a bare origin plus a working clone whose `main` is real.

    The ancestry gate reads `origin/main`, so a fixture without a genuine remote would
    exercise a different code path than the operator's.
    """
    origin = tmp_path / "origin.git"
    subprocess.run(
        ["git", "init", "--quiet", "--bare", "-b", "main", str(origin)], check=True
    )

    repo = tmp_path / "repo"
    subprocess.run(
        ["git", "clone", "--quiet", str(origin), str(repo)],
        check=True,
        capture_output=True,
    )
    _git(repo, "config", "user.email", "test@example.invalid")
    _git(repo, "config", "user.name", "test")

    api = repo / "api"
    api.mkdir()
    (api / "pyproject.toml").write_text(
        "[project]\n"
        'name = "synthetic"\n'
        'version = "0"\n'
        'requires-python = ">=3.11"\n'
        "dependencies = []\n\n"
        "[dependency-groups]\n"
        'dev = ["pytest"]\n',
        encoding="utf-8",
    )
    (api / "test_synthetic.py").write_text(test_body, encoding="utf-8")

    _git(repo, "add", "-A")
    _git(repo, "commit", "--quiet", "-m", "synthetic generation")
    _git(repo, "push", "--quiet", "origin", "main")
    return repo, _git(repo, "rev-parse", "HEAD")


PASSING = "def test_ok():\n    assert True\n"
FAILING = "def test_no():\n    assert False, 'this generation is not eligible'\n"


@pytest.fixture
def store(tmp_path: Path) -> tuple[Path, Path, Path]:
    """A release root whose `current` ALREADY points somewhere.

    Asserting that a refusal left `current` absent proves much less than asserting it
    left `current` pointing at the same generation as before — the live case is always
    a running service with an existing pin.
    """
    root = tmp_path / "releases"
    root.mkdir()
    incumbent = root / "incumbent"
    incumbent.mkdir()
    current = tmp_path / "current"
    current.symlink_to(incumbent)
    return root, current, incumbent


def _current_target(current: Path) -> str | None:
    return os.readlink(current) if current.is_symlink() else None


def _needs_uv() -> None:
    if shutil.which("uv") is None:
        pytest.skip("the release gate builds its venv with uv")


def test_dirty_tree_is_refused_and_current_is_untouched(tmp_path, store):
    """A SHA does not describe a tree with uncommitted edits in it.

    Releasing anyway would produce a generation that silently disagrees with what the
    developer is looking at — a subtler version of the original defect.
    """
    root, current, _ = store
    repo, sha = _make_repo(tmp_path, test_body=PASSING)
    (repo / "api" / "test_synthetic.py").write_text(
        "def test_edited():\n    assert True\n", encoding="utf-8"
    )
    before = _current_target(current)

    result = _run(repo, root, current, sha)

    assert result.returncode != 0
    assert "dirty" in result.stderr
    assert _current_target(current) == before
    assert not (root / sha).exists()


def test_commit_outside_origin_main_is_refused(tmp_path, store):
    """An unmerged branch is precisely the condition this script exists to end."""
    root, current, _ = store
    repo, _ = _make_repo(tmp_path, test_body=PASSING)
    _git(repo, "checkout", "--quiet", "-b", "feature")
    (repo / "unmerged.txt").write_text("not on main\n", encoding="utf-8")
    _git(repo, "add", "-A")
    _git(repo, "commit", "--quiet", "-m", "unmerged work")
    unmerged = _git(repo, "rev-parse", "HEAD")
    before = _current_target(current)

    result = _run(repo, root, current, unmerged)

    assert result.returncode != 0
    assert "not an ancestor of origin/main" in result.stderr
    assert _current_target(current) == before
    assert not (root / unmerged).exists()


def test_unknown_commit_is_refused(tmp_path, store):
    root, current, _ = store
    repo, _ = _make_repo(tmp_path, test_body=PASSING)
    before = _current_target(current)

    result = _run(repo, root, current, "0" * 40)

    assert result.returncode != 0
    assert "unknown commit" in result.stderr
    assert _current_target(current) == before


def test_missing_argument_is_refused(tmp_path, store):
    root, current, _ = store
    repo, _ = _make_repo(tmp_path, test_body=PASSING)

    result = _run(repo, root, current)

    assert result.returncode != 0
    assert "usage" in result.stderr


def test_failing_suite_does_not_become_current(tmp_path, store):
    """THE acceptance criterion: a generation that cannot prove its own tests pass
    must not become the thing the operator talks to.

    This really clones, really builds a venv, and really runs pytest, because the gate
    is worth exactly as much as its weakest real invocation.
    """
    _needs_uv()
    root, current, _ = store
    repo, sha = _make_repo(tmp_path, test_body=FAILING)
    before = _current_target(current)

    result = _run(repo, root, current, sha)

    assert result.returncode != 0
    assert "tests FAILED" in result.stderr
    assert _current_target(current) == before
    # Neither the finished generation nor the scratch build may survive a refusal: a
    # half-built tree left at $dest would let a later run's "already built"
    # short-circuit adopt an untested generation.
    assert not (root / sha).exists()
    assert not (root / f"{sha}.partial").exists()


def test_passing_generation_becomes_current(tmp_path, store):
    """The success path — so the refusals above are known to be refusals, and not a
    script that fails on everything."""
    _needs_uv()
    root, current, _ = store
    repo, sha = _make_repo(tmp_path, test_body=PASSING)

    result = _run(repo, root, current, sha)

    assert result.returncode == 0, result.stderr
    assert _current_target(current) == str(root / sha)
    assert (root / sha / ".reins-release-complete").exists()
    # A clean clone at the SHA, not a copy of the working tree.
    assert _git(root / sha, "rev-parse", "HEAD") == sha


def test_untracked_files_do_not_travel_into_a_generation(tmp_path, store):
    """`git clone`, not a copy: a developer's scratch files must not be served.

    The service used to run from the checkout itself, so anything lying around in it
    was live. A generation that inherited untracked files would carry that defect
    forward under a new name.
    """
    _needs_uv()
    root, current, _ = store
    repo, sha = _make_repo(tmp_path, test_body=PASSING)
    (repo / "api" / "scratch_note.txt").write_text("dev leftovers\n", encoding="utf-8")
    # Untracked-but-ignored, so the tree is still clean and the release is allowed.
    (repo / ".git" / "info" / "exclude").write_text(
        "api/scratch_note.txt\n", encoding="utf-8"
    )

    result = _run(repo, root, current, sha)

    assert result.returncode == 0, result.stderr
    assert not (root / sha / "api" / "scratch_note.txt").exists()
