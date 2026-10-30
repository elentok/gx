import { signOut } from "../apiFetch";
import { postJSON } from "../auth/http";
import { formatAge } from "../epics/cardTime";
import type { Refusable } from "../useWrite";

export interface WebLoginEntry {
  id: string;
  name: string;
  created?: string;
  last_used: string;
  last_ip: string;
  this_device: boolean;
}

export type FetchLogins = () => Promise<WebLoginEntry[]>;

export const fetchLogins: FetchLogins = async () => (await postJSON<{ logins: WebLoginEntry[] }>("/v1/web/logins")).logins;

export type LoginWrite = (id: string, name?: string) => Promise<Refusable>;

export const renameLogin: LoginWrite = (id, name) => postJSON("/v1/web/logins/rename", { id, name });

export const revokeLogin: LoginWrite = (id) => postJSON("/v1/web/logins/revoke", { id });

/** Ends this browser's login on the server, then signs the page out, whatever the server answered. */
export async function signOutThisDevice(): Promise<void> {
  try {
    await postJSON("/v1/web/auth/sign-out");
  } finally {
    signOut();
  }
}

const DAY = 24 * 60 * 60 * 1000;
// Matches webLoginIdleLimit on the server.
const IDLE_LIMIT_DAYS = 90;
const WARN_BELOW_DAYS = 30;

/** Whole days until the login ends unless used, only when under 30 are left. */
export function endsInDays(lastUsed: string, now: number): number | undefined {
  const t = Date.parse(lastUsed);
  if (Number.isNaN(t)) return undefined;
  const left = IDLE_LIMIT_DAYS * DAY - (now - t);
  return left < WARN_BELOW_DAYS * DAY ? Math.max(0, Math.ceil(left / DAY)) : undefined;
}

/** "Now · 192.168.1.23", "2h ago · 100.101.12.4"; the IP is left out when unknown. */
export function lastSeenText(login: Pick<WebLoginEntry, "last_used" | "last_ip">, now: number): string {
  const age = formatAge(now - Date.parse(login.last_used));
  return [age === "<1m" ? "Now" : `${age} ago`, login.last_ip].filter(Boolean).join(" · ");
}
