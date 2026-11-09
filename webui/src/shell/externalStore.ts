import { useSyncExternalStore } from "react";

export type ExternalStore<T> = {
  get: () => T;
  /** Stores `next` and notifies, unless it equals the current value. */
  set: (next: T) => void;
  /** useSyncExternalStore's subscribe. `onFirst` runs when the first listener joins and returns its cleanup, run when the last leaves. */
  subscribe: (listener: () => void) => () => void;
  use: () => T;
};

/** A value kept outside React so only the components that read it re-render when it changes. */
export function createExternalStore<T>(initial: T, onFirst?: () => () => void): ExternalStore<T> {
  let value = initial;
  let stop: (() => void) | undefined;
  const listeners = new Set<() => void>();
  const get = () => value;
  const set = (next: T) => {
    if (next === value) return;
    value = next;
    listeners.forEach((l) => l());
  };
  const subscribe = (l: () => void) => {
    if (listeners.size === 0) stop = onFirst?.();
    listeners.add(l);
    return () => {
      listeners.delete(l);
      if (listeners.size === 0) {
        stop?.();
        stop = undefined;
      }
    };
  };
  return { get, set, subscribe, use: () => useSyncExternalStore(subscribe, get, () => initial) };
}
