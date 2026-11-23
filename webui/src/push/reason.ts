import { isIOS, isStandalone } from "../platform"
import { pushSupported, type PushDevice } from "./deviceState"

export interface DisabledReason {
  /** Replaces the small line under the switch label. */
  line?: string
  /** The box under the switch card. */
  message?: string
  /** The switch cannot be turned on. */
  disabled: boolean
}

export const KEY_CHANGED_MESSAGE = "The server key changed. Turn push on again."

/** The first reason push cannot start on this device, or undefined. The iPhone check comes first: a plain Safari tab there has no PushManager. */
export function disabledReason(device: PushDevice): DisabledReason | undefined {
  if (isIOS() && !isStandalone()) {
    return {
      line: "Needs the Home Screen app",
      message:
        "On iPhone, push works only in the installed app. Tap Share, then Add to Home Screen, and open gx from there.",
      disabled: true,
    }
  }
  if (!pushSupported()) {
    return {
      line: "Not supported",
      message:
        "This browser has no push support. Use Safari on iPhone or Mac, or another current browser.",
      disabled: true,
    }
  }
  if (device.permission === "denied") {
    return {
      line: "Blocked",
      message:
        "This browser blocks notifications for gx. Allow them in the browser or iOS Settings, then reopen this page.",
      disabled: true,
    }
  }
  if (!device.contactSet) {
    return { line: "Set a contact email in the server config", disabled: true }
  }
  if (device.keyChanged && !device.on) return { message: KEY_CHANGED_MESSAGE, disabled: false }
  return undefined
}
