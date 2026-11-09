import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type RefObject } from "react";
import { useNavigate, type NavigateFunction } from "react-router";
import { FOCUS_PROMPT_STATE } from "../agents/promptFocus";
import { isTextTarget } from "./keys";
import { hasLayer } from "../ui/modal/layers";
import type { Shell } from "./ShellContext";

/** Marks every sidebar row anchor and section header: the stops of the roving focus. */
export const NAV_ATTR = "data-nav";
const NAV = `[${NAV_ATTR}]`;
/** Marks a chat row's anchor: Enter on it also focuses the chat's prompt. */
export const CHAT_ATTR = "data-chat";

/** Marks the sidebar's list: the focus target when no row is left. */
export const LIST_ATTR = "data-nav-list";

const items = (root: HTMLElement) => Array.from(root.querySelectorAll<HTMLElement>(NAV));
const keyOf = (it: HTMLElement) => it.getAttribute(NAV_ATTR) ?? "";

/**
 * One tab stop for the whole sidebar: the focused row (or the first) has tabindex 0, the rest -1.
 * The stop is tracked by the stop's key (the NAV_ATTR value), not its element or position, so rows
 * reordering as data changes keep it. Moving a focused node blurs it, so after a mutation focus is put
 * back on the same key; when that key is gone it lands on the row now at the old index (clamped), or
 * on the list when empty.
 */
function useRovingFocus(root: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = root.current;
    if (!el) return;
    let stop = "";
    let keys: string[] = [];
    // Set while focus is in a stop; cleared once focus has left on purpose (not by a reorder).
    let focused = false;
    const apply = () => {
      const all = items(el);
      const next = all.map(keyOf);
      let target = all.find((it) => keyOf(it) === stop);
      if (!target) target = all[Math.max(Math.min(keys.indexOf(stop), all.length - 1), 0)];
      stop = target ? keyOf(target) : "";
      keys = next;
      for (const it of all) it.tabIndex = it === target ? 0 : -1;
      if (focused && !el.contains(document.activeElement)) {
        (target ?? el.querySelector<HTMLElement>(`[${LIST_ATTR}]`))?.focus();
      }
    };
    const onFocusIn = (e: FocusEvent) => {
      const t = e.target instanceof HTMLElement ? e.target.closest<HTMLElement>(NAV) : null;
      focused = !!t;
      if (t) {
        stop = keyOf(t);
        apply();
      }
    };
    // A reorder's blur is followed by the observer's microtask, queued earlier than this one.
    const onFocusOut = () => queueMicrotask(() => (focused = el.contains(document.activeElement)));
    apply();
    const observer = new MutationObserver(apply);
    observer.observe(el, { childList: true, subtree: true });
    el.addEventListener("focusin", onFocusIn);
    el.addEventListener("focusout", onFocusOut);
    return () => {
      observer.disconnect();
      el.removeEventListener("focusin", onFocusIn);
      el.removeEventListener("focusout", onFocusOut);
    };
  }, [root]);
}

/** Moves between rows with Down/Up or j/k, expands with Right/l and collapses with Left/h; Enter opens the row. Only while a row has focus. */
function onSidebarKey(root: HTMLElement, e: ReactKeyboardEvent, navigate: NavigateFunction) {
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
    const href = target.getAttribute("href");
    if (target.hasAttribute(CHAT_ATTR) && href?.startsWith("#")) navigate(href.slice(1), { state: FOCUS_PROMPT_STATE });
    else target.click();
  }
}

/** Cmd/Ctrl+B toggles the sidebar and focuses its tab stop when it opens; the returned handler is the aside's onKeyDown. */
export function useSidebarKeys(aside: RefObject<HTMLElement | null>, shell: Shell) {
  useRovingFocus(aside);
  const navigate = useNavigate();
  const focusOnOpen = useRef(false);
  const { sidebarOpen, toggle } = shell;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Either modifier, on any platform: a menu holding focus must not make Ctrl+B on a Mac a dead key.
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey || e.key.toLowerCase() !== "b" || isTextTarget(e.target)) return;
      // A dialog opened from the sidebar owns the key.
      if (hasLayer()) return;
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
  return (e: ReactKeyboardEvent) => aside.current && onSidebarKey(aside.current, e, navigate);
}
