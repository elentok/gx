import { Box, chakra } from "@chakra-ui/react"
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import type { FetchEpicList } from "../epics/list"
import { Button } from "../ui/button/Button"
import { hasLayer } from "../ui/modal/layers"
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
} from "../select/selectModel"
import { dispatchSelectKey, selectIntent } from "../select/selectKeys"
import type { SidebarRow } from "../workspaces/sidebarModel"
import { showToast } from "../ui/toaster"
import { isTextTarget } from "./keys"
import { NAV_ATTR } from "./SidebarKeys"
import { useSidebarModel } from "./SidebarRows"
import { SidebarBulkActions } from "./SidebarBulkActions"

/** One select-mode instance: the sidebar has one, the Epics page has its own. */
export interface SelectApi {
  state: SelectState
  rows: SelectRow[]
  enter: () => void
  leave: () => void
  toggle: (key: string) => void
  toggleGroup: (group: string) => void
  groupState: (group: string) => GroupState
  selectAll: () => void
  selectNone: () => void
  /** A row's display name, for tooltips and toasts; the key when unknown. */
  nameOf: (key: string) => string
  /** Call after an action ran: leaves select mode when nothing is selected any more. */
  afterRun: () => void
}

export interface SidebarSelect extends SelectApi {
  /** Whether a section ("on-hold" or "other") is expanded; a collapsed section's rows are not visible to Select all. */
  sectionOpen: Record<string, boolean>
  setSectionOpen: (group: string, open: boolean) => void
}

const unfocused = (t: EventTarget | null) => t === document.body || t === document.documentElement
const inSidebar = (t: EventTarget | null) => t instanceof Element && !!t.closest("aside")

/** Escape belongs to the sidebar when focus is in it or nowhere. */
const sidebarOwnsKey = (t: EventTarget | null) => unfocused(t) || inSidebar(t)
/** Escape belongs to a page when focus is in the page, outside the sidebar. */
export const pageOwnsKey = (t: EventTarget | null) => !unfocused(t) && !inSidebar(t)

/**
 * The select model over `rows`, with Escape to leave when `ownsKey` accepts the focused element (each area
 * owns its own Escape). An unloaded list is not an empty one, so nothing is pruned until `loaded`.
 */
export function useSelectApi(
  rows: SelectRow[],
  names: Map<string, string>,
  loaded: boolean,
  ownsKey: (t: EventTarget | null) => boolean = pageOwnsKey,
): SelectApi {
  const [state, setState] = useState<SelectState>(initialSelect)
  const stateRef = useRef(state)
  stateRef.current = state
  // A run's moved rows leave the list only on a later refetch, after afterRun ran on the old rows.
  // The flag stays up until a prune changes something, so an unrelated list update in between does not eat it.
  const runPending = useRef(false)
  // A row that leaves the list leaves the selection.
  useEffect(() => {
    if (!loaded) return
    const next = prune(stateRef.current, rows, runPending.current)
    if (next === stateRef.current) return
    runPending.current = false
    setState(next)
  }, [loaded, rows])
  const ownsKeyRef = useRef(ownsKey)
  ownsKeyRef.current = ownsKey
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || !ownsKeyRef.current(e.target)) return
      if (selectIntent(e, stateRef.current.on, hasLayer() || isTextTarget(e.target)) === "leave")
        setState(leave)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [])

  return useMemo<SelectApi>(
    () => ({
      state,
      rows,
      enter: () => {
        runPending.current = false
        setState(enter)
      },
      leave: () => {
        runPending.current = false
        setState(leave)
      },
      toggle: (key) => setState((s) => toggle(s, key)),
      toggleGroup: (group) => setState((s) => toggleGroup(s, rows, group)),
      groupState: (group) => groupState(state, rows, group),
      selectAll: () => setState((s) => selectAll(s, rows)),
      selectNone: () => setState(selectNone),
      nameOf: (key) => names.get(key) ?? key,
      afterRun: () => {
        runPending.current = true
        setState((s) => prune(s, rows, true))
      },
    }),
    [state, rows, names],
  )
}

const SidebarSelectContext = createContext<SidebarSelect | null>(null)

/** The sidebar's select state. The sidebar rows only render inside the provider. */
export function useSidebarSelect(): SidebarSelect {
  const sel = useContext(SidebarSelectContext)
  if (!sel) throw new Error("useSidebarSelect needs a SidebarSelectProvider")
  return sel
}

const QUEUE_REASON = "Managed by the queue: it can't be selected."

/**
 * `s`, Space and Enter while a sidebar row has focus. Runs in the capture phase so that in select
 * mode Enter toggles instead of reaching the row's open handler. Escape is handled by useSelectApi.
 */
function useSidebarSelectKeys(sel: SidebarSelect) {
  const ref = useRef(sel)
  ref.current = sel
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const stop =
        e.target instanceof Element ? e.target.closest<HTMLElement>(`[${NAV_ATTR}]`) : null
      // With nothing focused the key lands on the body: `s` then enters the sidebar's select mode.
      if (e.defaultPrevented || !sidebarOwnsKey(e.target)) return
      const { state, rows, enter, toggle } = ref.current
      const nav = stop?.getAttribute(NAV_ATTR) ?? ""
      const key = nav.startsWith("row:")
        ? nav.slice(4)
        : nav.startsWith("agent:")
          ? nav.slice(6)
          : ""
      dispatchSelectKey(e, state.on, hasLayer() || isTextTarget(e.target), {
        enter,
        itemKey: key || null,
        toggle: (k) => {
          if (rows.find((r) => r.key === k)?.selectable === false)
            showToast({ type: "info", title: QUEUE_REASON, id: "select-queue-agent" })
          else toggle(k)
        },
      })
    }
    document.addEventListener("keydown", onKey, true)
    return () => document.removeEventListener("keydown", onKey, true)
  }, [])
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
})

const OTHER_OPEN_KEY = "gx.sidebar.other-open"

// Storage is a convenience; the sidebar works without it.
function readOtherOpen(): boolean {
  try {
    return localStorage.getItem(OTHER_OPEN_KEY) === "1"
  } catch {
    return false
  }
}

function writeOtherOpen(open: boolean) {
  try {
    localStorage.setItem(OTHER_OPEN_KEY, open ? "1" : "0")
  } catch {
    // See above.
  }
}

/** Holds one select model for the whole sidebar, above the rows and the footer, so the selection survives navigation. */
export function SidebarSelectProvider({
  fetcher,
  active,
  children,
}: {
  fetcher?: FetchEpicList
  active: string
  children: ReactNode
}) {
  const model = useSidebarModel(fetcher, active)
  const [sectionOpen, setOpen] = useState<Record<string, boolean>>(() => ({
    [GROUP_OTHER]: readOtherOpen(),
  }))
  const setSectionOpen = useCallback((group: string, open: boolean) => {
    setOpen((o) => ({ ...o, [group]: open }))
    if (group === GROUP_OTHER) writeOtherOpen(open)
  }, [])

  // Agents nested under a row are not visible to Select all: their parent's expanded state is local to the row.
  const rows = useMemo<SelectRow[]>(() => {
    if (!model) return []
    const out: SelectRow[] = []
    for (const r of model.rows) {
      out.push(rowOf(r, true))
      for (const a of r.agents) out.push(rowOfAgent(a.session, false))
    }
    for (const r of model.held) {
      out.push(rowOf(r, !!sectionOpen[GROUP_ON_HOLD]))
      for (const a of r.agents) out.push(rowOfAgent(a.session, false))
    }
    for (const a of model.other)
      out.push(rowOfAgent(a.session, !!sectionOpen[GROUP_OTHER], GROUP_OTHER))
    return out
  }, [model, sectionOpen])

  const names = useMemo(() => {
    const out = new Map<string, string>()
    if (!model) return out
    const add = (name: string, key: string) => out.set(key, name)
    for (const r of [...model.rows, ...model.held]) {
      add(r.title, r.key)
      for (const a of r.agents) add(a.label, a.session.id)
    }
    for (const a of model.other) add(a.label, a.session.id)
    return out
  }, [model])

  const api = useSelectApi(rows, names, !!model, sidebarOwnsKey)
  const select = useMemo<SidebarSelect>(
    () => ({ ...api, sectionOpen, setSectionOpen }),
    [api, sectionOpen, setSectionOpen],
  )
  useSidebarSelectKeys(select)

  return <SidebarSelectContext.Provider value={select}>{children}</SidebarSelectContext.Provider>
}

/** A row or section checkbox. The mixed state is a real indeterminate checkbox. */
export function SelectBox({
  checked,
  mixed,
  label,
  onToggle,
  size = "16px",
}: {
  checked: boolean
  mixed?: boolean
  label: string
  onToggle: () => void
  size?: string
}) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = !!mixed
  }, [mixed])
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
      boxSize={size}
      m="0"
      cursor="pointer"
      accentColor="var(--chakra-colors-tone-running-fg)"
    />
  )
}

/** The header's Select button. It hides while the mode is on; Done lives in the bar. */
export function SelectButton() {
  const sel = useSidebarSelect()
  if (sel.state.on) return null
  return (
    <chakra.button
      type="button"
      onClick={sel.enter}
      h="34px"
      px="2"
      borderRadius="6px"
      color="tone.running.fg"
      fontSize="13px"
      cursor="pointer"
      _hover={{ bg: "surface.raised" }}
    >
      Select
    </chakra.button>
  )
}

/** The footer, or the select bar in its place while the mode is on. */
export function FooterSlot({ children }: { children: ReactNode }) {
  const sel = useSidebarSelect()
  return sel.state.on ? (
    <SelectBar sel={sel} head={false}>
      <SidebarBulkActions sel={sel} />
    </SelectBar>
  ) : (
    children
  )
}

/** The workspace picker, or the select-mode line (count, Select all, Done) in its place while the mode is on. */
export function HeadSlot({ children }: { children: ReactNode }) {
  const sel = useSidebarSelect()
  if (!sel.state.on) return children
  return (
    <Box px="2" pb="2" h="44px" display="flex" alignItems="center">
      <SelectHead sel={sel} />
    </Box>
  )
}

/** The count, Select all/none and Done. */
function SelectHead({ sel, sticky }: { sel: SelectApi; sticky?: boolean }) {
  const n = sel.state.keys.length
  const pickable = sel.rows.filter((r) => r.visible && r.selectable)
  const all = pickable.length > 0 && pickable.every((r) => sel.state.keys.includes(r.key))
  return (
    <Box display="flex" alignItems="center" gap="1.5" flex={sticky ? "none" : "1"} minW="0">
      <Box
        as="span"
        flex={sticky ? "none" : "1"}
        mr={sticky ? "2" : undefined}
        fontSize="14px"
        fontWeight="600"
        whiteSpace="nowrap"
        data-testid="select-count"
      >
        {n} selected
      </Box>
      <Button
        size="sm"
        emphasis="ghost"
        tone="running"
        onClick={all ? sel.selectNone : sel.selectAll}
      >
        {all ? "Select none" : "Select all"}
      </Button>
      <Button size="sm" emphasis="ghost" tone="running" onClick={sel.leave}>
        Done
      </Button>
    </Box>
  )
}

/** The bar that replaces the footer in select mode. Action buttons go in `children`. `sticky` pins it under a page header instead. */
export function SelectBar({
  sel,
  sticky,
  head = true,
  children,
}: {
  sel: SelectApi
  sticky?: boolean
  /** Whether the bar shows the count line itself; the sidebar shows it in the header instead. */
  head?: boolean
  children?: ReactNode
}) {
  return (
    <Box
      as="footer"
      data-part="select-bar"
      flex="none"
      display="flex"
      flexDirection={sticky ? "row" : "column"}
      flexWrap={sticky ? "wrap" : undefined}
      alignItems={sticky ? "center" : undefined}
      gap={sticky ? "3" : "2"}
      px={sticky ? "14px" : "2"}
      py="10px"
      borderTopWidth="1px"
      borderColor="tone.running.fg"
      bg="surface.raised"
      _empty={{ display: "none" }}
      {...(sticky
        ? {
            position: "sticky",
            top: "0",
            zIndex: "sticky",
            mt: "3",
            borderWidth: "1px",
            borderRadius: "10px",
          }
        : {})}
    >
      {head && <SelectHead sel={sel} sticky={sticky} />}
      {sticky ? <Box ml="auto">{children}</Box> : children}
    </Box>
  )
}
