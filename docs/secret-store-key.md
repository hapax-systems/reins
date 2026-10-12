# The FileStore key boundary

`FileStore` keeps two things: the blobs (`<root>/<name>.bin`) and the key that
decrypts them (`<root>/.key`, 32 random bytes, 0600). By default they sit in the
same directory, inside `$HOME`. **One backup set that walks the home directory
therefore carries both halves**, and a ciphertext archive whose key travels with
it is not an encrypted archive.

## What is actually true on this estate, measured 2026-09-16

Not a risk assessment — a measurement, and it is narrower than the audit assumed.

| Backup path | Covers `~/.config/reins/secrets`? | How that was established |
|---|---|---|
| `hapax-backup-gdrive-critical` (restic → off-site) | **No** | Its manifest is an allowlist built in `build_manifest()`: the validated Postgres dump, the PITR directory, the latest Qdrant snapshots, named vault evidence files, and the vault bundle directory. No home sweep, no `.config`. |
| `hapax-backup-local.service` | **No** | `ExecStart=/bin/bash /home/hapax/projects/distro-work/hapax-backup-local.sh` — that directory does not exist. The unit runs nothing. |
| `hapax-backup-remote.service` | **No** | Same missing script. |
| `vault-git-snapshot` | **No** | It commits `~/Documents/Personal`. The store is under `~/.config`, not the vault. |

So the key is not in any backup today. **That is an accident, not a design.**
Two of the four are broken units, and the one that works is an allowlist that
simply has not been asked to include `$HOME` yet. The moment either broken unit
is repaired with a naive `restic backup /home/hapax`, or the critical manifest
grows a home sweep, key and ciphertext leave the machine together.

## The durable fix: move the key out of the tree

`--key-file <path>`, or `REINS_SECRET_KEY_FILE=<path>`, puts the key anywhere
you like while the blobs stay where they are:

```
hapax-secret --key-file /var/lib/reins/secret.key api-openai
REINS_SECRET_KEY_FILE=/var/lib/reins/secret.key hapax-secret --list
```

`is_store_host()` asks the store where its key lives, so the override also
decides whether a command runs here or forwards over ssh, and the flag survives
into the forwarded remote command.

Pick a location that is **on a different backup policy from the blobs**. Putting
it somewhere merely less obvious in `$HOME` buys nothing: the hazard is one
backup set holding both, not the path being guessable.

## What to check when a backup set changes

Any change to a backup manifest, include list, or exclude list should answer:
does this set now contain **both** `<store>/*.bin` and the key file? If it
contains exactly one, the property holds. If it contains both, the archive is
plaintext to anyone who can restore it.

There is deliberately no automated guard here. A guard would have to enumerate
every backup mechanism on the host to be sound, and a guard that checks three of
four is worse than none — it reports a property it cannot establish. What the
table above gives is the measurement and the date it was taken.

## What this does not protect against

- **A process running as the operator.** It can read the key file wherever it
  is, exactly as it can read the store. Same-uid isolation needs a different
  trust domain and is not claimed anywhere in this design.
- **Loss of the key.** There is no escrow. If the key file is lost the blobs are
  unreadable, and `hapax-secret <name>` says so — `integrity_failed`, exit 3 —
  rather than reporting the secret as absent. Back the key up somewhere the
  blobs are not.
