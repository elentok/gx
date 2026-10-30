import { apiFetch } from "../apiFetch";
import { TOTP_MISMATCH } from "../auth/AddAuthenticator";
import { WriteError, type Refusable } from "../useWrite";

export interface AccountResult extends Refusable {
  wait_seconds?: number;
  secret?: string;
  uri?: string;
}

export type AccountCall = (body: object) => Promise<AccountResult>;

const REFUSALS: Record<string, string> = {
  "wrong-credentials": "The password or code is wrong.",
  "password-too-short": "The new password is too short. Use at least 12 characters.",
  "totp-mismatch": TOTP_MISMATCH,
  "totp-off": "The authenticator is turned off.",
};

function refusalMessage(res: AccountResult): string | undefined {
  if (res.reason === "rate-limited") return `Too many tries. Wait ${res.wait_seconds ?? 0} seconds and try again.`;
  return res.reason && REFUSALS[res.reason];
}

async function post(path: string, body: object): Promise<AccountResult> {
  const res = await apiFetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  if (!res.ok) throw new WriteError(`Could not save (HTTP ${res.status}).`);
  const out: AccountResult = await res.json();
  if (out.refused) out.message = refusalMessage(out);
  return out;
}

export const changePassword: AccountCall = (body) => post("/v1/web/account/password", body);
export const beginReset: AccountCall = (body) => post("/v1/web/account/authenticator/begin", body);
export const finishReset: AccountCall = (body) => post("/v1/web/account/authenticator/finish", body);
