"""Lanebus unread projection (Reins slice 1) — read-only, metadata-only, fixture tree.

The unread fact is the EXISTING filesystem fact, recomputed on every read: no index, no state
model, no bus writes."""

from __future__ import annotations

import json
import os
from datetime import datetime, timezone
from pathlib import Path

import pytest

import facet_registry as fr
from reins_read import build_app, instance_config, read_lanebus_summary

#: A fixed clock: ``now`` is a parameter, so the ages below are exact.
NOW = 1_800_000_000.0

#: ``chmod 000`` does not deny root, so an unreadable input cannot be CONSTRUCTED there: the scan
#: succeeds, the row is not "unknown", and the test would report a pass it did not earn. Skip rather
#: than assert something the runner cannot set up.
requires_non_root = pytest.mark.skipif(
    hasattr(os, "geteuid") and os.geteuid() == 0,
    reason="chmod 000 does not deny root: the unreadable-input cases cannot be constructed",
)


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
    """(relpath, size, mtime_ns, inode) per path: a write anywhere changes it."""
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
    """age(f) = now - mtime(f), oldest == max age == now - MIN(mtime). The opposite reading
    (now - max(mtime)) reports the NEWEST unread file, so the two differ here."""
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


def test_count_only_withholds_names_but_keeps_counts(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "operator-name-20260101.md", NOW - 10)

    cfg = {"lanebus_inboxes": [str(root)], "lanebus_count_only_inboxes": [str(root)]}
    row = _row(read_lanebus_summary(cfg, [], now=NOW))

    assert row["unread_count"] == 1
    assert row["oldest_unread_age_s"] == 10.0
    assert all("basename" not in u for u in row["unread"])
    assert "operator-name-20260101.md" not in json.dumps(row)

    # an inbox named ONLY in the count-only list is still configured, and still withheld
    only = read_lanebus_summary({"lanebus_count_only_inboxes": [str(root)]}, [], now=NOW)
    assert only["dark"] is False
    assert _row(only)["unread_count"] == 1
    assert all("basename" not in u for u in _row(only)["unread"])


@requires_non_root
def test_projection_never_reads_a_body(tmp_path):
    """A body that CANNOT be read still projects: a stat-only read cannot fail here, while opening
    the message raises or drops it."""
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
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    _msg(root / "read", "b.md", NOW - 10)
    before = _tree(tmp_path)

    read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW)

    assert _tree(tmp_path) == before


def test_the_three_name_bearing_fields_never_air_in_the_registry():
    """A filename and a configured directory name are free-text-chosen by whoever made them, so
    both deny on air whatever facet they classify into; the inbox path already did."""
    assert fr.classify("LanebusUnread", "basename") == "identity"
    assert fr.classify("LanebusInbox", "inbox") == "place"
    assert fr.classify("LanebusInbox", "path") == "place"
    for domain, attr in (("LanebusUnread", "basename"), ("LanebusInbox", "inbox"), ("LanebusInbox", "path")):
        assert fr.air_policy(domain, attr) == "deny"
        assert attr not in set(fr.air_allowlist())


def test_an_allowlist_entry_cannot_re_air_the_path_filename_or_inbox_name(tmp_path):
    """classify_air() consults only the allowlist, so an explicit entry would re-expose these
    three; the registry deny is per-attribute policy, re-applied here."""
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    allowlist = ["basename", "path", "inbox", "mtime", "age_s", "unread_count"]

    row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, allowlist, now=NOW))

    assert row["air"]["path"] == "deny"
    assert row["air"]["inbox"] == "deny"
    assert row["unread"][0]["air"]["basename"] == "deny"
    assert row["air"]["unread_count"] == "ok"  # a non-sensitive override still wins


def test_normalized_config_entries_are_scanned_not_reported_missing(tmp_path):
    """The LIVE seam: instance_config() hands the app {path, filenames} entries, not bare strings.
    Stringifying one turns a real inbox into a reported-missing one."""
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)

    data = read_lanebus_summary(
        {"lanebus_inboxes": [{"path": str(root), "filenames": "metadata"}]}, [], now=NOW
    )
    row = _row(data)

    assert row["exists"] is True
    assert row["state"] == "unread"
    assert row["unread_count"] == 1
    withheld = read_lanebus_summary(
        {"lanebus_inboxes": [{"path": str(root), "filenames": "count-only"}]}, [], now=NOW
    )
    assert all("basename" not in u for u in _row(withheld)["unread"])


def test_the_whole_seam_from_config_file_to_endpoint(tmp_path, monkeypatch):
    """config.toml -> instance_config() -> build_app() -> /read/lanebus. The projection normalizes
    to {path, filenames} and the app hands those entries straight back in, so a projection taking
    only bare strings reported every inbox as missing while every unit test passed."""
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    cfg_file = tmp_path / "config.toml"
    cfg_file.write_text(f'lanebus_inboxes = ["{root}"]\n', encoding="utf-8")
    monkeypatch.setenv("REINS_CONFIG", str(cfg_file))
    monkeypatch.delenv("REINS_LANEBUS_INBOXES", raising=False)
    monkeypatch.delenv("REINS_LANEBUS_COUNT_ONLY_INBOXES", raising=False)

    cfg = instance_config()
    # the config-file path AND the normalized shape the app is handed directly
    for session_cfg in (cfg, {"lanebus_inboxes": [{"path": str(root), "filenames": "metadata"}]}):
        app = build_app(cfg["council_root"], cfg["allowlist"], session_cfg)
        endpoint = next(
            r.endpoint for r in app.routes if getattr(r, "path", "") == "/read/lanebus"
        )
        row = _row(endpoint())
        assert row["exists"] is True
        assert row["state"] == "unread"
        assert row["unread_count"] == 1


def test_a_non_canonical_spelling_cannot_escape_the_count_only_narrowing(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)

    cfg = {"lanebus_inboxes": [f"{root}/."], "lanebus_count_only_inboxes": [str(root)]}
    row = _row(read_lanebus_summary(cfg, [], now=NOW))

    assert row["unread_count"] == 1
    assert all("basename" not in u for u in row["unread"])


def test_an_unrecognized_filename_policy_narrows_to_count_only(tmp_path):
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)

    cfg = {"lanebus_inboxes": [{"path": str(root), "filenames": "everything"}]}

    assert all("basename" not in u for u in _row(read_lanebus_summary(cfg, [], now=NOW))["unread"])


@requires_non_root
def test_an_unreadable_inbox_is_unknown_never_a_false_clear(tmp_path):
    """An unreadable inbox is UNKNOWN, not clear: `clear` with zero counts asserts "no unread mail"
    about a directory never read."""
    good = _inbox(tmp_path, "good")
    _msg(good, "a.md", NOW - 10)
    sealed = _inbox(tmp_path, "sealed")
    _msg(sealed, "b.md", NOW - 10)
    sealed.chmod(0o000)
    try:
        data = read_lanebus_summary({"lanebus_inboxes": [str(good), str(sealed)]}, [], now=NOW)
        row = _row(data, "sealed")

        assert row["exists"] is True
        assert row["state"] == "unknown"
        assert row["read_dir"] == "unknown"
        assert row["files_present"] is None
        assert row["files_acked"] is None
        assert row["unread_count"] is None
        assert row["oldest_unread_age_s"] is None
        assert row["unread"] == []
        assert data["totals"]["unknown_inboxes"] == 1
        assert data["totals"]["unread_count"] == 1  # only the MEASURED inbox contributed

        # and a summary whose ONLY inbox is unmeasured reports no total at all, never zero
        alone = read_lanebus_summary({"lanebus_inboxes": [str(sealed)]}, [], now=NOW)
        assert alone["totals"]["unknown_inboxes"] == 1
        assert alone["totals"]["unread_count"] is None
        assert alone["totals"]["files_present"] is None
    finally:
        sealed.chmod(0o700)


@requires_non_root
def test_an_unreadable_receipt_dir_is_unknown_not_missing(tmp_path):
    """The inbox lists fine but ``read/`` cannot be read: an empty ack set would turn every message
    into unread."""
    root = _inbox(tmp_path)
    _msg(root, "a.md", NOW - 10)
    _msg(root / "read", "b.md", NOW - 10)
    (root / "read").chmod(0o000)
    try:
        row = _row(read_lanebus_summary({"lanebus_inboxes": [str(root)]}, [], now=NOW))

        assert row["files_present"] == 1
        assert row["read_dir"] == "unknown"
        assert row["files_acked"] is None
        assert row["unread_count"] is None
        assert row["oldest_unread_age_s"] is None
        assert row["state"] == "unknown"
    finally:
        (root / "read").chmod(0o700)


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
