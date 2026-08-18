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


_HEAD_SHA_RE = re.compile(r"\A[0-9a-f]{7,64}\Z")


def validate_head_sha(head_sha: str | None) -> str | None:
    """Return the sha if safe to interpolate, else None."""

    if head_sha is None:
        return None
    text = str(head_sha).strip().lower()
    if not _HEAD_SHA_RE.fullmatch(text):
        return None
    return text


def assess_arm(front: str) -> tuple[bool, str, str]:
    """Return (eligible, reason, legal_next). legal_next is empty when eligible."""

    if not re.search(r"(?m)^release_authorized:", front):
        return (
            False,
            "task is not a release-arm subject (no release_authorized field)",
            "add release_authorized to the note, or skip arm and use autoqueue",
        )
    if re.search(r"(?m)^release_authorized:\s*true\s*$", front, flags=re.I):
        return True, "already-armed", ""
    impl = re.search(r"(?m)^implementation_authorized:\s*(\S+)\s*$", front)
    if impl is None or impl.group(1).strip().lower() not in {"true", "yes", "1"}:
        return (
            False,
            "implementation_authorized is not true",
            "set implementation_authorized: true when the slice is authorized-in-principle, then retry arm",
        )
    return True, "eligible", ""


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
    safe_sha = validate_head_sha(head_sha)
    if safe_sha:
        line = f"release_authorized_head_sha: {safe_sha}"
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


_PR_RE = re.compile(r"\A[0-9]{1,8}\Z")
_CLAIMED_AT_EMPTY = frozenset({"", "null", "~", "none", "false"})


def validate_pr(pr: str | None) -> str | None:
    """Return a decimal PR number safe to interpolate, else None."""

    if pr is None:
        return None
    text = str(pr).strip()
    if not _PR_RE.fullmatch(text):
        return None
    return text


def closed_root(active_root: Path) -> Path:
    if active_root.name == "active":
        return active_root.parent / "closed"
    return active_root / "closed"


def find_closed_note(task_id: str, *, root: Path | None = None) -> Path | None:
    root = root or cc_task_root()
    if not task_id or "/" in task_id or task_id.startswith("."):
        return None
    candidate = closed_root(root) / f"{task_id}.md"
    return candidate if candidate.is_file() else None


def _front_field(front: str, name: str) -> str:
    match = re.search(rf"(?m)^{re.escape(name)}:\s*(.*)$", front)
    if match is None:
        return ""
    return match.group(1).strip().strip("'\"")


def assess_close(task_id: str, *, root: Path | None = None) -> tuple[bool, str, str]:
    """Return (eligible, reason, legal_next). legal_next is empty when eligible."""

    root = root or cc_task_root()
    if find_closed_note(task_id, root=root) is not None:
        return (
            False,
            "task is already closed",
            "already closed — nothing to do; the note is in closed/",
        )
    path = find_task_note(task_id, root=root)
    if path is None:
        return (
            False,
            "no active cc-task note named " + task_id,
            "mint or restore the active note, then retry close",
        )
    text = path.read_text(encoding="utf-8")
    end = text.find("\n---", 4)
    front = text[: end + 1] if end > 0 else text
    claimed = _front_field(front, "claimed_at").lower()
    if claimed in _CLAIMED_AT_EMPTY:
        return (
            False,
            "task is unclaimed",
            "unclaimed — cc-claim the task first, then retry close",
        )
    status = _front_field(front, "status").lower()
    if status in {"done", "withdrawn", "superseded"}:
        return (
            False,
            "task is already closed",
            "already closed — nothing to do; the note already carries a terminal status",
        )
    return True, "eligible", ""


def apply_close(
    note_text: str,
    *,
    now_iso: str,
    role: str,
    pr: str | None = None,
    status: str = "done",
) -> str:
    if not note_text.startswith("---"):
        raise ValueError("cc-task note must start with frontmatter")
    end = note_text.find("\n---", 4)
    if end < 0:
        raise ValueError("cc-task frontmatter must close")
    front, body = note_text[: end + 1], note_text[end + 1 :]
    if status not in {"done", "withdrawn", "superseded"}:
        status = "done"
    if re.search(r"(?m)^status:", front):
        front = re.sub(r"(?m)^status:\s*.*$", f"status: {status}", front, count=1)
    else:
        front = front.rstrip("\n") + f"\nstatus: {status}\n"
    if re.search(r"(?m)^completed_at:", front):
        front = re.sub(r"(?m)^completed_at:\s*.*$", f"completed_at: {now_iso}", front, count=1)
    else:
        front = front.rstrip("\n") + f"\ncompleted_at: {now_iso}\n"
    if re.search(r"(?m)^updated_at:", front):
        front = re.sub(r"(?m)^updated_at:\s*.*$", f"updated_at: {now_iso}", front, count=1)
    safe_pr = validate_pr(pr)
    if safe_pr:
        line = f"pr: {safe_pr}"
        if re.search(r"(?m)^pr:", front):
            front = re.sub(r"(?m)^pr:\s*.*$", line, front, count=1)
        else:
            front = front.rstrip("\n") + "\n" + line + "\n"
    log = f"- {now_iso} {role} closed as {status}"
    if safe_pr:
        log += f" (PR #{safe_pr})"
    log += " (reins POST /command/close)."
    if "## Session log" in body:
        body = body.replace("## Session log\n", "## Session log\n" + log + "\n", 1)
    else:
        body = body.rstrip("\n") + "\n" + log + "\n"
    return front + body


def _clear_claim_files(task_id: str) -> None:
    cache = Path(os.environ.get("REINS_CLAIM_DIR", "")).expanduser()
    if not cache.is_dir():
        cache = Path.home() / ".cache" / "hapax"
    if not cache.is_dir():
        return
    prefix = "cc-active-task-"
    for path in cache.glob(prefix + "*"):
        try:
            head = path.read_text(encoding="utf-8").splitlines()[:1]
        except OSError:
            continue
        if head and head[0].strip() == task_id:
            path.unlink(missing_ok=True)


def close_task(
    task_id: str,
    *,
    now_iso: str,
    role: str = "reins-command-close",
    pr: str | None = None,
    status: str = "done",
    root: Path | None = None,
    dry_run: bool = False,
) -> tuple[str, str]:
    """Apply the close. Returns (status, detail). status is ok|preview|refused."""

    root = root or cc_task_root()
    eligible, reason, legal_next = assess_close(task_id, root=root)
    if not eligible:
        return "refused", legal_next or reason
    if dry_run:
        return "preview", f"would close {task_id}"
    path = find_task_note(task_id, root=root)
    if path is None:
        return "refused", "mint or restore the active note, then retry close"
    dest_dir = closed_root(root)
    dest_dir.mkdir(parents=True, exist_ok=True)
    dest = dest_dir / path.name
    if dest.exists():
        return "refused", "already closed — nothing to do; the note is in closed/"
    text = apply_close(
        path.read_text(encoding="utf-8"),
        now_iso=now_iso,
        role=role,
        pr=pr,
        status=status,
    )
    tmp = dest.with_suffix(dest.suffix + ".tmp")
    tmp.write_text(text, encoding="utf-8")
    tmp.replace(dest)
    path.unlink()
    _clear_claim_files(task_id)
    return "ok", f"closed {task_id}"


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
    eligible, reason, legal_next = assess_arm(front)
    if not eligible:
        return "refused", legal_next or reason
    if reason == "already-armed":
        return "already-armed", reason
    path.write_text(apply_arm(text, now_iso=now_iso, role=role, head_sha=head_sha), encoding="utf-8")
    return "ok", f"armed {task_id}"
