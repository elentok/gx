import { apiFetch } from "../apiFetch"
import { postJSON } from "../auth/http"
import { reasonText } from "../auth/refusals"
import type { Refusable } from "../useWrite"

export interface PushState {
  public_key: string
  contact_set: boolean
  subscribed: boolean
  endpoint: string
  last_failure?: { reason: string; at: string }
}

export interface SubscriptionBody {
  endpoint: string
  p256dh: string
  auth: string
  vapid_key: string
}

export interface PushResult extends Refusable {
  sent?: boolean
  failure?: string
}

async function post(path: string, body: object = {}): Promise<PushResult> {
  const out = await postJSON<PushResult>(path, body)
  if (out.refused) out.message = reasonText(out)
  return out
}

export async function fetchPushState(): Promise<PushState> {
  const res = await apiFetch("/v1/web/push/state/")
  if (!res.ok) throw new Error(`Push state failed (HTTP ${res.status}).`)
  return res.json()
}

export const subscribePush = (body: SubscriptionBody) => post("/v1/web/push/subscribe", body)
export const renewPush = (body: SubscriptionBody & { old_endpoint: string }) =>
  post("/v1/web/push/renew", body)
export const removePush = () => post("/v1/web/push/remove")
export const testPush = () => post("/v1/web/push/test")
