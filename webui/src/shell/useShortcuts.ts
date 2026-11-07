import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext } from "../connection";
import type { FetchEpicList } from "../epics/list";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { digitOf, isTextTarget } from "./keys";
import { useSidebarModel } from "./SidebarRows";

/** True while Alt is held; a blur drops it, since the keyup goes to another window. */
export function useAltHeld(): boolean {
  const [held, setHeld] = useState(false);
  useEffect(() => {
    const down = (e: KeyboardEvent) => e.key === "Alt" && setHeld(true);
    const up = (e: KeyboardEvent) => e.key === "Alt" && setHeld(false);
    const off = () => setHeld(false);
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    window.addEventListener("blur", off);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
      window.removeEventListener("blur", off);
    };
  }, []);
  return held;
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
