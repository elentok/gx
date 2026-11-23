import { useSyncExternalStore } from "react"
import { fetchLogins } from "../settings/logins"
import { fetchPushState, type SubscriptionBody } from "./pushApi"

export interface PushDevice {
  supported: boolean
  permission: NotificationPermission
  /** This browser holds a push subscription. */
  browserSub: boolean
  /** The server holds a record for this login. */
  serverRecord: boolean
  /** The browser subscription was made with the server's current key. */
  keyMatches: boolean
  /** The name of the login of this device. */
  loginName: string | undefined
  /** The server has a contact email, so it can sign pushes. */
  contactSet: boolean
  /** The reason and time of the last failed send, until the next success. */
  failureReason: string | undefined
  failureAt: string | undefined
  /** An open-time check dropped this device's subscription because the server key changed. */
  keyChanged: boolean
  on: boolean
}

const OFF: PushDevice = {
  supported: false,
  permission: "default",
  browserSub: false,
  serverRecord: false,
  keyMatches: false,
  loginName: undefined,
  contactSet: true,
  failureReason: undefined,
  failureAt: undefined,
  keyChanged: false,
  on: false,
}

const KEY_CHANGED = "gx.push.keyChanged"

export function readKeyChanged(): boolean {
  try {
    return localStorage.getItem(KEY_CHANGED) === "1"
  } catch {
    return false
  }
}

export function setKeyChanged(changed: boolean) {
  try {
    if (changed) localStorage.setItem(KEY_CHANGED, "1")
    else localStorage.removeItem(KEY_CHANGED)
  } catch {
    // The page works without storage; the reason is only shown this session.
  }
}

export function subscriptionBody(sub: PushSubscription, vapidKey: string): SubscriptionBody {
  const { endpoint, keys } = sub.toJSON()
  return {
    endpoint: endpoint ?? sub.endpoint,
    p256dh: keys?.p256dh ?? "",
    auth: keys?.auth ?? "",
    vapid_key: vapidKey,
  }
}

let current = OFF
const listeners = new Set<() => void>()

function set(next: PushDevice) {
  if ((Object.keys(next) as (keyof PushDevice)[]).every((k) => next[k] === current[k])) return
  current = next
  for (const l of listeners) l()
}

export function pushSupported(): boolean {
  return (
    typeof Notification !== "undefined" &&
    "serviceWorker" in navigator &&
    typeof PushManager !== "undefined"
  )
}

export function decodeKey(b64url: string): Uint8Array<ArrayBuffer> {
  const pad = "=".repeat((4 - (b64url.length % 4)) % 4)
  const raw = atob((b64url + pad).replace(/-/g, "+").replace(/_/g, "/"))
  return Uint8Array.from(raw, (c) => c.charCodeAt(0))
}

export function subscriptionHasKey(sub: PushSubscription, key: Uint8Array): boolean {
  const have = sub.options.applicationServerKey
  if (!have) return false
  const bytes = new Uint8Array(have)
  return bytes.length === key.length && bytes.every((b, i) => b === key[i])
}

/** Re-reads permission, the browser subscription and the server's record. A failed read keeps what is shown. */
export async function refreshPushDevice(): Promise<void> {
  if (!pushSupported()) {
    set(OFF)
    return
  }
  try {
    const registration = await navigator.serviceWorker.ready
    const [sub, state, logins] = await Promise.all([
      registration.pushManager.getSubscription(),
      fetchPushState(),
      fetchLogins(),
    ])
    const permission = Notification.permission
    const browserSub = sub !== null
    const serverRecord = state.subscribed
    const keyMatches = sub !== null && subscriptionHasKey(sub, decodeKey(state.public_key))
    set({
      supported: true,
      permission,
      browserSub,
      serverRecord,
      keyMatches,
      loginName: logins.find((l) => l.this_device)?.name,
      contactSet: state.contact_set,
      failureReason: state.last_failure?.reason,
      failureAt: state.last_failure?.at,
      keyChanged: readKeyChanged(),
      on: permission === "granted" && browserSub && serverRecord && keyMatches,
    })
  } catch {
    // Keep the last state; the page shows what it knew.
  }
}

export const getDevice = (): PushDevice => current

/** The state of push on this device. The first reader starts a refresh. */
export function usePushDevice(): PushDevice {
  return useSyncExternalStore(subscribe, getDevice)
}

// Module level, so a render never resubscribes and so never restarts the refresh.
function subscribe(l: () => void) {
  if (listeners.size === 0) void refreshPushDevice()
  listeners.add(l)
  return () => listeners.delete(l)
}

export function resetPushDeviceForTest() {
  set(OFF)
}
