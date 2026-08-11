#!/usr/bin/env node
// Aggregates .scratch/<epic>/run-log.jsonl files into two metrics:
//   1. unattended completion rate per ticket (no needs-attention/needs-info/
//      conflict-hit before the ticket's last event)
//   2. failure/retry taxonomy: counts of intervention-signaling event types
//
// Usage: node scripts/ralph-stats.mjs [scratchDir...] [--since=YYYY-MM-DD] [--until=YYYY-MM-DD] [--daily]
// Accepts any number of scratch dirs (one per project) and aggregates across
// all of them. Defaults to .bare/.scratch relative to the gx bare repo.
// --since/--until filter events by local calendar day (inclusive).
// --daily breaks the two metrics down per calendar day; with no --since it
// defaults to the past 7 days (today and the 6 days before it).

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const rawArgs = process.argv.slice(2);
const flags = { since: null, until: null, daily: false };
const scratchDirArgs = [];
for (const arg of rawArgs) {
  if (arg.startsWith("--since=")) flags.since = arg.slice("--since=".length);
  else if (arg.startsWith("--until=")) flags.until = arg.slice("--until=".length);
  else if (arg === "--daily") flags.daily = true;
  else scratchDirArgs.push(arg);
}

const scratchDirs = scratchDirArgs.length > 0
  ? scratchDirArgs
  : [join(process.env.HOME, "dev/gx/.bare/.scratch")];

// local YYYY-MM-DD, matching how a person reads "today"/"this week" on this machine
function dayKey(isoTime) {
  const d = new Date(isoTime);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

const todayKey = dayKey(new Date().toISOString());
let sinceKey = flags.since;
let untilKey = flags.until ?? todayKey;
if (!sinceKey && flags.daily) {
  const d = new Date();
  d.setDate(d.getDate() - 6);
  sinceKey = dayKey(d);
}
const dateFilterActive = Boolean(sinceKey || flags.until);

function inRange(iso) {
  if (!dateFilterActive) return true;
  const key = dayKey(iso);
  if (sinceKey && key < sinceKey) return false;
  if (untilKey && key > untilKey) return false;
  return true;
}

const INTERVENTION_TYPES = new Set([
  "needs-attention",
  "needs-info",
  "conflict-hit",
  "paused-smart-zone",
  "smart-zone-recovery-failed",
  "paused-rate-limit",
  "commitless",
  "notification-failed",
]);

// epic dirs live directly under scratchDir, except finished ones which get
// moved one level deeper under .archive/ — walk both.
function findRunLogs(scratchDir) {
  const out = [];
  for (const entry of readdirSync(scratchDir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    if (entry.name === ".archive") {
      out.push(...findRunLogs(join(scratchDir, entry.name)));
      continue;
    }
    const runLog = join(scratchDir, entry.name, "run-log.jsonl");
    try {
      statSync(runLog);
      out.push({ epic: entry.name, path: runLog });
    } catch {
      // no run-log for this epic dir, skip
    }
  }
  return out;
}

function parseEvents(path) {
  const text = readFileSync(path, "utf8");
  const events = [];
  for (const line of text.split("\n")) {
    if (!line.trim()) continue;
    try {
      events.push(JSON.parse(line));
    } catch {
      // skip malformed line
    }
  }
  return events;
}

// project label = scratch dir's grandparent dir name (e.g. "gx" from
// ~/dev/gx/.bare/.scratch), falls back to the raw path if that's ambiguous.
function projectLabel(scratchDir) {
  const parts = scratchDir.split("/").filter(Boolean);
  const i = parts.lastIndexOf(".scratch");
  return i > 0 ? parts[i - 1] === ".bare" ? parts[i - 2] ?? scratchDir : parts[i - 1] : scratchDir;
}

const logsByProject = scratchDirs.map((dir) => ({
  project: projectLabel(dir),
  dir,
  logs: findRunLogs(dir),
}));

const totalLogs = logsByProject.reduce((n, p) => n + p.logs.length, 0);
if (totalLogs === 0) {
  console.error(`no run-log.jsonl files found under: ${scratchDirs.join(", ")}`);
  process.exit(1);
}

// ticket key = "project/epic/ticket"
const perTicket = new Map(); // key -> { interventions: Set<type>, finished: bool }
const typeCounts = new Map(); // event type -> count
const perEpicTypeCounts = new Map(); // "project/epic" -> Map(type -> count)
const perDayTicket = new Map(); // day -> Map(ticketKey -> { interventions: Set, finished: bool })
const perDayTypeCounts = new Map(); // day -> Map(type -> count)

for (const { project, logs } of logsByProject) {
  for (const { epic, path } of logs) {
    const events = parseEvents(path).filter((ev) => ev.time && inRange(ev.time));
    const epicKey = `${project}/${epic}`;
    const epicCounts = perEpicTypeCounts.get(epicKey) ?? new Map();
    perEpicTypeCounts.set(epicKey, epicCounts);

    for (const ev of events) {
      typeCounts.set(ev.type, (typeCounts.get(ev.type) ?? 0) + 1);
      epicCounts.set(ev.type, (epicCounts.get(ev.type) ?? 0) + 1);

      const day = dayKey(ev.time);
      const dayCounts = perDayTypeCounts.get(day) ?? new Map();
      dayCounts.set(ev.type, (dayCounts.get(ev.type) ?? 0) + 1);
      perDayTypeCounts.set(day, dayCounts);

      if (!ev.ticket) continue; // scheduler-scan and epic-level events have no ticket
      const key = `${epicKey}/${ev.ticket}`;

      const t = perTicket.get(key) ?? { interventions: new Set(), finished: false };
      if (INTERVENTION_TYPES.has(ev.type)) t.interventions.add(ev.type);
      if (ev.type === "cherry-picked" || ev.type === "iteration-finished") t.finished = true;
      perTicket.set(key, t);

      const dayTickets = perDayTicket.get(day) ?? new Map();
      const dt = dayTickets.get(key) ?? { interventions: new Set(), finished: false };
      if (INTERVENTION_TYPES.has(ev.type)) dt.interventions.add(ev.type);
      if (ev.type === "cherry-picked" || ev.type === "iteration-finished") dt.finished = true;
      dayTickets.set(key, dt);
      perDayTicket.set(day, dayTickets);
    }
  }
}

console.log(`=== Scanned ${totalLogs} epic(s) across ${logsByProject.length} project(s) ===`);
for (const { project, dir, logs } of logsByProject) {
  console.log(`  ${project}: ${logs.length} epic(s) under ${dir}`);
}
if (dateFilterActive) {
  console.log(`  date range: ${sinceKey ?? "(start)"} .. ${untilKey}`);
}
console.log();

// Metric 1: unattended completion rate
const finishedTickets = [...perTicket.entries()].filter(([, t]) => t.finished);
const unattended = finishedTickets.filter(([, t]) => t.interventions.size === 0);
console.log("--- Metric 1: unattended completion rate ---");
console.log(`${unattended.length}/${finishedTickets.length} finished tickets needed no intervention` +
  (finishedTickets.length ? ` (${((unattended.length / finishedTickets.length) * 100).toFixed(0)}%)` : ""));
for (const [key, t] of finishedTickets) {
  if (t.interventions.size > 0) {
    console.log(`  ! ${key}: ${[...t.interventions].join(", ")}`);
  }
}

// Metric 3: failure/retry taxonomy
console.log("\n--- Metric 3: failure/retry taxonomy (all events) ---");
const interventionEntries = [...typeCounts.entries()]
  .filter(([type]) => INTERVENTION_TYPES.has(type))
  .sort((a, b) => b[1] - a[1]);
if (interventionEntries.length === 0) {
  console.log("  (none — clean run so far)");
} else {
  const max = Math.max(...interventionEntries.map(([, n]) => n));
  for (const [type, n] of interventionEntries) {
    const bar = "#".repeat(Math.max(1, Math.round((n / max) * 30)));
    console.log(`  ${type.padEnd(28)} ${String(n).padStart(3)} ${bar}`);
  }
}

console.log("\n--- All event type counts ---");
for (const [type, n] of [...typeCounts.entries()].sort((a, b) => b[1] - a[1])) {
  console.log(`  ${type.padEnd(28)} ${n}`);
}

if (flags.daily) {
  console.log(`\n--- Per-day breakdown (${sinceKey} .. ${untilKey}) ---`);
  const days = [];
  for (let d = sinceKey; d <= untilKey; ) {
    days.push(d);
    const [y, m, dd] = d.split("-").map(Number);
    d = dayKey(new Date(y, m - 1, dd + 1));
  }
  console.log(
    `  ${"date".padEnd(12)} ${"finished".padStart(8)} ${"unattended".padStart(10)} ${"rate".padStart(6)}` +
    ` ${"compacted".padStart(10)} ${"comp-rate".padStart(9)}  interventions`
  );
  for (const day of days) {
    const dayTickets = [...(perDayTicket.get(day) ?? new Map()).values()];
    const dayFinished = dayTickets.filter((t) => t.finished);
    const dayUnattended = dayFinished.filter((t) => t.interventions.size === 0);
    const rate = dayFinished.length ? `${((dayUnattended.length / dayFinished.length) * 100).toFixed(0)}%` : "-";
    const dayCompacted = dayFinished.filter((t) => t.interventions.has("paused-smart-zone"));
    const compactRate = dayFinished.length ? `${((dayCompacted.length / dayFinished.length) * 100).toFixed(0)}%` : "-";
    const dayTypeCounts = perDayTypeCounts.get(day) ?? new Map();
    const interventions = [...dayTypeCounts.entries()]
      .filter(([type]) => INTERVENTION_TYPES.has(type))
      .map(([type, n]) => `${type}${n > 1 ? `x${n}` : ""}`)
      .join(", ");
    console.log(
      `  ${day.padEnd(12)} ${String(dayFinished.length).padStart(8)} ${String(dayUnattended.length).padStart(10)} ${rate.padStart(6)}` +
      ` ${String(dayCompacted.length).padStart(10)} ${compactRate.padStart(9)}  ${interventions}`
    );
  }
}
