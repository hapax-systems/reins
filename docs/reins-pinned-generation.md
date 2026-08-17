# Pinned generations: how the frontdoor API is delivered

The operator's only frontdoor API is `reins-read-api.service`. This document is how a
particular commit becomes the thing that service runs, and — more to the point — what
stops a commit from becoming that.

## The defect this replaces

The base unit ExecStarted from `~/projects/reins/api`: a mutable developer checkout.

```
systemctl --user cat reins-read-api.service
  ExecStart=%h/projects/reins/api/.venv/bin/python %h/projects/reins/api/reins_serve.py
  WorkingDirectory=%h/projects/reins/api

git -C ~/projects/reins rev-parse --abbrev-ref HEAD   ->  beta/k0-closed-world-harvest-20260812
curl :8799/read/meta | .serving_sha                   ->  943e6d8   (Aug-9 main)
```

Measured 2026-08-12. The live process was serving good code purely because nothing had
restarted it since 2026-08-09 11:59. Its next restart — for any reason at all, including
an unrelated `daemon-reload`, an OOM, or a reboot — would have loaded whatever branch
happened to be checked out at that moment.

That is a delivery problem rather than housekeeping, and it sits on R2.1's critical path:
a governed WRITE verb cannot be trusted while the process serving it is whatever someone
last checked out.

## The mechanism

`scripts/reins-release <sha>` builds an immutable generation and makes it `current`:

1. refuse if `~/projects/reins` is dirty — a SHA does not describe a tree with
   uncommitted edits in it
2. refuse if `<sha>` is not an ancestor of `origin/main` — an unmerged branch is the
   condition this script exists to end
3. `git clone` at that exact SHA into `~/.local/share/reins/releases/<sha>/`. A clone,
   never a copy of the working tree, so nothing untracked or ignored travels into a
   generation
4. build the venv with `uv sync`
5. run `pytest -q` inside `api/` — **refuse the release on any failure**
6. flip `~/.local/share/reins/current` to the new generation, atomically, by building the
   link beside the old one and renaming over it so a reader never sees a missing symlink

`systemd/units/reins-read-api.service.d/pinned-generation.conf` overrides `ExecStart`,
`WorkingDirectory` and `PYTHONPATH` to that `current` symlink.

`PYTHONPATH` matters as much as `ExecStart` here. The base unit sets
`PYTHONPATH=%h/projects/reins/api`; pinning only `ExecStart` would leave the developer
checkout importable, so a module some branch adds would still load from the mutable tree.
`sys.path[0]` is the running script's own directory, so same-named modules resolve inside
the generation — meaning the leak is invisible in exactly the cases anyone would think to
test, and shows up only for a file that exists on a branch and not in the generation.

`REINS_COUNCIL_ROOT` and `HAPAX_SPINE_CONFIG_DIR` are deliberately **not** pinned. They
name the estate *data* the API reads — live tasks, live config. Pinning them to a snapshot
would make the frontdoor answer about a frozen estate. The subject is the code that
serves, not the state it reads.

## The refusals are the point

A generation that cannot prove its own tests pass must never become `current`. Every
failure path does strictly **less** than the success path: it leaves `current` where it
is and exits non-zero. There is deliberately no `--force`.

If you genuinely need to serve code whose tests fail, the honest act is to move the
symlink by hand and own that in your shell history, where it is visible, rather than
through a flag that makes it routine.

Pinned by `api/test_reins_release.py`, which drives the real script against synthetic
repos and asserts on `current` rather than on exit codes — an exit code reports what the
script believed, and the symlink decides what the operator is served on the next restart.

## First install

Order matters. The drop-in asserts that `current` exists, so a generation has to exist
first; installing the drop-in against an empty release store puts the unit into `failed`
on its next restart.

```bash
# 1. a tested generation (must be an ancestor of origin/main)
scripts/reins-release "$(git -C ~/projects/reins rev-parse origin/main)"

# 2. the unit and its drop-in
mkdir -p ~/.config/systemd/user/reins-read-api.service.d
cp systemd/units/reins-read-api.service \
   ~/.config/systemd/user/
cp systemd/units/reins-read-api.service.d/pinned-generation.conf \
   ~/.config/systemd/user/reins-read-api.service.d/
systemctl --user daemon-reload
systemctl --user restart reins-read-api

# 3. verify — the sha served must be the sha released, and the unit must run from the pin
curl -s :8799/read/meta | python3 -c 'import json,sys; print(json.load(sys.stdin)["serving_sha"])'
curl -s :8799/read/tasks | head -c 200
systemctl --user cat reins-read-api.service | grep ExecStart
```

`serving_sha` comes from `git rev-parse HEAD` inside the generation, which is a detached
clone at the released SHA — so a mismatch between what you released and what `/read/meta`
reports is a real signal, not a display quirk.

## Routine release

```bash
scripts/reins-release <sha> && systemctl --user restart reins-read-api
```

Old generations are left in place under `~/.local/share/reins/releases/`. That is
deliberate: rollback is a symlink move, and it costs one checkout per release.

## Rollback

```bash
ls ~/.local/share/reins/releases/
ln -sfn ~/.local/share/reins/releases/<older-sha> ~/.local/share/reins/current.new
mv -T ~/.local/share/reins/current.new ~/.local/share/reins/current
systemctl --user restart reins-read-api
```

Rolling back by hand is intentional. It is rare, it should be deliberate, and it should
be legible afterwards in shell history.

## If the unit is `failed`

```
Assertion failed for Reins API ...
```

means `~/.local/share/reins/current/api/reins_serve.py` is not there: either no release
has been built yet, or `current` points at a generation that was deleted. Build one
(`scripts/reins-release <sha>`) or move the symlink to a generation that still exists.

The assertion is on purpose. The alternative — `ConditionPathExists=` — makes systemd
*skip* the unit and report success by omission, so the operator would see no API and no
error at all. Being down loudly is strictly better than being down quietly.

An assertion failure does not enter the restart loop; it fails the start job outright. The
loop that does exist — a `current` that resolves to a broken generation, so `ExecStart`
itself fails — is bounded by `StartLimitBurst`/`StartLimitIntervalSec` in the unit's
`[Unit]` section. Those two keys had been sitting in `[Service]`, where systemd ignored
the interval (measured `StartLimitIntervalUSec=10s` against a file asking for 300s) and
`RestartSec=10s` made 5-starts-per-10s unreachable, so the limiter could never fire. They
were moved on 2026-08-12 alongside this drop-in: adding a new way to fail at start while
leaving a rate limit that cannot trip would have traded one silent problem for another.

## Verifying the whole mechanism

```bash
cd api && uv run pytest -q test_reins_release.py     # the refusals
systemd-analyze --user verify systemd/units/reins-read-api.service   # unit + drop-in compose
```

`systemd-analyze verify` reads `reins-read-api.service.d/` from beside the unit, so it
reports the *merged* `ExecStart` — which is how you check the drop-in is actually
overriding rather than being appended. Before any release exists it also reports
`Command ... is not executable: No such file or directory`, which is the correct answer
to "is the pin live yet".
