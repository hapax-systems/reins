"""Release-arm (sdlc.authorization_flip) as a reins-local text transform.

``release_authorized`` is SDLC machinery. The cockpit already maps ``arm`` to
this flip. This module finds the target cc-task note and applies the same
frontmatter change ``apply_release_auto_arm`` would: the field becomes true,
stage advances to S7_RELEASE, the head stamp is recorded when given.

It does not import council. It does not mint authority. It refuses when the
row is not a release-arm subject or is not implementation-authorized.
"""

from __future__ import annotations

import os
import re
from pathlib import Path


def cc_task_root() -> Path:
    override = os.environ.get("REINS_CC_TASK_ROOT", "").strip()
    if override:
        return Path(override).expanduser()
    return Path.home() / "Documents" / "Personal" / "20-projects" / "hapax-cc-tasks" / "active"


def find_task_note(task_id: str, *, root: Path | None = None) -> Path | None:
    root = root or cc_task_root()
    if not task_id or "/" in task_id or task_id.startswith("."):
        return None
    candidate = root / f"{task_id}.md"
    return candidate if candidate.is_file() else None


def assess_arm(front: str) -> tuple[bool, str]:
    """Return (eligible, reason). reason is legal_next when not eligible."""

    if not re.search(r"(?m)^release_authorized:", front):
        return False, "task is not a release-arm subject (no release_authorized field); add the field or use autoqueue"
    if re.search(r"(?m)^release_authorized:\s*true\s*$", front, flags=re.I):
        return True, "already-armed"
    impl = re.search(r"(?m)^implementation_authorized:\s*(\S+)\s*$", front)
    if impl is None or impl.group(1).strip().lower() not in {"true", "yes", "1"}:
        return False, "implementation_authorized is not true; the row is not authorized-in-principle"
    return True, "eligible"


def apply_arm(
    note_text: str,
    *,
    now_iso: str,
    role: str,
    head_sha: str | None = None,
) -> str:
    if not note_text.startswith("---"):
        raise ValueError("cc-task note must start with frontmatter")
    end = note_text.find("\n---", 4)
    if end < 0:
        raise ValueError("cc-task frontmatter must close")
    front, body = note_text[: end + 1], note_text[end + 1 :]
    front = re.sub(r"(?m)^release_authorized:\s*.*$", "release_authorized: true", front, count=1)
    if head_sha:
        line = f"release_authorized_head_sha: {head_sha}"
        if re.search(r"(?m)^release_authorized_head_sha:", front):
            front = re.sub(r"(?m)^release_authorized_head_sha:\s*.*$", line, front, count=1)
        else:
            front = front.rstrip("\n") + "\n" + line + "\n"
    if re.search(r"(?m)^stage:", front):
        front = re.sub(r"(?m)^stage:\s*.*$", "stage: S7_RELEASE", front, count=1)
    else:
        front = front.rstrip("\n") + "\nstage: S7_RELEASE\n"
    if re.search(r"(?m)^updated_at:", front):
        front = re.sub(r"(?m)^updated_at:\s*.*$", f"updated_at: {now_iso}", front, count=1)
    log = f"- {now_iso} {role}: release arm via POST /command/arm — release_authorized -> true, stage -> S7_RELEASE."
    body = body.rstrip("\n") + "\n" + log + "\n"
    return front + body


def arm_task(
    task_id: str,
    *,
    now_iso: str,
    role: str = "reins-command-arm",
    head_sha: str | None = None,
    root: Path | None = None,
) -> tuple[str, str]:
    """Apply the flip. Returns (status, detail). status is ok|already-armed|refused."""

    path = find_task_note(task_id, root=root)
    if path is None:
        return "refused", f"no active cc-task note named {task_id}"
    text = path.read_text(encoding="utf-8")
    end = text.find("\n---", 4)
    front = text[: end + 1] if end > 0 else text
    eligible, reason = assess_arm(front)
    if not eligible:
        return "refused", reason
    if reason == "already-armed":
        return "already-armed", reason
    path.write_text(apply_arm(text, now_iso=now_iso, role=role, head_sha=head_sha), encoding="utf-8")
    return "ok", f"armed {task_id}"
