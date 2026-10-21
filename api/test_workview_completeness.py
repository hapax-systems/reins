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
    def test_unreadable_active_candidate_is_counted_as_unobserved(self):
        dirs = source_dirs(["good.md", "unreadable.md"])

        def loader(path):
            if path.name == "good.md":
                return {"task_id": "good", "status": "open"}, "", 0
            return None, "", 0

        with (
            patch.object(r, "_resolve_workview_dirs", return_value=dirs),
            patch.object(r, "_load_yaml_frontmatter_and_log", side_effect=loader),
            patch.object(r, "_projection", return_value={"tasks": {}}),
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
            patch.object(r, "_projection", return_value={"tasks": {"coord-only": {"status": "open"}}}),
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
            patch.object(r, "_projection", return_value={"tasks": {}}),
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
