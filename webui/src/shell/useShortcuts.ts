import { useEffect, useSyncExternalStore } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext } from "../connection";
import type { FetchEpicList } from "../epics/list";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { digitOf, isTextTarget } from "./keys";
import { useSidebarModel } from "./SidebarRows";

// Alt-held lives outside React so only the leaf that reads it (a row's slot) re-renders on Alt; no page component does.
let altHeld = false;
const altListeners = new Set<() => void>();
const setAlt = (v: boolean) => {
  if (altHeld === v) return;
  altHeld = v;
  altListeners.forEach((l) => l());
};

// A blur drops it, since the keyup goes to another window. Keydown stands down in a text field; keyup never does.
const onAltDown = (e: KeyboardEvent) => e.key === "Alt" && !isTextTarget(e.target) && setAlt(true);
const onAltUp = (e: KeyboardEvent) => e.key === "Alt" && setAlt(false);
const onAltOff = () => setAlt(false);

function subscribeAlt(l: () => void) {
  if (altListeners.size === 0) {
    window.addEventListener("keydown", onAltDown);
    window.addEventListener("keyup", onAltUp);
    window.addEventListener("blur", onAltOff);
  }
  altListeners.add(l);
  return () => {
    altListeners.delete(l);
    if (altListeners.size === 0) {
      window.removeEventListener("keydown", onAltDown);
      window.removeEventListener("keyup", onAltUp);
      window.removeEventListener("blur", onAltOff);
      altHeld = false;
    }
  };
}

/** True while Alt is held. Call it only from a leaf component. */
export function useAltHeld(): boolean {
  return useSyncExternalStore(subscribeAlt, () => altHeld, () => false);
}

/** Alt+1..9 open the nth numbered sidebar row; Alt+Shift+1..9 pick the nth workspace in the switcher's order. */
export function useShortcuts({ fetcher, active, setActive }: { fetcher?: FetchEpicList; active: string; setActive: (name: string) => void }) {
  const navigate = useNavigate();
  const { workspaces } = useWorkspaces(useConnectionContext());
  const model = useSidebarModel(fetcher, active);
  const rows = model?.rows;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.altKey || e.metaKey || e.ctrlKey || isTextTarget(e.target)) return;
      const n = digitOf(e);
      if (n === null) return;
      if (e.shiftKey) {
        const w = workspaces[n - 1];
        if (!w) return;
        e.preventDefault();
        setActive(w.name);
        return;
      }
      const row = rows?.find((r) => r.number === n);
      if (!row) return;
      e.preventDefault();
      navigate(row.href.replace(/^#/, ""));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [rows, workspaces, setActive, navigate]);
}
