import { apiFetch } from "../apiFetch";
import { postJSON } from "../auth/http";
import type { Connection } from "../connection";
import type { SettingChange, SettingsEntry, SettingsReadResponse, SettingsWriteRequest, SettingsWriteResult } from "../generated/server";
import { useFetched, type FetchedState } from "../useFetched";

/** The target name the server uses for the global config file. */
export const GLOBAL_TARGET = "global";

export type FetchSettings = (target: string) => Promise<SettingsReadResponse>;
export type PostSettings = (target: string, changes: SettingChange[]) => Promise<SettingsWriteResult>;

export const fetchSettings: FetchSettings = async (target) => {
  const res = await apiFetch(`/v1/settings?target=${encodeURIComponent(target)}`);
  if (!res.ok) throw new Error(`settings: HTTP ${res.status}`);
  return res.json();
};

export const postSettings: PostSettings = (target, changes) => {
  const req: SettingsWriteRequest = { changes };
  if (target !== GLOBAL_TARGET) req.project = target;
  return postJSON("/v1/settings", req);
};

/** Reads one target's settings; the page cache holds one entry per target. */
export function useSettings(target: string, conn?: Connection, fetcher: FetchSettings = fetchSettings): FetchedState<SettingsReadResponse> {
  return useFetched("settings", () => fetcher(target), conn, target);
}

export const setChange = (key: string, value: unknown): SettingChange => ({ key, value });

export const removeChange = (key: string): SettingChange => ({ key, remove: true });

/** "changed" means the target's file holds the key, whatever its value. */
export type RowState = "default" | "set";

export const rowState = (e: Pick<SettingsEntry, "set">): RowState => (e.set ? "set" : "default");

export interface SectionGroup {
  section: string;
  entries: SettingsEntry[];
  changed: number;
}

export const changedCount = (entries: Pick<SettingsEntry, "set">[]): number => entries.filter((e) => e.set).length;

/** Groups entries by section, sections and entries keeping the order the server sent. */
export function groupBySection(entries: SettingsEntry[]): SectionGroup[] {
  const groups = new Map<string, SectionGroup>();
  for (const e of entries) {
    let g = groups.get(e.section);
    if (!g) {
      g = { section: e.section, entries: [], changed: 0 };
      groups.set(e.section, g);
    }
    g.entries.push(e);
    if (e.set) g.changed++;
  }
  return [...groups.values()];
}

/** The row's display name: the last path segment, spaced and capitalised. */
export const entryLabel = (e: SettingsEntry) => {
  const text = (e.path[e.path.length - 1] ?? e.key).replace(/-/g, " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
};

/** Entries whose name, key path or description contains the query, case-insensitively; a blank query matches none. */
export function searchEntries(entries: SettingsEntry[], query: string): SettingsEntry[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  return entries.filter((e) => [entryLabel(e), e.key, e.description].some((s) => s.toLowerCase().includes(q)));
}

// Hints are for the browser only; the server's check decides. Each returns an error sentence or undefined.

const NUMBER = /^-?\d+(\.\d+)?$/;
const DURATION = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;
const SIZE = /^\d+(\.\d+)?\s*([kmgtp]i?b?)?$/i;

export function numberListHint(draft: string): string | undefined {
  const bad = draft
    .split(",")
    .map((s) => s.trim())
    .find((s) => !NUMBER.test(s));
  return bad === undefined ? undefined : `"${bad}" is not a number`;
}

export function durationHint(draft: string): string | undefined {
  return DURATION.test(draft.trim()) ? undefined : "Use a duration such as 30s, 5m or 1h30m";
}

export function sizeHint(draft: string): string | undefined {
  return SIZE.test(draft.trim()) ? undefined : "Use a size such as 500M or 50G";
}

/** The hint for a draft of the given entry type; types with no browser-side rule have none. */
export function draftHint(type: string, draft: string): string | undefined {
  if (type === "number-list") return numberListHint(draft);
  if (type === "duration") return durationHint(draft);
  if (type === "size") return sizeHint(draft);
  return undefined;
}
