import { postJSON } from "../auth/http";
import { reasonText } from "../auth/refusals";
import type { Refusable } from "../useWrite";

export interface AccountResult extends Refusable {
  wait_seconds?: number;
  secret?: string;
  uri?: string;
}

export type AccountCall = (body: object) => Promise<AccountResult>;

async function post(path: string, body: object): Promise<AccountResult> {
  const out = await postJSON<AccountResult>(path, body);
  if (out.refused) out.message = reasonText(out);
  return out;
}

export const changePassword: AccountCall = (body) => post("/v1/web/account/password", body);
export const beginReset: AccountCall = (body) => post("/v1/web/account/authenticator/begin", body);
export const finishReset: AccountCall = (body) => post("/v1/web/account/authenticator/finish", body);
