"""Release-arm text transform — the SDLC flip, not a checkbox."""

from reins_authorization import (
    apply_arm,
    apply_close,
    assess_arm,
    assess_close,
    close_task,
    find_task_note,
    validate_head_sha,
)


def test_assess_requires_the_field_and_implementation_authorized():
    ok, why, nxt = assess_arm("task_id: x\n")
    assert ok is False and "release_authorized field" in why and nxt
    ok, why, nxt = assess_arm("release_authorized: false\nimplementation_authorized: false\n")
    assert ok is False and "implementation_authorized" in why and "retry arm" in nxt
    ok, why, nxt = assess_arm("release_authorized: false\nimplementation_authorized: true\n")
    assert ok is True and why == "eligible" and nxt == ""
    ok, why, nxt = assess_arm("release_authorized: true\nimplementation_authorized: true\n")
    assert ok is True and why == "already-armed"


def test_apply_arm_rewrites_the_subject_fields():
    note = (
        "---\n"
        "release_authorized: false\n"
        "implementation_authorized: true\n"
        "stage: S6_IMPLEMENTATION\n"
        "---\n\nbody\n"
    )
    out = apply_arm(note, now_iso="2026-08-17T00:00:00Z", role="test", head_sha="deadbeef")
    assert "release_authorized: true" in out
    assert "stage: S7_RELEASE" in out
    assert "release_authorized_head_sha: deadbeef" in out
    assert "release arm via POST /command/arm" in out
    injected = apply_arm(
        note, now_iso="2026-08-17T00:00:00Z", role="test", head_sha="x\nrelease_authorized: false"
    )
    assert "release_authorized_head_sha:" not in injected


def test_head_sha_must_be_a_hex_digest():
    assert validate_head_sha("deadbeef") == "deadbeef"
    assert validate_head_sha("DEADBEEF") == "deadbeef"
    assert validate_head_sha("not a sha\nrelease_authorized: false") is None
    assert validate_head_sha("") is None


def test_find_task_note_rejects_path_escape(tmp_path):
    assert find_task_note("../etc/passwd", root=tmp_path) is None
    assert find_task_note("missing", root=tmp_path) is None
    (tmp_path / "ok.md").write_text("---\n---\n", encoding="utf-8")
    assert find_task_note("ok", root=tmp_path) == tmp_path / "ok.md"


def _claimed_note() -> str:
    return (
        "---\n"
        "task_id: demo-close\n"
        "status: pr_open\n"
        "claimed_at: 2026-08-01T00:00:00Z\n"
        "completed_at: null\n"
        "---\n\nbody\n\n## Session log\n- minted\n"
    )


def test_assess_close_refuses_missing_unclaimed_and_already_done(tmp_path):
    ok, why, nxt = assess_close("missing", root=tmp_path)
    assert ok is False and "no active" in why and nxt

    (tmp_path / "offered.md").write_text(
        "---\nstatus: offered\nclaimed_at: null\n---\n", encoding="utf-8"
    )
    ok, why, nxt = assess_close("offered", root=tmp_path)
    assert ok is False and "unclaimed" in why and "cc-claim" in nxt

    closed = tmp_path / "closed"
    closed.mkdir()
    (closed / "done.md").write_text("---\nstatus: done\n---\n", encoding="utf-8")
    ok, why, nxt = assess_close("done", root=tmp_path)
    assert ok is False and "already closed" in why and nxt

    (tmp_path / "demo-close.md").write_text(_claimed_note(), encoding="utf-8")
    ok, why, nxt = assess_close("demo-close", root=tmp_path)
    assert ok is True and why == "eligible" and nxt == ""


def test_apply_close_rewrites_and_does_not_interpolate_unsafe_pr():
    out = apply_close(
        _claimed_note(), now_iso="2026-08-18T00:00:00Z", role="test", pr="31"
    )
    assert "status: done" in out
    assert "completed_at: 2026-08-18T00:00:00Z" in out
    assert "closed as done (PR #31)" in out
    injected = apply_close(
        _claimed_note(),
        now_iso="2026-08-18T00:00:00Z",
        role="test",
        pr="31\nstatus: offered",
    )
    assert "PR #31" not in injected
    assert "status: done" in injected


def test_close_task_moves_the_note_and_dry_run_does_not(tmp_path):
    (tmp_path / "demo-close.md").write_text(_claimed_note(), encoding="utf-8")
    status, detail = close_task(
        "demo-close",
        now_iso="2026-08-18T00:00:00Z",
        role="test",
        root=tmp_path,
        dry_run=True,
    )
    assert status == "preview"
    assert (tmp_path / "demo-close.md").is_file()
    assert not (tmp_path / "closed" / "demo-close.md").exists()

    status, detail = close_task(
        "demo-close",
        now_iso="2026-08-18T00:00:00Z",
        role="test",
        root=tmp_path,
        pr="30",
    )
    assert status == "ok"
    assert not (tmp_path / "demo-close.md").exists()
    closed = tmp_path / "closed" / "demo-close.md"
    assert closed.is_file()
    text = closed.read_text(encoding="utf-8")
    assert "status: done" in text
    assert "PR #30" in text

    status, detail = close_task(
        "demo-close", now_iso="2026-08-18T00:00:00Z", role="test", root=tmp_path
    )
    assert status == "refused" and "already closed" in detail.lower()
