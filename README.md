<!-- hapax-sdlc:preamble:begin -->
<!-- hapax-public:surface=github.repo.reins.readme.preamble:begin -->

# reins

`reins` is the product front door for the **Hapax Research Lab** portfolio ([hapaxresearch.com](https://hapaxresearch.com)). Its current public ceiling is observation and governed command preview, not autonomous write authority.

## Reader promise

Product front-door terminal cockpit for observing and previewing governed AI-agent delivery state (`reins --demo` runs offline on bundled fixture data).

## Reader value

Lets operators, auditors, and technical evaluators inspect live or fixture agent-delivery state and see what a governed write command would request before any write authority is granted.

## Claim ceiling

Read and command-preview today; write commands render preview envelopes that mint nothing until receipt-backed writes ship.

## License and rights

Source-available under Business Source License 1.1; not open source until the Change Date converts the work to Apache-2.0. Self-hosted and non-competing production use is permitted within the Additional Use Grant; competing hosted-service rights remain reserved.

Rendered summary: Business Source License 1.1 (source-available; not Open Source until the Change Date converts the work to Apache-2.0). See `LICENSE`, `NOTICE.md`, `CITATION.cff`, and `.zenodo.json` for the authority surfaces.

## Public boundary

- Issues are redirect-only; support, commercial engagement, and roadmap commitments do not happen through GitHub
- Published by **Hapax Research Lab** (`hrl-han`, [hapaxresearch.com](https://hapaxresearch.com)). Legal entity (as filed): Hapax Research LLC, organized in Wyoming on 2026-05-10, filing ID 2026-001973400.
- Security disclosures and factual correction proposals: `hrl-han@hapaxresearch.com` (see `SECURITY.md` and [hapaxresearch.com/about/#contact](https://hapaxresearch.com/about/#contact)). The network commits to receipt, retention and published correction. It promises no response time.
- Public copy uses `hapax-systems` organization links for first-party repositories; README text is orientation rather than a live runtime receipt.
- Governance reference: https://github.com/hapax-systems/hapax-constitution

## Portfolio position

Public product front-door cockpit consuming hapax-spine and read APIs. Current public claim ceiling is observation and governed command preview, not autonomous write authority.

<!-- hapax-public:surface=github.repo.reins.readme.preamble:end -->
<!-- hapax-sdlc:preamble:end -->

`reins` is a single-screen terminal cockpit and HTTP read/command-preview front
door for governed AI-agent delivery systems published by **Hapax Research Lab**.
It folds append-only lifecycle events, work items, agent sessions, capability
routes, and readiness checks into a fixed-cell terminal interface engineered for
16-color terminals, screen readers, and redacted video capture.

## Offline Demo and Installation

Download a prebuilt archive for Linux or macOS from
[GitHub Releases](https://github.com/hapax-systems/reins/releases), or run
directly from source with Go 1.26+:

```sh
reins --demo          # prebuilt binary from GitHub Releases
go run ./cmd/reins --demo
```

`--demo` launches the cockpit against deterministic embedded fixture data with
no external network calls, accounts, or configuration files.

## Shipped Scope and Boundaries

`reins` implements **observation and governed command preview**:

- **Read observation:** folds append-only event streams and capability registries into scored, navigable terminal views (`:events`, `:tasks`, `:sessions`, `:yard`, `:readiness`, `:capabilities`, `:lifecycles`, and read-only `GET /read/lanebus` metadata counts).
- **Command preview only:** entering a write-shaped command renders a governed preview envelope showing the structured intent that would be submitted; it mutates no state and mints no write receipts.
- **Declared lifecycle lenses:** software (`SDLC`), research (`RDLC`), and custom domain lifecycles are rendered today as navigation and inspection lenses rather than general-purpose automated workflow executors.

This repository also ships `cmd/han`, the **Human-Agentic Network (`HAN`)**
front command dispatcher (`han scope`, `han docket`, `han forum`, `han locus`);
see [`docs/han.md`](docs/han.md) for subcommand bindings and companion program
requirements.

## Cell Grammar and Monochrome Legibility

The character cell is the primary layout unit: semantic glyphs encode state
class (`▸` in-progress · `✓` succeeded · `✖` failed · `⇡` advance · `⚑` flagged
· `◆` task), eighth-block bars encode magnitude, and blank cells indicate no
active exception. Color acts only as secondary reinforcement so every state
remains distinguishable in monochrome terminals and screen-reader output.

## AIR — Default-Deny Redaction

Redaction is enforced at the renderer through two target modes over the same
view model:

- **`LOCAL`:** renders full field values for local inspection.
- **`AIR`:** renders every field not explicitly listed on the allowlist as `▒▒▒`, failing closed on newly added fields unless allowlisted. Toggle with `[a]` or `:air on|off`.

## Live Read API Setup

To connect `reins` to a live event substrate instead of `--demo` fixtures, run
the Python read service and point the Go cockpit at its HTTP endpoint:

```sh
# 1. Start the HTTP read service (reads the path in REINS_CONFIG, or ~/.config/reins/config.toml)
python api/reins_read.py

# 2. Launch the Go terminal cockpit
go run ./cmd/reins
```

Instance configuration (`config.example.toml` copied to
`~/.config/reins/config.toml` or referenced via the `REINS_CONFIG` environment
variable) supplies the substrate directory (`council_root` in the current schema),
read-API URL, `AIR` allowlist, and color palette. Upstream substrate event
producers are not bundled in this repository; `--demo` is the standalone
inspection path.

## Architecture

```
  SUBSTRATE            UNIFIED READ API           COCKPIT
  (event spine,     ─▶ (scored, AIR-classified ─▶ (Go / Bubble Tea,
   append-only)        HTTP projection)           stateless fold)
                       api/reins_read.py          cmd/reins + internal/*
```

- **Process and language boundary:** the Go cockpit communicates with `api/reins_read.py` strictly over HTTP and never imports Python substrate modules directly.
- **Stateless Elm fold:** every terminal frame is a pure function of fetched read-API payloads; re-fetching and re-folding restores view state deterministically.

## Headless Verification and Tests

Drive key sequences headlessly and run the Go, Python, and purview-engine test
suites locally:

```sh
make smoke                                     # visit every page headlessly
go run ./cmd/reins --drive ":tasks; j; enter"  # drive a key sequence and print the final frame
go test ./...                                  # Go unit and integration tests
( cd api && uv run --with pytest --with httpx pytest -q )
```

## Documentation

- [`docs/getting-started.md`](docs/getting-started.md) — offline demo walkthrough and navigation basics.
- [`docs/concepts.md`](docs/concepts.md) — glossary, page catalog, and keyboard reference.
- [`docs/han.md`](docs/han.md) — `Human-Agentic Network` (`HAN`) front dispatcher reference.

## License

Source-available under the **Business Source License 1.1** (see `LICENSE` and
`NOTICE.md`), permitting non-competing production and self-hosted use under the
Additional Use Grant and converting to `Apache-2.0` on the Change Date
(`2030-07-03`).

---

*HACA-C v0.3.15, the lab's draft rule for labelling machine-made text, not an external certification. No request to a natural person to approve the final wording, and no such approval, was found in hrl-han's sending, review and coordination records when the text was released. hrl-han releases a text once its checking is complete, under a standing rule that hrl-han's principal, who is not named in this text, adopted before the text was drafted. Review record: https://hapaxresearch.com/review-record/ (open to anyone). This label records how the text was made; it does not certify that the text is true. An unaltered record can contain false claims or miss relevant events.*
