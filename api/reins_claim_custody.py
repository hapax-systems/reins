"""Read-only custody projection over explicitly bound task rows and claim markers.

``claim_custody`` binds absolute ``rows_dirs``, a finite ``task_ids`` list, and
optional ``claim_markers`` entries {role, path}. No paths are inferred. Selected
rows may reside in active or closed directories. Only top-level metadata and
recognized session-log event shapes leave this reader; raw prose never does.

The bounded deque is a disposable history of process observations, not a durable
ledger. A restart loses it. Source-reported events retain their own timestamps
and provenance; a started-work inference never asserts that a guard was run.
"""

from __future__ import annotations

import copy
import os
import re
import stat
import threading
import time
from collections import deque
from datetime import datetime
from pathlib import Path

import yaml

MAX_ROWS = 128
MAX_BYTES = 256 * 1024
MAX_EVENTS = 32
MAX_HISTORY = 256
_ID = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,191}\Z")
_NULL = {"", "null", "none", "nil", "~"}
_STATUSES = {
    "offered",
    "claimed",
    "in_progress",
    "blocked",
    "pr_open",
    "merge_queue",
    "merged",
    "merged_awaiting_runtime_witness",
    "done",
    "closed",
    "superseded",
    "withdrawn",
    "cancelled",
    "paused",
    "held",
}
_REMEDY = "Work that has started is not returned: finish it and run cc-close, or close it as withdrawn."
_EVENT = re.compile(
    r"^- (\d{4}-\d\d-\d\dT\S+) ([A-Za-z0-9][A-Za-z0-9_.-]{0,191}):? (.*)$"
)
_RELATIONS = ("depends_on", "related", "supersedes", "superseded_by")


def _stamp(now: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now))


def _identifier(value: object) -> str:
    if not isinstance(value, str) or not _ID.fullmatch(value):
        raise ValueError("invalid identifier")
    return value


def _absolute(value: object) -> Path:
    if not isinstance(value, str) or not value.strip():
        raise TypeError("missing binding")
    path = Path(value).expanduser()
    if not path.is_absolute():
        raise ValueError("relative binding")
    return path


def _binding(spec: object) -> tuple[list[Path], list[str], list[tuple[str, Path]]]:
    if not isinstance(spec, dict):
        raise TypeError("missing binding")
    roots, tasks, markers = (
        spec.get("rows_dirs"),
        spec.get("task_ids"),
        spec.get("claim_markers", []),
    )
    if not isinstance(roots, list) or not 1 <= len(roots) <= 8:
        raise ValueError("rows binding")
    if not isinstance(tasks, list) or not 1 <= len(tasks) <= MAX_ROWS:
        raise ValueError("task binding")
    if not isinstance(markers, list) or len(markers) > MAX_ROWS:
        raise ValueError("marker binding")
    tasks = [_identifier(t) for t in tasks]
    if len(set(tasks)) != len(tasks):
        raise ValueError("duplicate task")
    return (
        [_absolute(p) for p in roots],
        tasks,
        [(_identifier(m["role"]), _absolute(m["path"])) for m in markers],
    )


def _read(path: Path) -> str:
    # O_NONBLOCK keeps a substituted FIFO from hanging an API request. fstat checks
    # the opened object; O_NOFOLLOW rejects final-component symlinks before reads.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
            raise ValueError("not regular")
        data = stream.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES:
        raise ValueError("source too large")
    return data.decode("utf-8")


def _metadata(text: str) -> tuple[dict, str]:
    lines = text.splitlines()
    if not lines or lines[0] != "---":
        raise ValueError("frontmatter missing")
    end = lines.index("---", 1)
    raw = "\n".join(lines[1:end])
    # Reject aliases before composing: a bounded byte input need not imply a bounded
    # expanded YAML graph. BaseLoader leaves timestamps and null spellings as text.
    if any(
        isinstance(t, (yaml.tokens.AliasToken, yaml.tokens.AnchorToken))
        for t in yaml.scan(raw)
    ):
        raise ValueError("aliases unsupported")
    node = yaml.compose(raw, Loader=yaml.BaseLoader)
    if not isinstance(node, yaml.MappingNode):
        raise TypeError("frontmatter not mapping")
    out = {}
    for key, value in node.value:
        if not isinstance(key, yaml.ScalarNode) or key.value in out:
            raise ValueError("duplicate or non-scalar key")
        out[key.value] = value
    return out, "\n".join(lines[end + 1 :])


def _scalar(meta: dict, key: str, default: str = "") -> str:
    node = meta.get(key)
    if node is None:
        return default
    if not isinstance(node, yaml.ScalarNode):
        raise TypeError("non-scalar field")
    return node.value.strip()


def _relations(meta: dict) -> list[dict]:
    relations = []
    for key in _RELATIONS:
        node = meta.get(key)
        if node is None:
            continue
        values = node.value if isinstance(node, yaml.SequenceNode) else [node]
        if len(values) > MAX_ROWS:
            raise ValueError("too many relations")
        for item in values:
            if not isinstance(item, yaml.ScalarNode):
                raise TypeError("non-scalar relation")
            if item.value.lower() not in _NULL:
                relations.append({"relation": key, "target": _identifier(item.value)})
    return relations


def _events(body: str) -> tuple[list[dict], bool]:
    events = []
    in_log = False
    for line in body.splitlines():
        if line.startswith("## "):
            in_log = line.strip() == "## Session log"
        match = _EVENT.fullmatch(line) if in_log else None
        if not match:
            continue
        ts, actor, detail = match.groups()
        try:
            dt = datetime.fromisoformat(ts)
            if dt.utcoffset() is None:
                continue
        except ValueError:
            continue
        kind, guard, remedy, target = "", "", "", ""
        if "refused" in detail and "claim_return_started" in detail:
            kind, guard, remedy = "return-refused", "claim_return_started", _REMEDY
        elif detail.startswith("claimed (cc-claim,"):
            kind = "claimed"
        elif detail.startswith("closed as superseded:") and detail.endswith(
            "(cc-close)"
        ):
            kind = "superseded"
            successor = re.search(
                r"demand (?:preserved|retained) in ([A-Za-z0-9][A-Za-z0-9_.-]{0,191})(?:\s|$)",
                detail,
            )
            target = successor.group(1) if successor else ""
        if kind:
            events.append(
                {
                    "ts": ts,
                    "actor": actor,
                    "kind": kind,
                    "guard": guard,
                    "remedy": remedy,
                    "target": target,
                    "confidence": "reported",
                }
            )
    events.sort(key=lambda e: datetime.fromisoformat(e["ts"]))
    return events[-MAX_EVENTS:], len(events) > MAX_EVENTS


def _markers(bindings: list[tuple[str, Path]]) -> tuple[dict[str, set[str]], bool]:
    held: dict[str, set[str]] = {}
    complete = True
    for role, path in bindings:
        try:
            task = _identifier(_read(path).strip())
            held.setdefault(task, set()).add(role)
        except FileNotFoundError:
            # A missing parent means an unavailable source, not a missing marker.
            complete = complete and path.parent.is_dir()
        except (OSError, ValueError, UnicodeError):
            complete = False
    return held, complete


def _row(
    task: str,
    roots: list[Path],
    held: dict[str, set[str]],
    markers_ok: bool,
    configured: bool,
) -> dict:
    found = []
    for root in roots:
        try:
            found.append(_read(root / (task + ".md")))
        except FileNotFoundError:
            continue
    if len(found) != 1:
        raise ValueError("row missing or ambiguous")
    meta, body = _metadata(found[0])
    if _scalar(meta, "task_id") != task or _scalar(meta, "status") not in _STATUSES:
        raise ValueError("row identity or status unavailable")
    assigned = _scalar(meta, "assigned_to")
    owner = "" if assigned.lower() in _NULL | {"unassigned"} else _identifier(assigned)
    started = any(_scalar(meta, k).lower() not in _NULL for k in ("branch", "pr"))
    status = _scalar(meta, "status")
    events, truncated = _events(body)
    holders = sorted(held.get(task, set()))
    return {
        "task_id": task,
        "state": "ok",
        "status": status,
        "assigned_to": owner,
        "started": started,
        "return_state": (
            "not-returnable-status"
            if status not in {"claimed", "in_progress"}
            else "started-work-hold"
            if started
            else "unstarted-candidate"
        ),
        "guard": "claim_return_started" if started else "",
        "remedy": _REMEDY if started else "",
        "confidence": "inferred",
        "claim_state": (
            "unconfigured"
            if not configured
            else "claim-source-unavailable"
            if not markers_ok
            else "conflict"
            if holders and holders != [owner]
            else "present"
            if holders
            else "absent"
        ),
        "holders": holders,
        "relations": _relations(meta),
        "recorded_events": events,
        "events_truncated": truncated,
    }


class CustodyProjection:
    """One fixed binding per app; append observations, evict oldest at a hard cap.

    No file-backed store and no lifecycle import. Failed source observations never
    replace the last good comparison baseline. Returned structures are detached
    copies so a consumer cannot rewrite retained history.
    """

    def __init__(self, spec: object, capacity: int = MAX_HISTORY):
        if type(capacity) is not int or not 1 <= capacity <= MAX_HISTORY:
            raise ValueError("invalid history capacity")
        try:
            self.binding = _binding(copy.deepcopy(spec))
        except (KeyError, TypeError, ValueError):
            self.binding = None
        self._history: deque = deque(maxlen=capacity)
        self._prior: dict[str, dict] = {}
        self._sequence = 0
        self._lock = threading.Lock()

    def read(
        self, allowlist: list[str] | None = None, now: float | None = None
    ) -> dict:
        with self._lock:
            return self._read(allowlist or [], time.time() if now is None else now)

    def _read(self, allowlist: list[str], now: float) -> dict:
        rows, state = [], "ok"
        if self.binding is None:
            state = "custody-binding-unavailable"
        else:
            roots, tasks, bindings = self.binding
            try:
                # Listability matters: is_dir alone would turn an unreadable directory
                # into a collection of ordinary missing rows.
                for root in roots:
                    if not stat.S_ISDIR(root.lstat().st_mode):
                        raise OSError("not directory")
                    with os.scandir(root) as entries:
                        next(entries, None)
            except OSError:
                state = "rows-source-unavailable"
            if state == "ok":
                held, complete = _markers(bindings)
                for task in tasks:
                    try:
                        row = _row(task, roots, held, complete, bool(bindings))
                    except (
                        OSError,
                        TypeError,
                        ValueError,
                        UnicodeError,
                        yaml.YAMLError,
                        RecursionError,
                    ):
                        row = {"task_id": task, "state": "row-unavailable"}
                    rows.append(row)
                    if row["state"] != "ok":
                        state = "partial"
                        continue
                    prior = self._prior.get(task)
                    if row != prior:
                        self._sequence += 1
                        self._history.append(
                            {
                                "sequence": self._sequence,
                                "observed_at": _stamp(now),
                                "kind": "first-observed"
                                if prior is None
                                else "custody-changed",
                                "task_id": task,
                                "prior": prior,
                                "current": row,
                            }
                        )
                        self._prior[task] = row
        result = copy.deepcopy(
            {
                "state": state,
                "observed_at": _stamp(now),
                "rows": rows,
                "history_scope": "process-observations",
                "transitions": list(self._history),
                "dropped": max(0, self._sequence - len(self._history)),
            }
        )

        # Use the same field AIR contract as the READ API; identities and payload
        # containers remain denied even if a broad instance allowlist names them.
        def classify(value):
            if isinstance(value, list):
                return [classify(item) for item in value]
            elif isinstance(value, dict):
                fields = {key: classify(item) for key, item in value.items()}
                safe = {
                    "state",
                    "status",
                    "started",
                    "return_state",
                    "guard",
                    "confidence",
                    "claim_state",
                    "events_truncated",
                    "kind",
                    "sequence",
                    "observed_at",
                    "ts",
                    "history_scope",
                    "dropped",
                    "relation",
                }
                fields["air"] = {
                    key: "ok" if key in safe and key in allowlist else "deny"
                    for key in value
                }
                return fields
            return value

        return classify(result)
