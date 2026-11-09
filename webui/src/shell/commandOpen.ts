import { useSyncExternalStore } from "react";

// Open state of the Command palette (Cmd/Ctrl+E), kept outside React state like jumpOpen.ts.
let open = false;
const listeners = new Set<() => void>();

export function setCommandOpen(next: boolean | ((o: boolean) => boolean)) {
  const value = typeof next === "function" ? next(open) : next;
  if (value === open) return;
  open = value;
  listeners.forEach((l) => l());
}

export function useCommandOpen(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => open,
  );
}
