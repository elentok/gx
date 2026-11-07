import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type RefObject } from "react";
import { requestPromptFocus } from "../agents/promptFocus";
import { isMod, isTextTarget } from "./keys";
import type { Shell } from "./ShellContext";

/** Marks every sidebar row anchor and section header: the stops of the roving focus. */
export const NAV_ATTR = "data-nav";
const NAV = `[${NAV_ATTR}]`;
/** Marks a chat row's anchor: Enter on it also focuses the chat's prompt. */
export const CHAT_ATTR = "data-chat";

const items = (root: HTMLElement) => Array.from(root.querySelectorAll<HTMLElement>(NAV));

/**
 * One tab stop for the whole sidebar: the focused row (or the first) has tabindex 0, the rest -1.
 * The rows come and go with the epic list, so a mutation observer re-applies it.
 */
function useRovingFocus(root: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = root.current;
    if (!el) return;
    let stop: HTMLElement | undefined;
    const apply = () => {
      const all = items(el);
      if (!stop || !all.includes(stop)) stop = all[0];
      for (const it of all) it.tabIndex = it === stop ? 0 : -1;
    };
    const onFocusIn = (e: FocusEvent) => {
      const t = e.target instanceof HTMLElement ? e.target.closest<HTMLElement>(NAV) : null;
      if (t) {
        stop = t;
        apply();
      }
    };
    apply();
    const observer = new MutationObserver(apply);
    observer.observe(el, { childList: true, subtree: true });
    el.addEventListener("focusin", onFocusIn);
    return () => {
      observer.disconnect();
      el.removeEventListener("focusin", onFocusIn);
    };
  }, [root]);
}

/** Moves between rows with Down/Up or j/k, expands with Right/l and collapses with Left/h; Enter opens the row. Only while a row has focus. */
function onSidebarKey(root: HTMLElement, e: ReactKeyboardEvent) {
  const target = e.target;
  if (e.altKey || e.metaKey || e.ctrlKey || isTextTarget(target)) return;
  if (!(target instanceof HTMLElement) || !target.matches(NAV)) return;
  const step = e.key === "ArrowDown" || e.key === "j" ? 1 : e.key === "ArrowUp" || e.key === "k" ? -1 : 0;
  if (step) {
    e.preventDefault();
    const all = items(root);
    all[all.indexOf(target) + step]?.focus();
    return;
  }
  const open = e.key === "ArrowRight" || e.key === "l" ? true : e.key === "ArrowLeft" || e.key === "h" ? false : undefined;
  if (open !== undefined) {
    e.preventDefault();
    const toggle = target.matches("button") ? target : target.closest("li")?.querySelector<HTMLElement>("button[data-chevron]");
    if (toggle && toggle.getAttribute("aria-expanded") !== String(open)) toggle.click();
    return;
  }
  if (e.key === "Enter" && target.matches("a")) {
    e.preventDefault();
    if (target.hasAttribute(CHAT_ATTR)) requestPromptFocus();
    target.click();
  }
}

/** Cmd/Ctrl+B toggles the sidebar and focuses its tab stop when it opens; the returned handler is the aside's onKeyDown. */
export function useSidebarKeys(aside: RefObject<HTMLElement | null>, shell: Shell) {
  useRovingFocus(aside);
  const focusOnOpen = useRef(false);
  const { sidebarOpen, toggle } = shell;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!isMod(e) || e.altKey || e.shiftKey || e.key.toLowerCase() !== "b" || isTextTarget(e.target)) return;
      e.preventDefault();
      focusOnOpen.current = !sidebarOpen;
      toggle();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [sidebarOpen, toggle]);
  useEffect(() => {
    if (!focusOnOpen.current || !sidebarOpen) return;
    focusOnOpen.current = false;
    const all = aside.current ? items(aside.current) : [];
    (all.find((i) => i.tabIndex === 0) ?? all[0])?.focus();
  }, [sidebarOpen, aside]);
  return (e: ReactKeyboardEvent) => aside.current && onSidebarKey(aside.current, e);
}
