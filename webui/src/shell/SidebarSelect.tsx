import { Box, chakra } from "@chakra-ui/react";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { FetchEpicList } from "../epics/list";
import { Button } from "../ui/button/Button";
import { hasLayer } from "../ui/modal/layers";
import {
  GROUP_ON_HOLD,
  GROUP_OTHER,
  enter,
  groupState,
  initialSelect,
  leave,
  prune,
  rowOfAgent,
  selectAll,
  selectNone,
  toggle,
  toggleGroup,
  type GroupState,
  type SelectRow,
  type SelectState,
} from "../select/selectModel";
import { selectIntent } from "../select/selectKeys";
import type { SidebarRow } from "../workspaces/sidebarModel";
import { isTextTarget } from "./keys";
import { useSidebarModel } from "./SidebarRows";
import { SidebarBulkActions } from "./SidebarBulkActions";

/** One select-mode instance: the sidebar has one, the Epics page has its own. */
export interface SelectApi {
  state: SelectState;
  rows: SelectRow[];
  enter: () => void;
  leave: () => void;
  toggle: (key: string) => void;
  toggleGroup: (group: string) => void;
  groupState: (group: string) => GroupState;
  selectAll: () => void;
  selectNone: () => void;
  /** Drops these keys from the selection. */
  deselect: (keys: string[]) => void;
  /** A row's display name, for tooltips and toasts; the key when unknown. */
  nameOf: (key: string) => string;
  /** Call after an action ran: leaves select mode when nothing is selected any more. */
  afterRun: () => void;
}

export interface SidebarSelect extends SelectApi {
  /** Whether a section ("on-hold" or "other") is expanded; a collapsed section's rows are not visible to Select all. */
  sectionOpen: Record<string, boolean>;
  setSectionOpen: (group: string, open: boolean) => void;
}

/** The select model over `rows`, with Escape to leave. An unloaded list is not an empty one, so nothing is pruned until `loaded`. */
export function useSelectApi(rows: SelectRow[], names: Map<string, string>, loaded: boolean): SelectApi {
  const [state, setState] = useState<SelectState>(initialSelect);
  // A row that leaves the list leaves the selection.
  useEffect(() => {
    if (loaded) setState((s) => prune(s, rows));
  }, [loaded, rows]);

  const stateRef = useRef(state);
  stateRef.current = state;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      if (selectIntent(e, stateRef.current.on, hasLayer() || isTextTarget(e.target)) === "leave") setState(leave);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  return useMemo<SelectApi>(
    () => ({
      state,
      rows,
      enter: () => setState(enter),
      leave: () => setState(leave),
      toggle: (key) => setState((s) => toggle(s, key)),
      toggleGroup: (group) => setState((s) => toggleGroup(s, rows, group)),
      groupState: (group) => groupState(state, rows, group),
      selectAll: () => setState((s) => selectAll(s, rows)),
      selectNone: () => setState(selectNone),
      deselect: (keys) => setState((s) => ({ ...s, keys: s.keys.filter((k) => !keys.includes(k)) })),
      nameOf: (key) => names.get(key) ?? key,
      afterRun: () => setState((s) => prune(s, rows, true)),
    }),
    [state, rows, names],
  );
}

const SidebarSelectContext = createContext<SidebarSelect | null>(null);

/** The sidebar's select state. The sidebar rows only render inside the provider. */
export function useSidebarSelect(): SidebarSelect {
  const sel = useContext(SidebarSelectContext);
  if (!sel) throw new Error("useSidebarSelect needs a SidebarSelectProvider");
  return sel;
}

const rowOf = (r: SidebarRow, visible: boolean): SelectRow => ({
  key: r.key,
  kind: r.kind === "chat" || r.kind === "design" ? r.kind : "epic",
  visible,
  selectable: true,
  group: r.held ? GROUP_ON_HOLD : undefined,
  queued: r.queued,
  held: r.held,
  archivable: r.archivable,
  session: r.session,
});

/** Holds one select model for the whole sidebar, above the rows and the footer, so the selection survives navigation. */
export function SidebarSelectProvider({ fetcher, active, children }: { fetcher?: FetchEpicList; active: string; children: ReactNode }) {
  const model = useSidebarModel(fetcher, active);
  const [sectionOpen, setOpen] = useState<Record<string, boolean>>({});
  const setSectionOpen = useCallback((group: string, open: boolean) => setOpen((o) => ({ ...o, [group]: open })), []);

  // Agents nested under a row are not visible to Select all: their parent's expanded state is local to the row.
  const rows = useMemo<SelectRow[]>(() => {
    if (!model) return [];
    const out: SelectRow[] = [];
    for (const r of model.rows) {
      out.push(rowOf(r, true));
      for (const a of r.agents) out.push(rowOfAgent(a.session, false));
    }
    for (const r of model.held) {
      out.push(rowOf(r, !!sectionOpen[GROUP_ON_HOLD]));
      for (const a of r.agents) out.push(rowOfAgent(a.session, false));
    }
    for (const a of model.other) out.push(rowOfAgent(a.session, !!sectionOpen[GROUP_OTHER], GROUP_OTHER));
    return out;
  }, [model, sectionOpen]);

  const names = useMemo(() => {
    const out = new Map<string, string>();
    if (!model) return out;
    const add = (name: string, key: string) => out.set(key, name);
    for (const r of [...model.rows, ...model.held]) {
      add(r.title, r.key);
      for (const a of r.agents) add(a.label, a.session.id);
    }
    for (const a of model.other) add(a.label, a.session.id);
    return out;
  }, [model]);

  const api = useSelectApi(rows, names, !!model);
  const select = useMemo<SidebarSelect>(() => ({ ...api, sectionOpen, setSectionOpen }), [api, sectionOpen, setSectionOpen]);

  return <SidebarSelectContext.Provider value={select}>{children}</SidebarSelectContext.Provider>;
}

/** A row or section checkbox. The mixed state is a real indeterminate checkbox. */
export function SelectBox({ checked, mixed, label, onToggle }: { checked: boolean; mixed?: boolean; label: string; onToggle: () => void }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = !!mixed;
  }, [mixed]);
  return (
    <chakra.input
      ref={ref}
      type="checkbox"
      data-select-box
      aria-label={label}
      aria-checked={mixed ? "mixed" : checked}
      checked={checked}
      onChange={onToggle}
      onClick={(e) => e.stopPropagation()}
      flex="none"
      boxSize="16px"
      m="0"
      cursor="pointer"
      accentColor="var(--chakra-colors-tone-running-fg)"
    />
  );
}

/** The header's Select button. It hides while the mode is on; Done lives in the bar. */
export function SelectButton() {
  const sel = useSidebarSelect();
  if (sel.state.on) return null;
  return (
    <chakra.button type="button" onClick={sel.enter} h="34px" px="2" borderRadius="6px" color="tone.running.fg" fontSize="13px" cursor="pointer" _hover={{ bg: "surface.raised" }}>
      Select
    </chakra.button>
  );
}

/** The footer, or the select bar in its place while the mode is on. */
export function FooterSlot({ children }: { children: ReactNode }) {
  const sel = useSidebarSelect();
  return sel.state.on ? (
    <SelectBar sel={sel}>
      <SidebarBulkActions sel={sel} />
    </SelectBar>
  ) : (
    children
  );
}

/** The bar that replaces the footer in select mode. Action buttons go in `children`. `sticky` pins it under a page header instead. */
export function SelectBar({ sel, sticky, children }: { sel: SelectApi; sticky?: boolean; children?: ReactNode }) {
  const n = sel.state.keys.length;
  const pickable = sel.rows.filter((r) => r.visible && r.selectable);
  const all = pickable.length > 0 && pickable.every((r) => sel.state.keys.includes(r.key));
  return (
    <Box
      as="footer"
      data-part="select-bar"
      flex="none"
      display="flex"
      flexDirection="column"
      gap="2"
      px="2"
      py="10px"
      borderTopWidth="1px"
      borderColor="tone.running.fg"
      bg="surface.raised"
      {...(sticky ? { position: "sticky", top: "0", zIndex: "sticky", mt: "4", borderWidth: "1px", borderRadius: "md" } : {})}
    >
      <Box display="flex" alignItems="center" gap="1.5">
        <Box as="span" flex="1" fontSize="14px" fontWeight="600" whiteSpace="nowrap" data-testid="select-count">
          {n} selected
        </Box>
        <Button size="sm" emphasis="ghost" tone="running" onClick={all ? sel.selectNone : sel.selectAll}>
          {all ? "Select none" : "Select all"}
        </Button>
        <Button size="sm" emphasis="ghost" tone="running" onClick={sel.leave}>
          Done
        </Button>
      </Box>
      {children}
    </Box>
  );
}
