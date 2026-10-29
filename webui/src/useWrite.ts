import { useRef, useState } from "react";
import { showToast, type ToastSpec } from "./ui/toaster";

/** A non-200 answer: the message is the server's plain-text body. */
export class WriteError extends Error {}

export interface Refusable {
  refused?: boolean;
  reason?: string;
  message?: string;
}

export const refusalText = (res: Refusable) => res.message || res.reason || "The request was refused.";

export interface RunOptions<R> {
  /** Runs after a write the server accepted. */
  onDone?: (res: R) => void;
  /** A toast for a success the page does not show by itself (the card does not move, say). */
  success?: (res: R) => ToastSpec | undefined;
}

/**
 * Runs one write at a time and tells the person how it went: a pending flag for the button's
 * spinner, an error or refusal toast, and a success toast when the caller asks for one. The
 * refusal and the failure text are also returned so a caller can show them inline.
 */
export function useWrite({ toastRefusal = true }: { toastRefusal?: boolean } = {}) {
  const [pending, setPending] = useState(false);
  const inFlight = useRef(false);
  const [refusal, setRefusal] = useState<Refusable>();
  const [error, setError] = useState<string>();

  async function run<R extends Refusable>(call: () => Promise<R>, { onDone, success }: RunOptions<R> = {}) {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setRefusal(undefined);
    setError(undefined);
    try {
      const res = await call();
      if (res.refused) {
        setRefusal(res);
        if (toastRefusal) showToast({ type: "error", title: refusalText(res) });
        return;
      }
      const toast = success?.(res);
      if (toast) showToast(toast);
      onDone?.(res);
    } catch (err) {
      const title = err instanceof WriteError ? err.message : "Could not reach gx. Try again.";
      setError(title);
      showToast({ type: "error", title });
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }

  return { run, pending, refusal, error };
}
