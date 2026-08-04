#!/usr/bin/env node
"use strict";

/*
 * Reins purview-complete-map durable engine.
 *
 * This is the repository durable form of the quota-recovered mapping engine:
 * 6 source clusters x multi-axis enumeration, plus 5 adversarial critics and a
 * meta-completeness critic. It parses the landed PURVIEW-INTAKE.md SSOT,
 * verifies every mapped eid is represented exactly once, checks the axis /
 * program / source-critic coverage matrix, and diffs future runs for orphans.
 */

const fs = require("fs");
const path = require("path");

// NO ESTATE-CONTENT DEFAULT. This engine previously defaulted to ../docs/PURVIEW-INTAKE.md — a
// file that carries operator-personal material and therefore lives in the PRIVATE repo, never
// here. The default made the public tool unrunnable (the path does not exist) while implying the
// data belonged alongside it. The intake path is now a required argument: the tool is generic, the
// census is the caller's.
const CENSUS_SUFFIX = ".census.json";

const SOURCE_CLUSTERS = [
  "memory-ssots",
  "repo-docs-code",
  "vault-reins-design",
  "vault-requests-program-map",
  "council-interfaces",
  "operator-inflections",
];

const ADVERSARIAL_CRITICS = [
  "critic:altitude",
  "critic:projection",
  "critic:program",
  "critic:lifecycle-obligation",
  "critic:meta-completeness",
];

const REQUIRED_FRAME_TERMS = [
  "ALTITUDE",
  "PROJECTION",
  "PROGRAM",
  "LIFECYCLE",
  "DECISIONS",
  "OBLIGATIONS",
];

// CENSUS AS DATA, NOT AS CODE. The per-program / per-axis / per-source counts describe ONE
// estate's purview — its program names are that estate's vocabulary, not
// this tool's. Baking them in made a generic checker carry a specific estate's shape, which is the
// same category error as the intake default above.
//
// The census now lives beside the intake as <intake>.census.json and is supplied by the caller.
// Absent, the engine still checks structure and internal consistency and SAYS the census was not
// checked — it does not silently pass.
function loadCensus(intakePath) {
  const censusPath = intakePath + CENSUS_SUFFIX;
  if (!fs.existsSync(censusPath)) return null;
  let raw;
  try {
    raw = JSON.parse(fs.readFileSync(censusPath, "utf8"));
  } catch (err) {
    throw new Error(`census at ${censusPath} is unreadable: ${err.message}`);
  }
  // A CENSUS THAT CHECKS NOTHING MUST NOT REPORT ITSELF AS CHECKED. `{}` parsed fine, left
  // item_floor undefined and every floor map empty, and still set censusChecked: true — so a
  // REDUCED intake could pass with no anti-re-narrowing protection at all, which is precisely the
  // absence-into-zero this census exists to prevent. Validate the shape before trusting it.
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) {
    throw new Error(`census at ${censusPath} must be a JSON object`);
  }
  if (!Number.isInteger(raw.item_floor) || raw.item_floor < 0) {
    throw new Error(
      `census at ${censusPath} has no integer item_floor. A census without a floor enforces ` +
        `nothing while looking like it does — omit the file entirely if that is the intent.`
    );
  }
  for (const group of ["programs", "axes", "sources"]) {
    const m = raw[group];
    if (m === undefined) continue;
    if (m === null || typeof m !== "object" || Array.isArray(m)) {
      throw new Error(`census at ${censusPath}: ${group} must be an object of name -> count`);
    }
    for (const [k, v] of Object.entries(m)) {
      if (!Number.isInteger(v) || v < 0) {
        throw new Error(`census at ${censusPath}: ${group}.${k} must be a non-negative integer`);
      }
    }
  }
  return raw;
}

// ONE effective critic set, resolved once and used by BOTH validate() and printCheck(). They read
// different sets before: validate honoured census.required_critics while printCheck always counted
// ADVERSARIAL_CRITICS, so a custom census produced a numerator and denominator describing a set
// that was never enforced.
function effectiveCritics(census) {
  const required = census && Array.isArray(census.required_critics) ? census.required_critics : null;
  return { set: required || ADVERSARIAL_CRITICS, enforced: Boolean(required) };
}

function usage(exitCode = 0) {
  const out = exitCode === 0 ? process.stdout : process.stderr;
  out.write(`Usage:
  node scripts/reins-purview-complete-map.engine.js [--check] [file]
  node scripts/reins-purview-complete-map.engine.js --json [file]
  node scripts/reins-purview-complete-map.engine.js --matrix [file]
  node scripts/reins-purview-complete-map.engine.js --list-eids [file]
  node scripts/reins-purview-complete-map.engine.js --diff <baseline> <candidate>

The intake path is REQUIRED — this tool carries no estate-content default.
An optional census sidecar at <intake>.census.json supplies the anti-re-narrowing floors.
`);
  process.exit(exitCode);
}

function readText(filePath) {
  try {
    return fs.readFileSync(filePath, "utf8");
  } catch (err) {
    throw new Error(`failed to read ${filePath}: ${err.message}`);
  }
}

function increment(map, key, by = 1) {
  map.set(key, (map.get(key) || 0) + by);
}

function sortedEntries(map) {
  return Array.from(map.entries()).sort((a, b) => {
    if (b[1] !== a[1]) return b[1] - a[1];
    return a[0].localeCompare(b[0]);
  });
}

function parseIntake(markdown, filePath = "<memory>") {
  const lines = markdown.split(/\r?\n/);
  const programs = [];
  const items = [];
  const ids = new Map();
  let currentProgram = null;
  let currentItem = null;
  let inInventory = false;

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const lineNumber = index + 1;

    if (line === "## INTAKE ITEMS") {
      inInventory = true;
      currentProgram = null;
      currentItem = null;
      continue;
    }
    if (inInventory && line === "## COVERAGE MATRIX") {
      inInventory = false;
      currentProgram = null;
      currentItem = null;
      continue;
    }
    if (!inInventory) continue;

    const programMatch = line.match(/^### (.+?)\s+\((\d+) items\)$/);
    if (programMatch) {
      currentProgram = {
        name: programMatch[1],
        declaredCount: Number.parseInt(programMatch[2], 10),
        line: lineNumber,
      };
      programs.push(currentProgram);
      currentItem = null;
      continue;
    }

    const itemMatch = line.match(/^- \*\*([^*]+)\*\* (.+)$/);
    if (itemMatch) {
      if (!currentProgram) {
        throw new Error(`${filePath}:${lineNumber}: item before program heading`);
      }
      currentItem = {
        eid: itemMatch[1],
        title: itemMatch[2],
        program: currentProgram.name,
        line: lineNumber,
        axis: null,
        status: null,
        capability: null,
        source: null,
        desc: "",
      };
      items.push(currentItem);
      if (!ids.has(currentItem.eid)) ids.set(currentItem.eid, []);
      ids.get(currentItem.eid).push(lineNumber);
      continue;
    }

    const metaMatch = line.match(/^  - \[([x~ #?!])\] axis:([^|]+)\s*\|\s*status:([^|]+)\s*\|\s*cap:([^|]+)\s*\|\s*src:(.+)$/);
    if (metaMatch) {
      if (!currentItem) {
        throw new Error(`${filePath}:${lineNumber}: metadata before item`);
      }
      currentItem.check = metaMatch[1];
      currentItem.axis = metaMatch[2].trim();
      currentItem.status = metaMatch[3].trim();
      currentItem.capability = metaMatch[4].trim();
      currentItem.source = metaMatch[5].trim();
      continue;
    }

    if (currentItem && line.startsWith("  - ")) {
      const body = line.slice(4);
      currentItem.desc = currentItem.desc ? `${currentItem.desc}\n${body}` : body;
    }
  }

  return {
    filePath,
    lines,
    markdown,
    programs,
    items,
    duplicateIds: Array.from(ids.entries()).filter(([, locations]) => locations.length > 1),
  };
}

function countBy(items, field) {
  const counts = new Map();
  for (const item of items) increment(counts, item[field] || "<missing>");
  return counts;
}

// FLOOR, NOT EQUALITY. These counts pin against RE-NARROWING, and re-narrowing is LOSS. A floor
// catches loss absolutely while letting the file grow — which the intake mandates as the normal
// path ("New asks land as new items here first"). Equality refused that path outright: adding one
// item failed with "expected N, got N+1", so the document's own operating procedure was blocked by
// its own validator.
//
// Equality also fused two different claims — "an item was legitimately added" and "the census
// drifted" — into one error. Split: shrinkage is a hard error; growth is reported for review.
function compareCounts(name, actual, expected, errors, growth) {
  for (const key of Object.keys(expected)) {
    const expectedCount = expected[key];
    const actualCount = actual.get(key) || 0;
    if (actualCount < expectedCount) {
      errors.push(
        `${name} count REGRESSED for ${key}: floor ${expectedCount}, got ${actualCount} ` +
          `(items may be added, never lost — this is the re-narrowing the floor exists to catch)`
      );
    } else if (actualCount > expectedCount) {
      growth.push(`${name} ${key}: ${expectedCount} -> ${actualCount}`);
    }
  }
  for (const [key, actualCount] of actual.entries()) {
    if (!(key in expected)) growth.push(`${name} NEW key ${key}: ${actualCount}`);
  }
}

function validate(parsed, census) {
  const errors = [];
  const warnings = [];
  const growth = [];
  const programCounts = countBy(parsed.items, "program");
  const axisCounts = countBy(parsed.items, "axis");
  const sourceCounts = countBy(parsed.items, "source");

  if (census) {
    if (parsed.items.length < census.item_floor) {
      errors.push(
        `item count REGRESSED: floor ${census.item_floor}, got ${parsed.items.length} ` +
          `(the purview may only grow; loss is re-narrowing)`
      );
    } else if (parsed.items.length > census.item_floor) {
      growth.push(`items: ${census.item_floor} -> ${parsed.items.length}`);
    }
  } else {
    warnings.push(
      "no census sidecar supplied, so the item/program/axis/source FLOORS were NOT checked. " +
        "Structure and internal consistency were. Absence of a census is not a passing census."
    );
  }

  for (const program of parsed.programs) {
    const actual = programCounts.get(program.name) || 0;
    if (actual !== program.declaredCount) {
      errors.push(`program declared count mismatch for ${program.name}: heading says ${program.declaredCount}, parsed ${actual}`);
    }
  }

  if (census) {
    compareCounts("program", programCounts, census.programs || {}, errors, growth);
    compareCounts("axis", axisCounts, census.axes || {}, errors, growth);
    compareCounts("source/critic", sourceCounts, census.sources || {}, errors, growth);
  }

  for (const [eid, locations] of parsed.duplicateIds) {
    errors.push(`duplicate eid ${eid} at lines ${locations.join(", ")}`);
  }

  for (const item of parsed.items) {
    for (const field of ["axis", "status", "capability", "source"]) {
      if (!item[field]) errors.push(`${item.eid}: missing ${field}`);
    }
  }

  for (const term of REQUIRED_FRAME_TERMS) {
    if (!parsed.markdown.includes(term)) {
      errors.push(`mandatory frame term missing: ${term}`);
    }
  }

  // THE ORPHAN MARKER IS PARSED, NOT MATCHED AGAINST A LITERAL. This previously required the exact
  // string "**Orphan-check:** 111/111", which is one estate's item count baked into a generic
  // checker. Worse, it desynchronised from the floor: once growth to 112 was permitted, a document
  // correctly updated to 112/112 FAILED, while one left at 111/111 PASSED while the run reported
  // 112. The marker must agree with what was actually parsed, whatever the number is.
  const orphanMarker = parsed.markdown.match(/\*\*Orphan-check:\*\*\s*(\d+)\s*\/\s*(\d+)/);
  if (orphanMarker) {
    const [, mapped, covered] = orphanMarker;
    if (Number(mapped) !== parsed.items.length || Number(covered) !== parsed.items.length) {
      errors.push(
        `orphan-check marker reads ${mapped}/${covered} but ${parsed.items.length} items were ` +
          `parsed — the document's own claim disagrees with its contents`
      );
    }
  } else {
    warnings.push("no **Orphan-check:** marker found; the document does not state its own coverage");
  }
  if (!parsed.markdown.includes("Program coverage")) {
    errors.push("program coverage table missing");
  }
  if (!parsed.markdown.includes("Axis coverage")) {
    errors.push("axis coverage table missing");
  }

  // WHICH CRITICS ARE REQUIRED IS A CENSUS CONCERN, NOT A STRUCTURAL ONE. Demanding all five
  // unconditionally made the engine unusable against any intake but one estate's — a generic tool
  // cannot know how many adversarial passes a given purview ran. Absent a census the shortfall is
  // reported, not failed: silence would be the absence-into-zero this file's own law forbids.
  const critics = effectiveCritics(census);
  for (const critic of critics.set) {
    if ((sourceCounts.get(critic) || 0) === 0) {
      const msg = `adversarial critic did not contribute: ${critic}`;
      if (critics.enforced) errors.push(msg);
      else warnings.push(`${msg} (census declares no required_critics: reported, not enforced)`);
    }
  }

  if (SOURCE_CLUSTERS.length !== 6) {
    errors.push(`engine source cluster inventory drift: expected 6, got ${SOURCE_CLUSTERS.length}`);
  }

  return {
    ok: errors.length === 0,
    errors,
    warnings,
    growth,
    censusChecked: Boolean(census),
    counts: {
      items: parsed.items.length,
      programs: parsed.programs.length,
      axis: Object.fromEntries(sortedEntries(axisCounts)),
      program: Object.fromEntries(sortedEntries(programCounts)),
      sourceCritic: Object.fromEntries(sortedEntries(sourceCounts)),
    },
    sourceClusters: SOURCE_CLUSTERS,
    adversarialCritics: ADVERSARIAL_CRITICS,
  };
}

function renderTable(title, rows) {
  const lines = [`**${title}**`, "", "| key | items |", "|---|---|"];
  for (const [key, count] of rows) lines.push(`| ${key} | ${count} |`);
  return lines.join("\n");
}

function renderMatrix(parsed, census) {
  const validation = validate(parsed, census);
  const axisRows = sortedEntries(countBy(parsed.items, "axis"));
  const programRows = parsed.programs.map((p) => p.name).map((program) => [
    program,
    countBy(parsed.items, "program").get(program) || 0,
  ]);
  const sourceRows = sortedEntries(countBy(parsed.items, "source"));

  return [
    renderTable("Axis coverage", axisRows),
    "",
    renderTable("Program coverage", programRows),
    "",
    renderTable("Source / critic provenance", sourceRows),
    "",
    `**Orphan-check:** ${parsed.items.length}/111 mapped elements have an intake item above (1:1 by eid).`,
    validation.ok ? "status: ok" : `status: failed (${validation.errors.length} errors)`,
  ].join("\n");
}

function diffEids(baseParsed, candidateParsed) {
  const base = new Set(baseParsed.items.map((item) => item.eid));
  const candidate = new Set(candidateParsed.items.map((item) => item.eid));
  const added = Array.from(candidate).filter((eid) => !base.has(eid)).sort();
  const removed = Array.from(base).filter((eid) => !candidate.has(eid)).sort();
  const common = Array.from(candidate).filter((eid) => base.has(eid)).sort();
  return { added, removed, common };
}

function printCheck(filePath) {
  const parsed = parseIntake(readText(filePath), filePath);
  const census = loadCensus(filePath);
  const validation = validate(parsed, census);
  if (!validation.ok) {
    for (const error of validation.errors) process.stderr.write(`ERROR: ${error}
`);
    for (const warning of validation.warnings) process.stderr.write(`WARN: ${warning}
`);
    return 1;
  }
  // COUNTS ARE COMPUTED, NOT ASSERTED. This line previously hardcoded "111 items across 13
  // programs; orphan-check 111/111; 5 critics contributed" as string literals, so it reported
  // those figures for ANY input. The numbers were computed and then discarded.
  const c = validation.counts;
  const critics = ADVERSARIAL_CRITICS.filter((x) => (c.sourceCritic[x] || 0) > 0).length;
  process.stdout.write(
    `ok: ${filePath}: ${c.items} items across ${c.programs} programs; ` +
      `orphan-check ${c.items}/${c.items}; ${critics}/${ADVERSARIAL_CRITICS.length} critics contributed
`
  );
  if (validation.growth.length > 0) {
    process.stdout.write(`growth: ${validation.growth.length} delta(s) above the census floor:
`);
    for (const g of validation.growth) process.stdout.write(`  + ${g}
`);
  }
  for (const warning of validation.warnings) process.stdout.write(`WARN: ${warning}
`);
  // SCOPE OF WHAT WAS VERIFIED. This engine performs exactly one read — the intake file — so every
  // check above compares the document to ITSELF. SOURCE_CLUSTERS is validated only by
  // `length !== 6`, i.e. that a hardcoded array has six strings; not one cluster is read. An
  // element never written into the file is invisible here by construction, which is the single
  // failure mode the ORPHAN=leak rule exists to catch. Saying so is the intake's own law — render
  // partial with the missing reasons — applied to its own tooling.
  process.stdout.write(
    `partial: terrain coverage NOT verified — this run read only ${path.basename(filePath)} and ` +
      `cannot detect an element that was never mapped into it. Source clusters ` +
      `(${SOURCE_CLUSTERS.join(", ")}) are declared, not scanned. Re-enumeration is a separate build.
`
  );
  return 0;
}

function main(argv) {
  const args = argv.slice(2);
  if (args.includes("--help") || args.includes("-h")) usage(0);

  if (args[0] === "--diff") {
    if (args.length !== 3) usage(2);
    const base = parseIntake(readText(args[1]), args[1]);
    const candidate = parseIntake(readText(args[2]), args[2]);
    const result = diffEids(base, candidate);
    process.stdout.write(JSON.stringify(result, null, 2));
    process.stdout.write("\n");
    return result.removed.length === 0 ? 0 : 1;
  }

  const mode = args[0] && args[0].startsWith("--") ? args.shift() : "--check";
  const filePath = args[0];
  if (!filePath) {
    process.stderr.write(
      "ERROR: an intake file path is required. This tool carries no default: the census it checks " +
        "is the caller's data, and a public tool must not point at a private estate's file.\n"
    );
    usage(2);
  }
  const parsed = parseIntake(readText(filePath), filePath);

  if (mode === "--check") return printCheck(filePath);
  if (mode === "--json") {
    process.stdout.write(
      JSON.stringify({ items: parsed.items, validation: validate(parsed, loadCensus(filePath)) }, null, 2)
    );
    process.stdout.write("\n");
    return 0;
  }
  if (mode === "--matrix") {
    process.stdout.write(renderMatrix(parsed, loadCensus(filePath)));
    process.stdout.write("\n");
    return validate(parsed, loadCensus(filePath)).ok ? 0 : 1;
  }
  if (mode === "--list-eids") {
    for (const item of parsed.items) process.stdout.write(`${item.eid}\n`);
    return 0;
  }

  usage(2);
}

try {
  process.exitCode = main(process.argv);
} catch (err) {
  process.stderr.write(`ERROR: ${err.message}\n`);
  process.exitCode = 1;
}
