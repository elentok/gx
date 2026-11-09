import { useEffect } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext } from "../connection";
import type { FetchEpicList } from "../epics/list";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { createExternalStore } from "./externalStore";
import { digitOf, isTextTarget } from "./keys";
import { useSidebarModel } from "./SidebarRows";

// Alt-held lives outside React so only the leaf that reads it (a row's slot) re-renders on Alt; no page component does.
// A blur drops it, since the keyup goes to another window. Keydown stands down in a text field; keyup never does.
const altStore = createExternalStore(false, () => {
  const onDown = (e: KeyboardEvent) => e.key === "Alt" && !isTextTarget(e.target) && altStore.set(true);
  const onUp = (e: KeyboardEvent) => e.key === "Alt" && altStore.set(false);
  const onOff = () => altStore.set(false);
  window.addEventListener("keydown", onDown);
  window.addEventListener("keyup", onUp);
  window.addEventListener("blur", onOff);
  return () => {
    window.removeEventListener("keydown", onDown);
    window.removeEventListener("keyup", onUp);
    window.removeEventListener("blur", onOff);
    altStore.set(false);
  };
});

/** True while Alt is held. Call it only from a leaf component. */
export const useAltHeld = altStore.use;

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
