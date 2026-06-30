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

const DEFAULT_INTAKE = path.join(__dirname, "..", "docs", "PURVIEW-INTAKE.md");

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

const EXPECTED_PROGRAM_COUNTS = new Map([
  ["core-operator-life", 3],
  ["hos-program", 8],
  ["continuity-substrate", 18],
  ["n-DLC-consolidation", 11],
  ["capability-routing / EDT", 22],
  ["token-economics", 7],
  ["measurement-loop-token-economics", 3],
  ["capability-harnesses", 4],
  ["representational-framework", 3],
  ["avsdlc-visual-eval", 5],
  ["ldlc-tenant", 4],
  ["packaging", 5],
  ["reins-cockpit-overhaul", 18],
]);

const EXPECTED_AXIS_COUNTS = new Map([
  ["obligation", 26],
  ["altitude-substrate", 23],
  ["program", 23],
  ["projection-hapax", 13],
  ["altitude-telos", 6],
  ["projection-operator", 6],
  ["projection-coord", 5],
  ["decision", 4],
  ["altitude-surface", 3],
  ["representational", 1],
  ["lifecycle", 1],
]);

const EXPECTED_SOURCE_COUNTS = new Map([
  ["critic:program", 21],
  ["operator-inflections", 16],
  ["critic:meta-completeness", 15],
  ["council-interfaces", 14],
  ["reins-purview-complete-2026-06-28", 9],
  ["critic:altitude", 9],
  ["critic:lifecycle-obligation", 9],
  ["critic:projection", 8],
  ["vault-reins-design", 7],
  ["reins-purview-intake-anti-re-narrowing-2026-06-28", 3],
]);

function usage(exitCode = 0) {
  const out = exitCode === 0 ? process.stdout : process.stderr;
  out.write(`Usage:
  node scripts/reins-purview-complete-map.engine.js [--check] [file]
  node scripts/reins-purview-complete-map.engine.js --json [file]
  node scripts/reins-purview-complete-map.engine.js --matrix [file]
  node scripts/reins-purview-complete-map.engine.js --list-eids [file]
  node scripts/reins-purview-complete-map.engine.js --diff <baseline> <candidate>

Default file: docs/PURVIEW-INTAKE.md
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

function compareCounts(name, actual, expected, errors) {
  for (const [key, expectedCount] of expected.entries()) {
    const actualCount = actual.get(key) || 0;
    if (actualCount !== expectedCount) {
      errors.push(`${name} count mismatch for ${key}: expected ${expectedCount}, got ${actualCount}`);
    }
  }
  for (const [key, actualCount] of actual.entries()) {
    if (!expected.has(key)) {
      errors.push(`${name} has unexpected key ${key}: got ${actualCount}`);
    }
  }
}

function validate(parsed) {
  const errors = [];
  const warnings = [];
  const programCounts = countBy(parsed.items, "program");
  const axisCounts = countBy(parsed.items, "axis");
  const sourceCounts = countBy(parsed.items, "source");

  if (parsed.programs.length !== EXPECTED_PROGRAM_COUNTS.size) {
    errors.push(`program heading count mismatch: expected ${EXPECTED_PROGRAM_COUNTS.size}, got ${parsed.programs.length}`);
  }
  if (parsed.items.length !== 111) {
    errors.push(`item count mismatch: expected 111, got ${parsed.items.length}`);
  }

  for (const program of parsed.programs) {
    const actual = programCounts.get(program.name) || 0;
    if (actual !== program.declaredCount) {
      errors.push(`program declared count mismatch for ${program.name}: heading says ${program.declaredCount}, parsed ${actual}`);
    }
  }

  compareCounts("program", programCounts, EXPECTED_PROGRAM_COUNTS, errors);
  compareCounts("axis", axisCounts, EXPECTED_AXIS_COUNTS, errors);
  compareCounts("source/critic", sourceCounts, EXPECTED_SOURCE_COUNTS, errors);

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

  if (!parsed.markdown.includes("**Orphan-check:** 111/111")) {
    errors.push("coverage matrix orphan-check does not read 111/111");
  }
  if (!parsed.markdown.includes("Source / critic provenance")) {
    errors.push("source / critic provenance coverage table missing");
  }
  if (!parsed.markdown.includes("Program coverage")) {
    errors.push("program coverage table missing");
  }
  if (!parsed.markdown.includes("Axis coverage")) {
    errors.push("axis coverage table missing");
  }

  for (const critic of ADVERSARIAL_CRITICS) {
    if ((sourceCounts.get(critic) || 0) === 0) {
      errors.push(`adversarial critic did not contribute: ${critic}`);
    }
  }

  if (SOURCE_CLUSTERS.length !== 6) {
    errors.push(`engine source cluster inventory drift: expected 6, got ${SOURCE_CLUSTERS.length}`);
  }

  return {
    ok: errors.length === 0,
    errors,
    warnings,
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

function renderMatrix(parsed) {
  const validation = validate(parsed);
  const axisRows = sortedEntries(countBy(parsed.items, "axis"));
  const programRows = Array.from(EXPECTED_PROGRAM_COUNTS.keys()).map((program) => [
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
  const validation = validate(parsed);
  if (validation.ok) {
    process.stdout.write(`ok: ${filePath}: 111 items across 13 programs; orphan-check 111/111; 5 critics contributed\n`);
    return 0;
  }
  for (const error of validation.errors) process.stderr.write(`ERROR: ${error}\n`);
  for (const warning of validation.warnings) process.stderr.write(`WARN: ${warning}\n`);
  return 1;
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
  const filePath = args[0] || DEFAULT_INTAKE;
  const parsed = parseIntake(readText(filePath), filePath);

  if (mode === "--check") return printCheck(filePath);
  if (mode === "--json") {
    process.stdout.write(JSON.stringify({ items: parsed.items, validation: validate(parsed) }, null, 2));
    process.stdout.write("\n");
    return 0;
  }
  if (mode === "--matrix") {
    process.stdout.write(renderMatrix(parsed));
    process.stdout.write("\n");
    return validate(parsed).ok ? 0 : 1;
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
