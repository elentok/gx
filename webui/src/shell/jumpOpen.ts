import { useSyncExternalStore } from "react";

// Shared by the palette's Cmd/Ctrl+K handler and the header's search button. Kept outside React state
// so opening the palette re-renders only the palette, not AppShell.
let open = false;
const listeners = new Set<() => void>();

export function setJumpOpen(next: boolean | ((o: boolean) => boolean)) {
  const value = typeof next === "function" ? next(open) : next;
  if (value === open) return;
  open = value;
  listeners.forEach((l) => l());
}

export function useJumpOpen(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => open,
  );
}
