"""Stateless seat continuity projection; instance bindings are never inferred.

Optional config table ``seat_succession`` has ``socket`` (tmux socket path; omit
for tmux's normal socket) and ``lanes`` (a list of tables). Each lane declares
``role``, exact ``session``, ``claim_marker_pattern`` (absolute path, filename
glob only), and absolute ``charter_path``. No deployment defaults live here.
Only claim filenames and charter assigned_to are read, never claim contents or
pane contents. Missing bindings/censuses remain unknown, rather than absent.
"""

from __future__ import annotations

import fnmatch
import re
import stat
import subprocess
from collections.abc import Callable
from pathlib import Path

_PANE_FORMAT = (
    "#{session_name}\t#{pane_id}\t#{pane_pid}\t#{pane_current_command}\t#{pane_dead}"
)
_MAX_FRONTMATTER_LINES = 512
_MAX_FRONTMATTER_BYTES = 65536


def _bindings(cfg: dict) -> tuple[dict, list[dict]] | None:
    spec = cfg.get("seat_succession")
    if not isinstance(spec, dict):
        return None
    lanes = spec.get("lanes")
    if not isinstance(lanes, list) or not lanes:
        return None
    socket = spec.get("socket")
    if socket is not None and (not isinstance(socket, str) or not socket):
        return None
    roles = set()
    for lane in lanes:
        if not isinstance(lane, dict):
            return None
        for key in ("role", "session", "claim_marker_pattern", "charter_path"):
            if not isinstance(lane.get(key), str) or not lane[key].strip():
                return None
        if lane["role"] in roles:
            return None
        roles.add(lane["role"])
        pattern = Path(lane["claim_marker_pattern"]).expanduser()
        charter = Path(lane["charter_path"]).expanduser()
        if not pattern.is_absolute() or not charter.is_absolute():
            return None
        if any(c in str(pattern.parent) for c in "*?["):
            return None
    return spec, lanes


def _pane_census(spec: dict) -> dict[str, list[dict]] | None:
    argv = ["tmux"]
    if spec.get("socket"):
        argv += ["-S", str(Path(spec["socket"]).expanduser())]
    argv += ["list-panes", "-a", "-F", _PANE_FORMAT]
    try:
        result = subprocess.run(
            argv, capture_output=True, text=True, check=True, timeout=2
        )
        census: dict[str, list[dict]] = {}
        for line in result.stdout.splitlines():
            session, pane, raw_pid, command, dead = line.split("\t")
            pid = int(raw_pid)
            if pid <= 0 or dead not in {"0", "1"}:
                return None
            if dead == "0":
                census.setdefault(session, []).append(
                    {"id": pane, "pid": pid, "command": command}
                )
        return census
    except (OSError, subprocess.SubprocessError, ValueError, UnicodeError):
        return None


def _is_regular_marker(path: Path) -> bool:
    try:
        return stat.S_ISREG(path.stat().st_mode)
    except FileNotFoundError:
        return False


def _claims(pattern: str) -> dict:
    path = Path(pattern).expanduser()
    try:
        paths = sorted(
            str(p)
            for p in path.parent.iterdir()
            if fnmatch.fnmatchcase(p.name, path.name) and _is_regular_marker(p)
        )
        return {"state": "ok", "present": bool(paths), "paths": paths}
    except OSError:
        return {"state": "claim-census-unavailable", "present": None, "paths": None}


def _assignment(path: str) -> tuple[str, str | None]:
    # The charter contract uses a scalar lane identifier, not arbitrary YAML.
    # Stop at the frontmatter boundary; bodies, YAML tags and aliases are not read.
    try:
        with Path(path).expanduser().open(encoding="utf-8") as stream:
            first = stream.readline(_MAX_FRONTMATTER_BYTES + 1)
            first_bytes = len(first.encode("utf-8"))
            if first_bytes > _MAX_FRONTMATTER_BYTES or first.strip() != "---":
                return "charter-unavailable", None
            values = []
            seen_bytes = first_bytes
            for _ in range(_MAX_FRONTMATTER_LINES):
                line = stream.readline(_MAX_FRONTMATTER_BYTES + 1)
                if not line:
                    return "charter-unavailable", None
                seen_bytes += len(line.encode("utf-8"))
                if seen_bytes > _MAX_FRONTMATTER_BYTES:
                    return "charter-unavailable", None
                if line.strip() == "---":
                    break
                if line.startswith("assigned_to:"):
                    values.append(line.split(":", 1)[1].strip())
            else:
                return "charter-unavailable", None
        if len(values) != 1:
            return "charter-unavailable", None
        value = values[0]
        if value in {"", "null", "~"}:
            return "unassigned", None
        if len(value) >= 2 and value[0] in "\"'" and value[-1] == value[0]:
            value = value[1:-1]
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]*", value):
            return "charter-unavailable", None
        return "ok", value
    except (OSError, UnicodeError):
        return "charter-unavailable", None


def read_seat_succession(cfg: dict, allowlist: list[str], classify: Callable) -> dict:
    """One bounded metadata census; never creates a server or drives a lane.

    This is a current observation, not historical succession detection: comparing
    observations and authorizing a successor remain outside this read surface.
    """
    binding = _bindings(cfg)
    if binding is None:
        return {"state": "seat-binding-unavailable", "rows": []}
    spec, lanes = binding
    census = _pane_census(spec)

    def classified(fields: dict) -> dict:
        return {**fields, "air": classify(fields, allowlist)}

    rows = []
    for lane in lanes:
        panes = census.get(lane["session"], []) if census is not None else None
        charter_state, assigned_to = _assignment(lane["charter_path"])
        fields = {
            "role": lane["role"],
            "session": lane["session"],
            "pane_state": (
                "pane-census-unavailable"
                if panes is None
                else "present"
                if panes
                else "absent"
            ),
            "panes": [classified(p) for p in panes] if panes is not None else None,
            "claims": classified(_claims(lane["claim_marker_pattern"])),
            "charter_state": charter_state,
            "assigned_to": assigned_to,
        }
        rows.append(classified(fields))
    return {
        "state": "ok" if census is not None else "pane-census-unavailable",
        "rows": rows,
    }
