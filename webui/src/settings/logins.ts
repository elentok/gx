import { apiFetch, signOut } from "../apiFetch";
import { formatAge } from "../epics/cardTime";
import { WriteError, type Refusable } from "../useWrite";

export interface WebLoginEntry {
  id: string;
  name: string;
  created?: string;
  last_used: string;
  last_ip: string;
  this_device: boolean;
}

export type FetchLogins = () => Promise<WebLoginEntry[]>;

export const fetchLogins: FetchLogins = async () => {
  const res = await apiFetch("/v1/web/logins", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
  if (!res.ok) throw new Error(`web logins: HTTP ${res.status}`);
  return (await res.json()).logins;
};

export type LoginWrite = (id: string, name?: string) => Promise<Refusable>;

async function postLogin(path: string, body: object): Promise<Refusable> {
  const res = await apiFetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  if (!res.ok) throw new WriteError(`Could not save (HTTP ${res.status}).`);
  return res.json();
}

export const renameLogin: LoginWrite = (id, name) => postLogin("/v1/web/logins/rename", { id, name });

export const revokeLogin: LoginWrite = (id) => postLogin("/v1/web/logins/revoke", { id });

/** Ends this browser's login on the server, then signs the page out. */
export async function signOutThisDevice(): Promise<void> {
  await apiFetch("/v1/web/auth/sign-out", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
  signOut();
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
