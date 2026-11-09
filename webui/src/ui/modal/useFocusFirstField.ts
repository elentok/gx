import { useCallback } from "react";
import { useWide } from "../../useWide";

// Dialogs the person has pressed or typed in. Focus that landed inside a dialog on its own (a select
// or a load grabbing it) is not the person moving, so only these dialogs keep their focus.
const touched = new WeakSet<Element>();
if (typeof document !== "undefined") {
  const mark = (e: Event) => {
    const dialog = e.target instanceof Element ? e.target.closest('[role="dialog"]') : null;
    if (dialog) touched.add(dialog);
  };
  document.addEventListener("pointerdown", mark, true);
  document.addEventListener("keydown", mark, true);
}

/**
 * Ref callback for a dialog's first text field: focuses it when it mounts, on tablet and desktop only.
 * On a phone the dialog panel keeps focus, so the on-screen keyboard stays closed.
 * Acts at the field's mount, not the dialog's, because some fields render after a load; and it takes
 * focus from anywhere outside the dialog, or from the panel itself, so a person who already moved to
 * another field inside the dialog keeps their place.
 */
export function useFocusFirstField() {
  const wide = useWide();
  return useCallback(
    (el: HTMLElement | null) => {
      if (!el || !wide) return;
      // Lets a closing menu hand focus here when its own focus return lands after this mount.
      el.dataset.firstField = "";
      const active = document.activeElement;
      const dialog = el.closest('[role="dialog"]');
      if (active && dialog?.contains(active) && active !== dialog && touched.has(dialog)) return;
      el.focus();
    },
    [wide],
  );
}
