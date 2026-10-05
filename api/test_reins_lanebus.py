"""Lanebus unread projection (Reins slice 1) — read-only, metadata-only, fixture tree.

The unread fact is the EXISTING filesystem fact: a ``*.md`` file directly in the inbox with no
same-named file under ``read/``. There is no index and no state model — the projection recomputes
from the filesystem on every read and writes nothing, ever (no bus writes).

Scope is pinned here rather than assumed: basename-only join, no recursion outside the inbox root,
symlink/traversal rejection, configured inboxes only (an empty config denies), metadata only (a
message body is never opened), and the oldest-unread age is the MAXIMUM age over the unread set
(``now - min(mtime)``) — null when the unread set is empty.
"""

from __future__ import annotations

import json
import os
from datetime import datetime, timezone
from pathlib import Path

import facet_registry as fr
from reins_read import build_app, instance_config, read_lanebus_summary

#: A fixed clock. ``read_lanebus_summary`` takes ``now`` as a parameter (this codebase threads time
#: rather than reading a global), so ages below are exact and not merely monotonic.
NOW = 1_800_000_000.0


def _inbox(tmp_path: Path, name: str = "lane") -> Path:
    root = tmp_path / name
    root.mkdir(parents=True)
    (root / "read").mkdir()
    return root


def _msg(directory: Path, name: str, mtime: float) -> Path:
    path = directory / name
    path.write_text("SECRET-BODY", encoding="utf-8")
    os.utime(path, (mtime, mtime))
    return path


def _row(data: dict, inbox: str = "lane") -> dict:
    return next(r for r in data["inboxes"] if r["inbox"] == inbox)


def _tree(root: Path) -> list[tuple[str, int, int, int]]:
    """Every path under ``root`` with (relpath, size, mtime_ns, inode) — a write anywhere changes it."""
    out = []
    for dirpath, dirnames, filenames in os.walk(root):
        for name in sorted(dirnames) + sorted(filenames):
            p = Path(dirpath) / name
            st = p.lstat()
            out.append((str(p.relative_to(root)), st.st_size, st.st_mtime_ns, st.st_ino))
    return sorted(out)


def test_unread_is_present_minus_acked(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    _msg(root, "b.md", NOW - 20)
    _msg(root, "c.md", NOW - 30)
    _msg(root / "read", "a.md", NOW - 5)  # same basename under read/ == acked
    _msg(root / "read", "b.md", NOW - 5)

    row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW))

    assert row["files_present"] == 3
    assert row["files_acked"] == 2
    assert row["unread_count"] == 1
    assert [u["basename"] for u in row["unread"]] == ["c.md"]
    assert row["state"] == "unread"
    assert row["exists"] is True
    assert row["read_dir"] == "ok"


def test_oldest_unread_age_is_the_maximum_age_over_the_unread_set(tmp_path):
    """The steward disambiguation: age(f) = now - mtime(f); oldest == max age == now - MIN(mtime).

    The literal opposite reading (now - max(mtime)) would report the NEWEST unread file, so the two
    readings are numerically distinct here and this test fails under the wrong one.
    """
    root = _inbox(tmp_path)
    _msg(root, "oldest.md", NOW - 900)   # the oldest unread
    _msg(root, "middle.md", NOW - 300)
    _msg(root, "newest.md", NOW - 60)
    _msg(root, "acked.md", NOW - 9_999)  # acked: must not contribute to the age
    _msg(root / "read", "acked.md", NOW - 5)

    row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW))

    assert row["unread_count"] == 3
    assert row["oldest_unread_age_s"] == 900.0
    by_name = {u["basename"]: u for u in row["unread"]}
    assert by_name["oldest.md"]["age_s"] == 900.0
    assert by_name["newest.md"]["age_s"] == 60.0
    # the ISO mtime round-trips to the exact instant that produced the age
    stamp = datetime.strptime(by_name["oldest.md"]["mtime"], "%Y-%m-%dT%H:%M:%SZ")
    assert stamp.replace(tzinfo=timezone.utc).timestamp() == NOW - 900


def test_empty_unread_set_reports_null_oldest_age(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    _msg(root / "read", "a.md", NOW - 5)

    data = read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW)
    row = _row(data)

    assert row["unread"] == []
    assert row["unread_count"] == 0
    assert row["oldest_unread_age_s"] is None  # null, never 0.0
    assert row["state"] == "clear"
    assert data["totals"]["oldest_unread_age_s"] is None


def test_unconfigured_denies_by_default(tmp_path):
    _inbox(tmp_path)

    data = read_lanebus_summary({}, [], now=NOW)

    assert data["dark"] is True
    assert data["inboxes"] == []
    assert data["totals"]["inboxes"] == 0


def test_missing_inbox_is_reported_without_darkening_the_rest(tmp_path):
    present = _inbox(tmp_path, "present")
    _msg(present, "a.md", NOW - 10)
    absent = tmp_path / "absent"

    data = read_lanebus_summary({"lanebus_inboxes": [str(present), str(absent)]}, [], now=NOW)

    assert data["dark"] is False
    assert _row(data, "absent")["exists"] is False
    assert _row(data, "absent")["state"] == "missing"
    assert _row(data, "absent")["files_present"] == 0
    assert _row(data, "absent")["unread"] == []
    assert _row(data, "present")["unread_count"] == 1


def test_only_top_level_markdown_files_are_counted(tmp_path):
    """No recursion outside the inbox root, and only ``*.md`` — a nested note is not an inbox message."""
    root = _inbox(tmp_path)
    _msg(root, "top.md", NOW - 10)
    _msg(root, "notes.txt", NOW - 10)
    nested = root / "sub"
    nested.mkdir()
    _msg(nested, "deep.md", NOW - 10)

    row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW))

    assert row["files_present"] == 1
    assert [u["basename"] for u in row["unread"]] == ["top.md"]


def test_symlinked_message_is_rejected(tmp_path):
    root = _inbox(tmp_path)
    outside = tmp_path / "outside.md"
    outside.write_text("SECRET-BODY", encoding="utf-8")
    os.symlink(outside, root / "link.md")
    _msg(root, "real.md", NOW - 10)

    row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW))

    assert row["files_present"] == 1
    assert [u["basename"] for u in row["unread"]] == ["real.md"]


def test_symlinked_read_dir_is_rejected_not_followed(tmp_path):
    """A ``read/`` that resolves outside the inbox root is an escape: it must not ack anything."""
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    (root / "read").rmdir()
    elsewhere = tmp_path / "elsewhere"
    elsewhere.mkdir()
    _msg(elsewhere, "a.md", NOW - 5)  # would ack a.md if the symlink were followed
    os.symlink(elsewhere, root / "read")

    row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW))

    assert row["read_dir"] == "rejected"
    assert row["files_acked"] == 0
    assert [u["basename"] for u in row["unread"]] == ["a.md"]


def test_a_directory_named_like_a_message_is_not_an_ack(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    (root / "read" / "a.md").mkdir()  # a directory, not a receipt

    row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW))

    assert row["files_acked"] == 0
    assert row["unread_count"] == 1


def test_count_only_policy_omits_basenames_but_keeps_counts(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "operator-name-20260101.md", NOW - 10)

    cfg = {"lanebus_inboxes": [str(root)], "lanebus_count_only_inboxes": [str(root)]}
    row = _row(read_lanebus_summary(cfg, [], now=NOW))

    assert row["unread_count"] == 1
    assert row["oldest_unread_age_s"] == 10.0
    assert all("basename" not in u for u in row["unread"])
    assert "operator-name-20260101.md" not in json.dumps(row)


def test_an_inbox_named_only_as_count_only_is_still_configured_and_withholds_names(tmp_path):
    """The two lists are a union, and the NARROWER policy wins — never the wider one."""
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)

    cfg = {"lanebus_count_only_inboxes": [str(root)]}
    data = read_lanebus_summary(cfg, [], now=NOW)
    row = _row(data)

    assert data["dark"] is False
    assert row["unread_count"] == 1
    assert all("basename" not in u for u in row["unread"])


def test_projection_never_reads_a_body(tmp_path):
    """Metadata only, measured at the read itself: a message whose body CANNOT be read still projects.

    A stat-only projection cannot fail here; an implementation that opens the message either raises
    (PermissionError) or silently drops it from the counts — both visible below. Asserting only that
    the body text is absent from the payload would pass for a projection that reads every body and
    discards it, which is why the body is sealed instead.
    """
    root = _inbox(tmp_path)
    sealed = _msg(root, "a.md", NOW - 10)
    sealed.chmod(0o000)
    try:
        data = read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW)
        assert "SECRET-BODY" not in json.dumps(data)
        assert _row(data)["files_present"] == 1
        assert _row(data)["unread_count"] == 1
    finally:
        sealed.chmod(0o600)


def test_projection_writes_nothing(tmp_path):
    """Read-only is a measured property: the tree (names, sizes, mtimes, inodes) is identical after."""
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    _msg(root / "read", "b.md", NOW - 10)
    before = _tree(tmp_path)

    read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW)

    assert _tree(tmp_path) == before


def test_basename_never_airs_and_the_registry_denies_it():
    assert fr.classify("LanebusUnread", "basename") == "identity"
    assert fr.air_policy("LanebusUnread", "basename") == "deny"
    assert "basename" not in set(fr.air_allowlist())


def test_endpoint_serves_the_projection(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    app = build_app("", [], {"lanebus_inboxes": [str(root)]})
    endpoint = next(
        route.endpoint for route in app.routes if getattr(route, "path", "") == "/read/lanebus"
    )

    data = endpoint()

    assert data["dark"] is False
    assert _row(data)["unread_count"] == 1


def test_instance_config_reads_lanebus_inboxes_from_toml_and_env(tmp_path, monkeypatch):
    cfg_file = tmp_path / "config.toml"
    cfg_file.write_text(
        'lanebus_inboxes = ["/toml/one", "/toml/two"]\n'
        'lanebus_count_only_inboxes = ["/toml/two"]\n',
        encoding="utf-8",
    )
    monkeypatch.setenv("REINS_CONFIG", str(cfg_file))
    monkeypatch.delenv("REINS_LANEBUS_INBOXES", raising=False)
    monkeypatch.delenv("REINS_LANEBUS_COUNT_ONLY_INBOXES", raising=False)

    from_file = instance_config()["lanebus_inboxes"]
    assert from_file == [
        {"path": "/toml/one", "filenames": "metadata"},
        {"path": "/toml/two", "filenames": "count-only"},
    ]

    monkeypatch.setenv("REINS_LANEBUS_INBOXES", "/env/one:/env/two")
    monkeypatch.setenv("REINS_LANEBUS_COUNT_ONLY_INBOXES", "/env/two")
    assert instance_config()["lanebus_inboxes"] == [
        {"path": "/env/one", "filenames": "metadata"},
        {"path": "/env/two", "filenames": "count-only"},
    ]
