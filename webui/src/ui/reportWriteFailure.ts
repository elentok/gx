import { WriteError } from "../useWrite"
import { showToast } from "./toaster"

/** The one error toast for a failed write: a WriteError shows its message, anything else a reach failure. */
export function reportWriteFailure(err: unknown): void {
  showToast({
    type: "error",
    title: err instanceof WriteError ? err.message : "Could not reach gx. Try again.",
  })
}

/** Fire-and-forget a write; a failure toasts and leaves the control as the last snapshot showed it. */
export function guarded(write: Promise<unknown>): void {
  void write.catch(reportWriteFailure)
}
