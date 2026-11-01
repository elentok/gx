import { useEffect, useState } from "react";
import { apiFetch } from "../apiFetch";
import type { SessionCommandEntry, SessionCommandsResponse } from "../generated/server";

/** A session id, or the project + dir of a First prompt. */
export type CommandTarget = { sessionId: string } | { project: string; dir: string };

export type FetchCommands = (target: CommandTarget, kind: "/" | "$") => Promise<SessionCommandsResponse>;

export const fetchCommands: FetchCommands = async (target, kind) => {
  const q = new URLSearchParams({ kind });
  if ("sessionId" in target) q.set("session", target.sessionId);
  else {
    q.set("project", target.project);
    q.set("dir", target.dir);
  }
  const res = await apiFetch(`/v1/sessions/commands?${q}`);
  if (!res.ok) throw new Error(`session commands: HTTP ${res.status}`);
  return res.json();
};

export const COMMAND_LIST_MAX_AGE_MS = 60_000;

export interface CommandList {
  entries: SessionCommandEntry[];
  status: "loading" | "ready" | "failed";
  /** The harness did not answer: entries are only the built-ins. */
  degraded: boolean;
}

interface Cached {
  entries: SessionCommandEntry[];
  degraded: boolean;
  at: number;
}

// The last list per key, in memory only: it is never persisted.
const cache = new Map<string, Cached>();

export const clearCommandListCache = () => cache.clear();

const keyOf = (target: CommandTarget, kind: string) =>
  "sessionId" in target ? `${kind}|s|${target.sessionId}` : `${kind}|p|${target.project}|${target.dir}`;

const fromCache = (c: Cached | undefined): CommandList =>
  c ? { entries: c.entries, status: "ready", degraded: c.degraded } : { entries: [], status: "loading", degraded: false };

/** Shows the last list at once and re-reads it when it is older than a minute, each time `enabled` turns on (a menu opens). A failed refresh keeps the last list. */
export function useCommandList(target: CommandTarget, kind: "/" | "$", fetcher: FetchCommands = fetchCommands, enabled = true): CommandList {
  const key = keyOf(target, kind);
  const [state, setState] = useState<{ key: string; list: CommandList }>(() => ({ key, list: fromCache(cache.get(key)) }));
  // Switching key shows that key's cache on the same render, not one render late.
  const list = state.key === key ? state.list : fromCache(cache.get(key));

  useEffect(() => {
    const cached = cache.get(key);
    setState({ key, list: fromCache(cached) });
    if (!enabled) return;
    if (cached &&Date.now() - cached.at < COMMAND_LIST_MAX_AGE_MS) return;
    let live = true;
    fetcher(target, kind).then(
      (res) => {
        const entry = { entries: res.entries ?? [], degraded: res.degraded, at: Date.now() };
        cache.set(key, entry);
        if (live) setState({ key, list: fromCache(entry) });
      },
      () => {
        if (live && !cached) setState({ key, list: { entries: [], status: "failed", degraded: false } });
      },
    );
    return () => {
      live = false;
    };
    // target is rebuilt each render by callers; key is its identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, kind, fetcher, enabled]);

  return list;
}
