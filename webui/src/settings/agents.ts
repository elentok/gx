import { apiFetch } from "../apiFetch";
import { postJSON } from "../auth/http";
import type { Refusable } from "../useWrite";

export type Harness = "claude" | "codex";

// Hand-written from server/defaultharness.go: generating that file makes tygo emit a second
// `Reason` type, which collides with the one already in generated/server.ts.
export interface DefaultHarnessStatus {
  default: string;
  runner: string;
  projects: { project: string; harness: string }[];
}

export interface SetDefaultHarnessRequest {
  scope: "global" | "project";
  project?: string;
  /** Empty with scope project removes the override. */
  harness: Harness | "";
}

export interface SetDefaultHarnessResult extends Refusable {
  changed: boolean;
  /** The server's sentence for the switch; empty when nothing changed. */
  notice?: string;
}

export type FetchAgentSettings = () => Promise<DefaultHarnessStatus>;
export type PostAgentSettings = (req: SetDefaultHarnessRequest) => Promise<SetDefaultHarnessResult>;

export const fetchAgentSettings: FetchAgentSettings = async () => {
  const res = await apiFetch("/v1/settings/agents");
  if (!res.ok) throw new Error(`settings/agents: HTTP ${res.status}`);
  return res.json();
};

export const postAgentSettings: PostAgentSettings = (req) => postJSON("/v1/settings/agents", req);

export const harnessName = (h: string): string => (h === "codex" ? "Codex" : "Claude");

export const otherHarness = (h: string): Harness => (h === "codex" ? "claude" : "codex");

/** The dim line under the title: where Codex runs when that differs from the usual PTY. */
export function runnerNote(runner: string): string | undefined {
  if (runner === "headless") return "Codex runs on PTY";
  if (runner === "herdr") return "Codex runs on herdr with a reduced flow";
  return undefined;
}

/** Codex is the global default or some project overrides to it. */
export const usesCodex = (s: DefaultHarnessStatus): boolean => s.default === "codex" || s.projects.some((p) => p.harness === "codex");
