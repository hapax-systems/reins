"""Read-only seat continuity, with synthetic bindings and no real pane contents."""

import builtins
import io
import os
import subprocess
from pathlib import Path

import pytest
from fastapi.testclient import TestClient
from reins_read import build_app, instance_config

FORMAT = (
    "#{session_name}\t#{pane_id}\t#{pane_pid}\t#{pane_current_command}\t#{pane_dead}"
)


@pytest.fixture
def bound(tmp_path):
    marker = tmp_path / "active-alpha-lease"
    marker.write_text("MUST NOT READ claim contents")
    charter = tmp_path / "charter.md"
    charter.write_text("---\nassigned_to: successor\n---\nPRIVATE BODY\n")
    return {
        "seat_succession": {
            "socket": str(tmp_path / "dead.sock"),
            "lanes": [
                {
                    "role": "alpha",
                    "session": "lane-alpha",
                    "claim_marker_pattern": str(tmp_path / "active-alpha-*"),
                    "charter_path": str(charter),
                },
                {
                    "role": "gone",
                    "session": "lane-gone",
                    "claim_marker_pattern": str(tmp_path / "active-gone-*"),
                    "charter_path": str(tmp_path / "missing.md"),
                },
            ],
        },
    }


def read(cfg):
    response = TestClient(build_app("", [], cfg)).get("/read/seat-succession")
    assert response.status_code == 200
    return response.json()


def test_no_write_or_content_read_and_current_projection(bound, monkeypatch, tmp_path):
    """Observe attempts, including same-byte writes; assert only metadata argv before effect."""
    calls = []

    def census(argv, **kwargs):
        assert argv == [
            "tmux",
            "-S",
            bound["seat_succession"]["socket"],
            "list-panes",
            "-a",
            "-F",
            FORMAT,
        ]
        assert kwargs == {
            "capture_output": True,
            "text": True,
            "check": True,
            "timeout": 2,
        }
        calls.append(argv)
        return subprocess.CompletedProcess(
            argv,
            0,
            "lane-alpha\t%1\t101\tbash\t0\nlane-alpha\t%2\t102\tpython\t0\nunbound\t%3\t103\tprivate\t0\n",
            "",
        )

    def forbid(*args, **kwargs):
        pytest.fail("forbidden mutation or alternate process path")

    original_open = builtins.open
    original_io_open = io.open
    original_os_open = os.open

    def guard(original):
        def opened(file, mode="r", *args, **kwargs):
            assert not any(flag in mode for flag in "wax+"), "write attempt"
            assert not Path(file).name.startswith("active-"), "claim content read"
            return original(file, mode, *args, **kwargs)

        return opened

    def low_open(path, flags, *args, **kwargs):
        assert not flags & (
            os.O_WRONLY | os.O_RDWR | os.O_CREAT | os.O_TRUNC | os.O_APPEND
        ), "write attempt"
        return original_os_open(path, flags, *args, **kwargs)

    before = {
        p.name: (p.read_bytes(), p.stat().st_mtime_ns) for p in tmp_path.iterdir()
    }
    with monkeypatch.context() as m:
        m.setattr(subprocess, "run", census)
        m.setattr(subprocess, "Popen", forbid)
        m.setattr(os, "system", forbid)
        m.setattr(builtins, "open", guard(original_open))
        m.setattr(io, "open", guard(original_io_open))
        m.setattr(os, "open", low_open)
        for method in ("touch", "unlink", "rename", "replace", "mkdir", "rmdir"):
            m.setattr(Path, method, forbid)
        result = read(bound)
    assert len(calls) == 1
    assert result["state"] == "ok"
    alpha, gone = result["rows"]
    assert alpha["pane_state"] == "present"
    assert [(p["pid"], p["command"]) for p in alpha["panes"]] == [
        (101, "bash"),
        (102, "python"),
    ]
    assert alpha["claims"]["state"] == "ok"
    assert alpha["claims"]["paths"] == [str(tmp_path / "active-alpha-lease")]
    assert alpha["claims"]["present"] is True
    assert alpha["assigned_to"] == "successor"
    assert alpha["charter_state"] == "ok"
    assert gone["pane_state"] == "absent" and gone["panes"] == []
    assert gone["claims"]["present"] is False
    assert gone["charter_state"] == "charter-unavailable"
    assert gone["assigned_to"] is None
    assert alpha["air"]["session"] == "deny"
    assert alpha["panes"][0]["air"]["command"] == "deny"
    assert "PRIVATE BODY" not in str(result) and "private" not in str(result)
    assert before == {
        p.name: (p.read_bytes(), p.stat().st_mtime_ns) for p in tmp_path.iterdir()
    }


def test_dead_socket_is_typed_unavailable(bound):
    # A nonexistent socket can only fail list-panes; no server is created.
    result = read(bound)
    assert result["state"] == "pane-census-unavailable"
    assert all(
        r["pane_state"] == "pane-census-unavailable" and r["panes"] is None
        for r in result["rows"]
    )
    assert result["rows"][0]["assigned_to"] == "successor"


@pytest.mark.parametrize(
    "failure",
    [FileNotFoundError(), subprocess.TimeoutExpired("tmux", 2), UnicodeError()],
)
def test_unavailable_census_never_claims_absence(bound, monkeypatch, failure):
    def failed(*args, **kwargs):
        raise failure

    monkeypatch.setattr(subprocess, "run", failed)
    assert read(bound)["state"] == "pane-census-unavailable"


def test_dead_retained_pane_is_not_a_live_leader(bound, monkeypatch):
    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *a, **k: subprocess.CompletedProcess(
            a, 0, "lane-alpha\t%1\t101\tbash\t1\n", ""
        ),
    )
    row = read(bound)["rows"][0]
    assert row["pane_state"] == "absent" and row["panes"] == []


@pytest.mark.parametrize(
    "output",
    [
        "malformed",
        "lane-alpha\t%1\tnan\tbash\t0\n",
        "lane-alpha\t%1\t101\tbash\tunknown\n",
    ],
)
def test_malformed_census_is_not_absence(bound, monkeypatch, output):
    monkeypatch.setattr(
        subprocess, "run", lambda *a, **k: subprocess.CompletedProcess(a, 0, output, "")
    )
    assert read(bound)["state"] == "pane-census-unavailable"


@pytest.mark.parametrize(
    "binding", [None, {}, {"lanes": []}, {"lanes": [{"role": "alpha"}]}]
)
def test_missing_or_invalid_binding_never_probes(binding, monkeypatch):
    monkeypatch.setattr(subprocess, "run", lambda *a, **k: pytest.fail("unbound probe"))
    assert read({"seat_succession": binding}) == {
        "state": "seat-binding-unavailable",
        "rows": [],
    }


def test_instance_config_carries_binding(monkeypatch, tmp_path):
    path = tmp_path / "instance.toml"
    path.write_text(
        '[seat_succession]\nsocket = "/tmp/fixture-socket"\n[[seat_succession.lanes]]\nrole = "fixture"\nsession = "fixture-session"\nclaim_marker_pattern = "/tmp/fixture-claims/*"\ncharter_path = "/tmp/fixture-charter.md"\n'
    )
    monkeypatch.setenv("REINS_CONFIG", str(path))
    cfg = instance_config()
    assert cfg["seat_succession"]["socket"] == "/tmp/fixture-socket"
    assert cfg["seat_succession"]["lanes"][0]["session"] == "fixture-session"


def test_unreadable_claims_and_unassigned_charter_are_distinct(
    bound, monkeypatch, tmp_path
):
    monkeypatch.setattr(
        subprocess, "run", lambda *a, **k: subprocess.CompletedProcess(a, 0, "", "")
    )
    Path(bound["seat_succession"]["lanes"][0]["charter_path"]).write_text(
        "---\nassigned_to: null\n---\n"
    )
    bound["seat_succession"]["lanes"][0]["claim_marker_pattern"] = str(
        tmp_path / "missing" / "*"
    )
    row = read(bound)["rows"][0]
    assert row["claims"] == {
        "state": "claim-census-unavailable",
        "present": None,
        "paths": None,
        "air": {"state": "deny", "present": "deny", "paths": "deny"},
    }
    assert row["charter_state"] == "unassigned" and row["assigned_to"] is None


def test_unstatable_matching_marker_is_unknown_not_absent(bound, monkeypatch, tmp_path):
    """Under Python 3.14 Path.is_file() swallows PermissionError and returns False.
    A matching marker that cannot be stat'ed (EACCES) must report claim-census-unavailable,
    never present: False."""
    import errno
    import reins_seat_succession as ss

    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *a, **k: subprocess.CompletedProcess(
            a, 0, "lane-alpha\t%1\t101\tbash\t0\n", ""
        ),
    )
    # 1. Real filesystem EACCES via symlink into an unsearchable (0o000) directory
    for p in tmp_path.glob("active-alpha-*"):
        p.unlink()
    locked_dir = tmp_path / "locked_dir"
    locked_dir.mkdir()
    secret_target = locked_dir / "marker"
    secret_target.write_text("x", encoding="utf-8")
    (tmp_path / "active-alpha-locked").symlink_to(secret_target)
    locked_dir.chmod(0o000)
    try:
        if os.geteuid() != 0:
            # Verify Python 3.14's Path.is_file() behavior or direct stat failure:
            row = read(bound)["rows"][0]
            assert row["claims"]["state"] == "claim-census-unavailable"
            assert row["claims"]["present"] is None
            assert row["claims"]["paths"] is None
    finally:
        locked_dir.chmod(0o700)

    # 2. Interpreter-independent pin: even if Path.is_file() swallows PermissionError
    # (as Python 3.14 does), _claims must use stat() so EACCES yields claim-census-unavailable.
    orig_stat = Path.stat

    def stat_eacces(self, *args, **kwargs):
        if self.name == "active-alpha-locked":
            raise PermissionError(errno.EACCES, "Permission denied", str(self))
        return orig_stat(self, *args, **kwargs)

    with monkeypatch.context() as m:
        m.setattr(Path, "is_file", lambda self: False)
        m.setattr(Path, "stat", stat_eacces)
        claims = ss._claims(str(tmp_path / "active-alpha-*"))
    assert claims == {
        "state": "claim-census-unavailable",
        "present": None,
        "paths": None,
    }


def test_unbounded_charter_frontmatter_fails_closed(bound, monkeypatch):
    monkeypatch.setattr(
        subprocess, "run", lambda *a, **k: subprocess.CompletedProcess(a, 0, "", "")
    )
    charter = Path(bound["seat_succession"]["lanes"][0]["charter_path"])
    charter.write_text("---\nassigned_to: successor\n" + ("x: y\n" * 1000), encoding="utf-8")
    row = read(bound)["rows"][0]
    assert row["charter_state"] == "charter-unavailable"
    assert row["assigned_to"] is None

