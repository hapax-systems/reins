"""Release-arm text transform — the SDLC flip, not a checkbox."""

from reins_authorization import apply_arm, assess_arm, find_task_note


def test_assess_requires_the_field_and_implementation_authorized():
    ok, why = assess_arm("task_id: x\n")
    assert ok is False and "release_authorized field" in why
    ok, why = assess_arm("release_authorized: false\nimplementation_authorized: false\n")
    assert ok is False and "implementation_authorized" in why
    ok, why = assess_arm("release_authorized: false\nimplementation_authorized: true\n")
    assert ok is True and why == "eligible"
    ok, why = assess_arm("release_authorized: true\nimplementation_authorized: true\n")
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


def test_find_task_note_rejects_path_escape(tmp_path):
    assert find_task_note("../etc/passwd", root=tmp_path) is None
    assert find_task_note("missing", root=tmp_path) is None
    (tmp_path / "ok.md").write_text("---\n---\n", encoding="utf-8")
    assert find_task_note("ok", root=tmp_path) == tmp_path / "ok.md"
