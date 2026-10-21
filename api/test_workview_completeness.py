"""Pure work-view source-loss, conservation and coord-join regressions."""
import unittest
from pathlib import Path
from unittest.mock import patch

import reins_read as r


class Directory:
    def __init__(self, files=()):
        self.files = files

    def is_dir(self):
        return True

    def glob(self, pattern):
        return [Path("/synthetic") / name for name in self.files]


def source_dirs(active=()):
    return {
        "explicit": True,
        "base": "synthetic",
        "base_exists": True,
        "cc_active_dir": Directory(active),
        "cc_closed_dir": Directory(),
        "req_active_dir": Directory(),
        "req_closed_dir": Directory(),
    }


class CompletenessTests(unittest.TestCase):
    def setUp(self):
        actual = r._workview_markdown_paths

        def enumerate_source(directory):
            if isinstance(directory, Directory):
                return directory.glob("*.md"), True
            return actual(directory)

        mocked = patch.object(r, "_workview_markdown_paths", side_effect=enumerate_source)
        mocked.start()
        self.addCleanup(mocked.stop)

    def test_unreadable_active_candidate_is_counted_as_unobserved(self):
        dirs = source_dirs(["good.md", "unreadable.md"])

        def loader(path):
            if path.name == "good.md":
                return {"task_id": "good", "status": "open"}, "", 0
            return None, "", 0

        with (
            patch.object(r, "_resolve_workview_dirs", return_value=dirs),
            patch.object(r, "_load_yaml_frontmatter_and_log", side_effect=loader),
            patch.object(r, "_projection", return_value={"tasks": {}, "_replay": {"source": "sqlite", "degraded": False, "errors_present": False}}),
            patch.object(r, "_task_history", return_value=({}, False)),
        ):
            tasks = r.read_workview_tasks("synthetic", [])
            census = r.build_wait_census("synthetic", council_root="synthetic")
        for result in (tasks, census):
            self.assertTrue(result.get("partial"))
            self.assertEqual(result["universe"]["cc_task_files"]["active"], 2)
            self.assertEqual(result["universe"].get("unobserved_files"), 1)
            self.assertEqual(result["universe"]["active_distinct_task_ids"], 1)
        self.assertFalse(tasks["dark"])

    def test_missing_cc_and_failed_coord_cannot_claim_empty_live_work(self):
        dirs = source_dirs()
        dirs["cc_active_dir"] = None
        with (
            patch.object(r, "_resolve_workview_dirs", return_value=dirs),
            patch.object(r, "_projection", side_effect=RuntimeError("unavailable")),
        ):
            result = r.read_workview_tasks("synthetic", [])
        self.assertTrue(result["dark"])
        self.assertTrue(result.get("partial"))
        self.assertIsNone(result["universe"]["known_work"])
        self.assertIsNone(result["universe"]["cc_task_files"]["active"])
        self.assertIsNone(result["universe"]["coord_tasks"])

    def test_coord_only_task_and_its_edges_are_in_both_projections(self):
        dirs = source_dirs(["vault-task.md"])
        row = {"task_id": "vault-task", "status": "open", "depends_on": ["coord-only"]}
        with (
            patch.object(r, "_resolve_workview_dirs", return_value=dirs),
            patch.object(r, "_load_yaml_frontmatter_and_log", return_value=(row, "", 0)),
            patch.object(r, "_projection", return_value={"tasks": {"coord-only": {"status": "open"}}, "_replay": {"source": "sqlite", "degraded": False, "errors_present": False}}),
            patch.object(r, "_task_history", return_value=({}, False)),
        ):
            tasks = r.read_workview_tasks("synthetic", [])
            census = r.build_wait_census("synthetic", council_root="synthetic")
        for result in (tasks, census):
            self.assertEqual(result["universe"]["active_distinct_task_ids"], 2)
            self.assertEqual(result["universe"]["known_work"], 2)
            self.assertEqual(result["universe"].get("coord_only_tasks"), 1)
        self.assertEqual(sum(census["wait_bands"].values()), 2)
        self.assertEqual(tasks["wait_band_counts"], census["wait_bands"])
        by_id = {row["task_id"]: row for row in tasks["tasks"]}
        self.assertEqual(by_id["coord-only"]["reverse_dep_count"], 1)

    def test_genuinely_empty_available_sources_are_complete(self):
        with (
            patch.object(r, "_resolve_workview_dirs", return_value=source_dirs()),
            patch.object(r, "_projection", return_value={"tasks": {}, "_replay": {"source": "sqlite", "degraded": False, "errors_present": False}}),
            patch.object(r, "_task_history", return_value=({}, False)),
        ):
            tasks = r.read_workview_tasks("synthetic", [])
            census = r.build_wait_census("synthetic", council_root="synthetic")
        for result in (tasks, census):
            self.assertFalse(result["dark"])
            self.assertFalse(result.get("partial", False))
            self.assertEqual(result["universe"]["known_work"], 0)

    def test_coord_loss_preserves_vault_row_and_marks_partial(self):
        dirs = source_dirs(["vault-task.md"])
        with (
            patch.object(r, "_resolve_workview_dirs", return_value=dirs),
            patch.object(r, "_load_yaml_frontmatter_and_log", return_value=({"task_id": "vault-task", "status": "open"}, "", 0)),
            patch.object(r, "_projection", side_effect=RuntimeError("unavailable")),
        ):
            tasks = r.read_workview_tasks("synthetic", [])
            census = r.build_wait_census("synthetic", council_root="synthetic")
        for result in (tasks, census):
            self.assertFalse(result["dark"])
            self.assertTrue(result.get("partial"))
            self.assertIsNone(result["universe"]["coord_tasks"])
            self.assertIsNone(result["universe"]["known_work"])
            self.assertEqual(result["universe"]["active_distinct_task_ids"], 1)


def _empty_vault(tmp_path):
    base = tmp_path / "vault"
    for project in ("hapax-cc-tasks", "hapax-requests"):
        for state in ("active", "closed"):
            (base / "20-projects" / project / state).mkdir(parents=True)
    return base


def test_actual_unreadable_active_directory_is_partial_not_false_empty(tmp_path, monkeypatch):
    import os
    import pytest

    if os.geteuid() == 0:
        pytest.skip("Root can enumerate chmod000 directories; run this permission probe unprivileged")
    base = _empty_vault(tmp_path)
    cc = base / "20-projects" / "hapax-cc-tasks"
    active = cc / "active"
    (active / "hidden.md").write_text("---\ntask_id: hidden\nstatus: open\n---\n")
    cfg = {"vault_root": str(base), "cc_tasks_active": str(cc)}
    monkeypatch.setattr(r, "_projection", lambda _, **kw: {"tasks": {}, "_replay": {"source": "sqlite", "degraded": False, "errors_present": False}})
    monkeypatch.setattr(r, "_task_history", lambda _: ({}, False))
    active.chmod(0)
    try:
        for result in (
            r.read_workview_tasks("synthetic", [], cfg),
            r.build_wait_census(cfg, council_root="synthetic"),
        ):
            assert result["partial"] is True
            assert result["completeness"]["sources"]["cc_active_dir"] == "dark"
            assert result["universe"]["known_work"] is None
            assert result["universe"]["cc_task_files"]["active"] is None
    finally:
        active.chmod(0o700)


def test_actual_readable_empty_directories_are_complete(tmp_path, monkeypatch):
    base = _empty_vault(tmp_path)
    monkeypatch.setattr(r, "_projection", lambda _, **kw: {"tasks": {}, "_replay": {"source": "sqlite", "degraded": False, "errors_present": False}})
    monkeypatch.setattr(r, "_task_history", lambda _: ({}, False))
    cfg = {"vault_root": str(base)}
    for result in (
        r.read_workview_tasks("synthetic", [], cfg),
        r.build_wait_census(cfg, council_root="synthetic"),
    ):
        assert result["dark"] is False
        assert result["partial"] is False
        assert result["universe"]["known_work"] == 0


def test_missing_cc_preserves_coord_with_request_sources_in_both_views(tmp_path, monkeypatch):
    base = tmp_path / "vault"
    for state in ("active", "closed"):
        (base / "20-projects" / "hapax-requests" / state).mkdir(parents=True)
    monkeypatch.setattr(r, "_projection", lambda _, **kw: {"tasks": {"coord-only": {"status": "open"}}, "_replay": {"source": "sqlite", "degraded": False, "errors_present": False}})
    monkeypatch.setattr(r, "_task_history", lambda _: ({}, False))
    cfg = {"vault_root": str(base)}
    tasks = r.read_workview_tasks("synthetic", [], cfg)
    census = r.build_wait_census(cfg, council_root="synthetic")
    for result in (tasks, census):
        assert result["dark"] is False
        assert result["partial"] is True
        assert result["universe"]["active_distinct_task_ids"] == 1
        assert result["universe"]["known_work"] is None
    assert tasks["tasks"][0]["task_id"] == "coord-only"
    assert tasks["wait_band_counts"] == census["wait_bands"]
    assert tasks["universe"] == census["universe"]


def test_workview_does_not_reread_history_from_another_generation(tmp_path, monkeypatch):
    base = _empty_vault(tmp_path)
    monkeypatch.setattr(r, "_projection", lambda _, **kw: {"tasks": {"coord-only": {"status": "open"}}, "_replay": {"source": "sqlite", "degraded": False, "errors_present": False}})

    def unavailable(_):
        raise AssertionError("Workview must use the history from its projection replay")

    monkeypatch.setattr(r, "_task_history", unavailable)
    cfg = {"vault_root": str(base)}
    tasks = r.read_workview_tasks("synthetic", [], cfg)
    census = r.build_wait_census(cfg, council_root="synthetic")
    assert tasks["universe"] == census["universe"]
    assert tasks["completeness"]["sources"]["coord"] == "live"
    assert tasks["tasks"][0]["task_id"] == "coord-only"


def _real_event():
    from hapax.spine.coord_event_log import CoordEvent

    return CoordEvent(
        event_id="workview-test-event",
        timestamp="2026-10-20T10:00:00Z",
        event_type="sdlc.stage_transition",
        actor="test",
        subject="mirror-only",
        payload={"from_stage": "S1", "to_stage": "S2"},
    )


def test_real_corrupt_sqlite_empty_mirror_is_partial(tmp_path, monkeypatch):
    _assert_degraded_replay(tmp_path, monkeypatch, with_event=False)


def test_real_corrupt_sqlite_retains_visible_mirror_rows_as_partial(tmp_path, monkeypatch):
    _assert_degraded_replay(tmp_path, monkeypatch, with_event=True)


def _assert_degraded_replay(tmp_path, monkeypatch, *, with_event):
    import json
    from hapax.spine.coord_event_log import CoordEventLog

    base = _empty_vault(tmp_path)
    db = tmp_path / "ledger.db"
    db.write_bytes(b"not a sqlite database")
    mirror = tmp_path / "ledger.jsonl"
    event = _real_event()
    mirror.write_text(json.dumps(event.to_record()) + "\n" if with_event else "")
    log = CoordEventLog(db_path=db, jsonl_path=mirror, spool_dir=tmp_path / "spool")
    replay = log.replay(fail_open=True)
    assert replay.degraded is True and replay.source == "jsonl_mirror"
    monkeypatch.setattr("hapax.spine.coord_event_log.default_event_log", lambda: log)
    cfg = {"vault_root": str(base)}
    # Metadata is opt-in; preexisting consumers keep the exact projection record shape.
    assert set(r._projection("synthetic")) == {"tasks"}
    with patch.object(log, "replay", wraps=log.replay) as observed:
        tasks = r.read_workview_tasks("synthetic", [], cfg)
        assert observed.call_count == 1
    with patch.object(log, "replay", wraps=log.replay) as observed:
        census = r.build_wait_census(cfg, council_root="synthetic")
        assert observed.call_count == 1
    for result in (tasks, census):
        assert result["dark"] is False
        assert result["partial"] is True
        assert result["completeness"]["sources"]["coord"] == "partial"
        assert result["completeness"]["provenance"]["coord"]["source"] == "jsonl_mirror"
        assert result["universe"]["known_work"] is None
        assert result["universe"]["active_distinct_task_ids"] == int(with_event)
    assert tasks["universe"] == census["universe"]
    if with_event:
        assert tasks["tasks"][0]["task_id"] == "mirror-only"
        assert tasks["tasks"][0]["prior_stage_state"] != "origin"


def test_real_healthy_sqlite_projection_is_complete_with_single_replay(tmp_path, monkeypatch):
    from hapax.spine.coord_event_log import CoordEventLog, CoordWriter

    base = _empty_vault(tmp_path)
    log = CoordEventLog(
        db_path=tmp_path / "ledger.db",
        jsonl_path=tmp_path / "ledger.jsonl",
        spool_dir=tmp_path / "spool",
    )
    log.append(_real_event(), writer=CoordWriter.daemon("test"))
    monkeypatch.setattr("hapax.spine.coord_event_log.default_event_log", lambda: log)
    cfg = {"vault_root": str(base)}
    for reader in (
        lambda: r.read_workview_tasks("synthetic", [], cfg),
        lambda: r.build_wait_census(cfg, council_root="synthetic"),
    ):
        with patch.object(log, "replay", wraps=log.replay) as observed:
            result = reader()
            assert observed.call_count == 1
        assert result["partial"] is False
        assert result["completeness"]["provenance"]["coord"]["source"] == "sqlite"
        assert result["universe"]["known_work"] == 1
