import { useCallback } from "react";
import { useWide } from "../../useWide";

/**
 * Ref callback for a dialog's first text field: focuses it when it mounts, on tablet and desktop only.
 * On a phone the dialog panel keeps focus, so the on-screen keyboard stays closed.
 * Acts at the field's mount, not the dialog's, because some fields render after a load; and only while
 * focus is still on the panel or the body, so a person who already moved to another field keeps their place.
 */
export function useFocusFirstField() {
  const wide = useWide();
  return useCallback(
    (el: HTMLElement | null) => {
      if (!el || !wide) return;
      const active = document.activeElement;
      if (active && active !== document.body && active !== el.closest('[role="dialog"]')) return;
      el.focus();
    },
    [wide],
  );
}
