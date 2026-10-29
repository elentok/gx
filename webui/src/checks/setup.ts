import { apiFetch } from "../apiFetch";
import { readWrite } from "../design/open";
import type { Refusable } from "../useWrite";
import type { ProjectChecks } from "./status";

/** The server's save targets for a check config. */
export type SaveTarget = "repo" | "project.json";

export interface SaveChecksBody {
  project: string;
  target: SaveTarget;
  checks: Record<string, false | { full: string }>;
}

export interface SaveChecksResult extends Refusable {
  trunk?: string;
}

export type SaveChecks = (body: SaveChecksBody) => Promise<SaveChecksResult>;

export const postSaveChecks: SaveChecks = async (body) => {
  const res = await apiFetch("/v1/checks/save", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return readWrite(res);
};

/** The server types a candidate loosely, so this is the shape `check.Candidate` marshals to. */
export interface Candidate {
  command: string;
  source: string;
}

export type Mode = "use" | "default" | "off";

export interface Row {
  kind: string;
  mode: Mode;
  command: string;
}

/** A found command starts as "Use command"; a kind with nothing found starts on the gx default. */
export function presetRows(p: ProjectChecks): Row[] {
  return p.kinds.map((k) => {
    const found = k.recommended as Candidate | undefined;
    return { kind: k.kind, mode: found ? "use" : "default", command: found?.command ?? "" };
  });
}

/** Use command becomes `{full}`, Off becomes `false`, and the gx default stays out of the file. */
export function configOf(rows: Row[]): SaveChecksBody["checks"] {
  const checks: SaveChecksBody["checks"] = {};
  for (const r of rows) {
    if (r.mode === "use") checks[r.kind] = { full: r.command.trim() };
    else if (r.mode === "off") checks[r.kind] = false;
  }
  return checks;
}

export const targetLabel = (target: SaveTarget, trunk: string) =>
  target === "repo" ? `.gx/checks.json (committed on ${trunk})` : "gx's project.json (this machine only)";

export const previewOf = (checks: SaveChecksBody["checks"], target: SaveTarget, trunk: string) =>
  `// ${targetLabel(target, trunk)}\n${JSON.stringify(checks, null, 2)}`;

export const savedMessage = (target: SaveTarget, trunk: string) =>
  target === "repo" ? `Committed .gx/checks.json on ${trunk}` : "Saved checks to project.json";
