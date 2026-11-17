"""authority-case-check: the release arm's required check, driven through every branch.

The workflow's steps call .github/scripts/authority-case-check.sh; these tests run that script
the way the steps do (environment in, $GITHUB_OUTPUT and stdout out) on fixture PR bodies and a
throwaway git history.
"""

from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path

import pytest
import yaml

REPO = Path(__file__).resolve().parents[1]
SCRIPT = REPO / ".github" / "scripts" / "authority-case-check.sh"
WORKFLOW = REPO / ".github" / "workflows" / "authority-case-check.yml"


def run(command: str, cwd: Path | None = None, **env: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", str(SCRIPT), command],
        cwd=cwd or REPO,
        env={"PATH": os.environ["PATH"], "HOME": os.environ.get("HOME", "/tmp"), **env},
        capture_output=True,
        text=True,
        timeout=60,
        check=False,
    )


def parse(body: str, tmp_path: Path) -> dict[str, str]:
    out = tmp_path / "github_output"
    result = run("parse", PR_BODY=body, GITHUB_OUTPUT=str(out))
    assert result.returncode == 0, result.stderr
    return dict(line.split("=", 1) for line in out.read_text().splitlines())


@pytest.mark.parametrize(
    ("body", "expected"),
    [
        ("Row CASE-SYS-INTEGRITY-1, SLICE-2.", ("CASE-SYS-INTEGRITY-1", "SLICE-2", "false", "")),
        ("cc-task: `reins-row-1`", ("", "", "false", "reins-row-1")),
        ("CC-TASK: Reins-Row-2", ("", "", "false", "Reins-Row-2")),
        ("A pre-methodology change.", ("", "", "true", "")),
        ("No reference at all.", ("", "", "false", "")),
    ],
)
def test_parse_reads_each_reference_form(tmp_path, body, expected):
    got = parse(body, tmp_path)
    assert (got["case_id"], got["slice_id"], got["pre_methodology"], got["cc_task"]) == expected


def test_parse_survives_a_body_long_enough_to_break_a_pipe(tmp_path):
    # GitHub's PR-body ceiling (65,536 characters), every line a reference: a `grep | head`
    # pipeline would write more than one pipe buffer after head exits, and die of SIGPIPE.
    body = "CASE-LONG-0000\n" * (65_536 // 15)
    assert parse(body, tmp_path)["case_id"] == "CASE-LONG-0000"


@pytest.mark.parametrize(
    ("env", "message"),
    [
        ({"PRE_METHODOLOGY": "true"}, "PASS — pre-methodology"),
        ({"CC_TASK": "reins-row-1"}, "PASS — legacy cc-task reference 'reins-row-1'"),
        ({}, "::warning::No AuthorityCase or cc-task reference"),
        (
            {"CASE_ID": "CASE-X-1", "SLICE_ID": "SLICE-2"},
            "PASS — AuthorityCase reference: CASE-X-1 / SLICE-2",
        ),
    ],
)
def test_validate_passes_every_branch_and_says_which(env, message):
    full = {"CASE_ID": "", "SLICE_ID": "", "PRE_METHODOLOGY": "false", "CC_TASK": "", **env}
    result = run("validate", **full)

    assert result.returncode == 0, result.stderr
    assert message in result.stdout


@pytest.fixture
def history(tmp_path: Path) -> Path:
    def git(*args: str) -> None:
        subprocess.run(["git", *args], cwd=tmp_path, check=True, capture_output=True)

    git("init", "-q", "-b", "main")
    git("config", "user.email", "test@example.invalid")
    git("config", "user.name", "test")
    (tmp_path / "README.md").write_text("base\n")
    git("add", "-A")
    git("commit", "-q", "-m", "base")
    return tmp_path


def commit(repo: Path, *paths: str) -> str:
    base = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=repo, check=True, capture_output=True, text=True
    ).stdout.strip()
    for rel in paths:
        (repo / rel).parent.mkdir(parents=True, exist_ok=True)
        (repo / rel).write_text("changed\n")
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
    subprocess.run(["git", "commit", "-q", "-m", "change"], cwd=repo, check=True)
    return base


@pytest.mark.parametrize(
    "path",
    [
        "GOVERNANCE.md",
        ".github/workflows/ci.yml",
        ".github/scripts/authority-case-check.sh",
        "api/hapax_secret.py",
        "scripts/hapax-secret",
        "api/reins_claim_custody.py",
    ],
)
def test_a_protected_path_is_named_and_still_passes(history, path):
    base = commit(history, path, "internal/api/x.go")
    result = run("paths", cwd=history, PR_BASE_SHA=base)

    assert result.returncode == 0, result.stderr
    assert "::warning::Protected governance paths changed" in result.stdout
    assert f"  - {path}" in result.stdout
    assert "  - internal/api/x.go" not in result.stdout
    assert "Changed files: 2, protected paths touched: true" in result.stdout


def test_unprotected_changes_touch_nothing(history):
    base = commit(history, "internal/api/x.go", "docs/concepts.md")
    result = run("paths", cwd=history, PR_BASE_SHA=base)

    assert result.returncode == 0, result.stderr
    assert "::warning::" not in result.stdout
    assert "Changed files: 2, protected paths touched: false" in result.stdout


def test_no_base_sha_and_no_changes_both_skip(history):
    no_base = run("paths", cwd=history, PR_BASE_SHA="")
    head = commit(history, "README.md")
    no_change = run("paths", cwd=history, PR_BASE_SHA=subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=history, check=True, capture_output=True, text=True
    ).stdout.strip())  # fmt: skip

    assert (no_base.returncode, no_change.returncode) == (0, 0)
    assert "No base SHA — skipping path check." in no_base.stdout
    assert "No changed files detected." in no_change.stdout
    assert head


def test_an_unknown_command_is_refused_with_a_next_action():
    result = run("bogus")

    assert result.returncode == 2
    assert "next action" in result.stderr.lower()


def test_the_workflow_runs_each_step_through_the_script():
    jobs = yaml.safe_load(WORKFLOW.read_text())["jobs"]
    steps = jobs["authority-case-check"]["steps"]
    runs = [step.get("run", "") for step in steps]

    for command in ("parse", "validate", "paths"):
        assert any(f".github/scripts/authority-case-check.sh {command}" in r for r in runs)
    assert all("${{" not in r for r in runs), "untrusted input must arrive through env only"


def test_checkout_is_pinned_to_the_full_sha_reins_ci_uses():
    pin = re.compile(r"actions/checkout@([0-9a-f]+)")
    ours = pin.findall(WORKFLOW.read_text())
    ci = set(pin.findall((REPO / ".github" / "workflows" / "ci.yml").read_text()))

    assert ours and all(len(sha) == 40 for sha in ours)
    assert set(ours) == ci
