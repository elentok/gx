import { hasLayer } from "../ui/modal/layers";

// Anything open that takes Esc first. The completion menu's own handler has not run when we judge, so it is
// found by its DOM; the Questions view leaves on this same Escape.
const ON_TOP = '[role="dialog"],[role="alertdialog"],[role="menu"],[role="listbox"],[data-questions-view]';

/**
 * Whether an Escape keydown aimed at `target` should open the Interrupt confirm.
 *
 * - Vi editor (`data-vi-mode` on an ancestor): NORMAL lets Esc through; INSERT and VISUAL keep it for vim.
 * - The Vi editor's own dialog is not "something on top".
 * - Outside the Vi editor, any layer in the `ui/modal/layers` stack (`hasLayer()`) takes the Esc.
 * - Any open `ON_TOP` element (dialog, alertdialog, menu, listbox, Questions view) takes the Esc.
 */
export function escOpensInterrupt(target: EventTarget | null): boolean {
  const vi = target instanceof Element ? target.closest("[data-vi-mode]") : null;
  if (vi && vi.getAttribute("data-vi-mode") !== "NORMAL") return false;
  const own = vi?.closest('[role="dialog"]');
  if (!vi && hasLayer()) return false;
  for (const el of document.querySelectorAll(ON_TOP)) if (el !== own) return false;
  return true;
}
