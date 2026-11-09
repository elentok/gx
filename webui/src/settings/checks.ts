import { removeChange, setChange } from "./settingsApi";
import type { SettingChange } from "../generated/server";

// Mirrors the shapes check.ParseConfig accepts (docs/checks-schema.json).
export const CHECK_KINDS = ["format", "lint", "typecheck", "test"] as const;
export type CheckKind = (typeof CHECK_KINDS)[number];

export const SLOW_TOOLS = ["go", "vitest", "playwright"] as const;
export type SlowTool = (typeof SLOW_TOOLS)[number];

export const CHECKS_KEY = "checks";

export type CheckMode = "default" | "off" | "one" | "pair";

export interface CheckForm {
  mode: CheckMode;
  /** The single command of the "one" mode. */
  command: string;
  /** Pair mode: command for changed files; empty means the gx default. */
  changed: string;
  /** Pair mode: command for a --full run (required). */
  full: string;
  /** Pair mode only: commands that also run in every non-full run. */
  always: string[];
  /** Test kind limits; an empty field means no limit. */
  suite: string;
  hang: string;
  slow: string;
  /** The file held a per-tool slow map; saving keeps that form. */
  slowIsMap: boolean;
  slowMap: Record<SlowTool, string>;
}

export type ChecksModel = Record<CheckKind, CheckForm>;

const emptyForm = (): CheckForm => ({
  mode: "default",
  command: "",
  changed: "",
  full: "",
  always: [],
  suite: "",
  hang: "",
  slow: "",
  slowIsMap: false,
  slowMap: { go: "", vitest: "", playwright: "" },
});

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const str = (v: unknown): string => (typeof v === "string" ? v : "");

function parseLimits(form: CheckForm, raw: unknown): void {
  if (!isRecord(raw)) return;
  form.suite = str(raw.suite);
  form.hang = str(raw.hang);
  if (isRecord(raw.slow)) {
    form.slowIsMap = true;
    for (const tool of SLOW_TOOLS) form.slowMap[tool] = str(raw.slow[tool]);
  } else {
    form.slow = str(raw.slow);
  }
}

function parseForm(raw: unknown): CheckForm {
  const form = emptyForm();
  if (raw === false) {
    form.mode = "off";
  } else if (typeof raw === "string") {
    form.mode = "one";
    form.command = raw;
  } else if (isRecord(raw)) {
    form.changed = str(raw.changed);
    form.full = str(raw.full);
    if (Array.isArray(raw.always)) form.always = raw.always.filter((a): a is string => typeof a === "string");
    // formValue writes "one command" with limits as equal changed/full; read that back as "one".
    const isOne = form.full !== "" && form.changed === form.full && form.always.length === 0;
    form.mode = isOne ? "one" : "pair";
    if (isOne) form.command = form.full;
    parseLimits(form, raw.limits);
  }
  return form;
}

/** Form state for a project's `checks` value (anything unrecognised reads as default). */
export function checksToModel(value: unknown): ChecksModel {
  const src = isRecord(value) ? value : {};
  return Object.fromEntries(CHECK_KINDS.map((k) => [k, parseForm(src[k])])) as ChecksModel;
}

function limitsValue(form: CheckForm): Record<string, unknown> | undefined {
  const limits: Record<string, unknown> = {};
  if (form.suite) limits.suite = form.suite;
  if (form.hang) limits.hang = form.hang;
  if (form.slowIsMap) {
    const map: Record<string, string> = {};
    for (const tool of SLOW_TOOLS) if (form.slowMap[tool]) map[tool] = form.slowMap[tool];
    if (Object.keys(map).length > 0) limits.slow = map;
  } else if (form.slow) {
    limits.slow = form.slow;
  }
  return Object.keys(limits).length > 0 ? limits : undefined;
}

const hasLimits = (kind: CheckKind, form: CheckForm): boolean => kind === "test" && limitsValue(form) !== undefined;

function formValue(kind: CheckKind, form: CheckForm): unknown {
  const limits = kind === "test" ? limitsValue(form) : undefined;
  switch (form.mode) {
    case "off":
      return false;
    case "one":
      // Limits only live on the object form, which needs both commands.
      return limits ? { changed: form.command, full: form.command, limits } : form.command;
    case "pair": {
      const out: Record<string, unknown> = {};
      if (form.changed) out.changed = form.changed;
      out.full = form.full;
      if (form.always.length > 0) out.always = form.always;
      if (limits) out.limits = limits;
      return out;
    }
    default:
      return undefined;
  }
}

/** Why a kind cannot be saved as it stands, or undefined when it can. */
export function checkError(kind: CheckKind, form: CheckForm): string | undefined {
  switch (form.mode) {
    case "one":
      return form.command.trim() ? undefined : "Enter a command.";
    case "pair":
      if (!form.full.trim()) return form.changed.trim() ? "A changed command needs a full command too." : "Enter a full command.";
      if (form.always.some((a) => !a.trim())) return "An always command must not be empty.";
      return undefined;
    case "default":
      return hasLimits(kind, form) ? "Limits need a command; pick one command or changed + full." : undefined;
    default:
      return undefined;
  }
}

export function checksErrors(model: ChecksModel): Partial<Record<CheckKind, string>> {
  const out: Partial<Record<CheckKind, string>> = {};
  for (const kind of CHECK_KINDS) {
    const err = checkError(kind, model[kind]);
    if (err) out[kind] = err;
  }
  return out;
}

/** The minimal `checks` value: absent kinds stay absent; undefined when nothing is set. */
export function modelToChecks(model: ChecksModel): Record<string, unknown> | undefined {
  const out: Record<string, unknown> = {};
  for (const kind of CHECK_KINDS) {
    const v = formValue(kind, model[kind]);
    if (v !== undefined) out[kind] = v;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** The settings change that saves the model; an all-default editor removes the key. */
export function modelToChange(model: ChecksModel): SettingChange {
  const value = modelToChecks(model);
  return value === undefined ? removeChange(CHECKS_KEY) : setChange(CHECKS_KEY, value);
}
