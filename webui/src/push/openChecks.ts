import {
  decodeKey,
  pushSupported,
  setKeyChanged,
  subscriptionBody,
  subscriptionHasKey,
} from "./deviceState"
import { fetchPushState, removePush, renewPush } from "./pushApi"

/** Brings the browser subscription and the server record back in line, once per app open. A failure leaves both as they were. */
export async function runOpenChecks(): Promise<void> {
  if (!pushSupported()) return
  try {
    const { pushManager } = await navigator.serviceWorker.ready
    const [sub, state] = await Promise.all([pushManager.getSubscription(), fetchPushState()])
    if (!sub) {
      if (state.subscribed) await removePush()
      return
    }
    if (!subscriptionHasKey(sub, decodeKey(state.public_key))) {
      await sub.unsubscribe()
      if (state.subscribed) await removePush()
      setKeyChanged(true)
      return
    }
    if (state.subscribed && state.endpoint !== sub.endpoint) {
      await renewPush({
        ...subscriptionBody(sub, state.public_key),
        old_endpoint: state.endpoint,
      })
    }
  } catch {
    // The next open tries again.
  }
}
