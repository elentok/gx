import { Box } from "@chakra-ui/react"
import { useCallback, useEffect, useMemo, useRef } from "react"
import { useLocation, useNavigate } from "react-router"
import { toggleAfk } from "../afk"
import { useConnectionContext, useConnectionState } from "../connection"
import { useEpicList } from "../epics/list"
import { AGENTS_HREF, EPICS_HREF, SETTINGS_HREF, STATS_HREF } from "../hrefs"
import { jumpHref, Menu, type ListSection } from "../nav/JumpMenu"
import { isMac } from "../platform"
import { useWide } from "../useWide"
import { setCommandOpen, useCommandOpen } from "./commandOpen"
import { openNewAgent, openNewDesign } from "./newItemOpen"
import { nextQuestion } from "./nextQuestion"
import { useShell } from "./ShellContext"
import { useDismissLayer } from "./useDismissLayer"

export interface Command {
  id: string
  title: string
  /** Extra words the filter matches besides the title. */
  words?: string
  subtitle?: string
  run: () => void
}

/** The one command list, in display order. Insert a row at any position. */
function useCommands(): Command[] {
  const navigate = useNavigate()
  const shell = useShell()
  const afk = useConnectionState(useConnectionContext()).snapshot?.afk ?? false
  const { list } = useEpicList()
  const { pathname } = useLocation()
  const workspace = shell?.workspace || undefined
  const { next, count } = useMemo(
    () =>
      nextQuestion(
        { epics: list?.epics ?? [], follow_ups: list?.follow_ups ?? [] },
        workspace,
        pathname,
      ),
    [list, workspace, pathname],
  )
  return useMemo(() => {
    const goto = (id: string, title: string, href: string): Command => ({
      id,
      title,
      run: () => {
        navigate(href.replace(/^#/, ""))
        shell?.closeSliding()
      },
    })
    const nextRow: Command = next
      ? {
          ...goto("next-question", "Next question", jumpHref(next)),
          words: "needs you",
          subtitle: `${next.name || next.project} · ${count} waiting`,
        }
      : {
          id: "next-question",
          title: "Next question",
          words: "needs you",
          subtitle: "Nothing needs you",
          run: () => {},
        }
    return [
      { id: "new-design", title: "New design", run: () => openNewDesign() },
      { id: "new-chat", title: "New chat", run: () => openNewAgent() },
      nextRow,
      goto("go-epics", "Go to Epics", EPICS_HREF),
      goto("go-agents", "Go to Agents", AGENTS_HREF),
      goto("go-stats", "Go to Stats", STATS_HREF),
      goto("go-settings", "Go to Settings", SETTINGS_HREF),
      {
        id: "toggle-afk",
        title: "Toggle AFK",
        subtitle: afk ? "AFK is on" : "AFK is off",
        run: () => toggleAfk(afk),
      },
    ]
  }, [navigate, shell, afk, next, count])
}

/** Cmd+P on a Mac, Ctrl+P elsewhere, on every page, in text fields too. Same layout as the jump palette. */
export function CommandPalette() {
  const phone = !useWide()
  const open = useCommandOpen()
  const root = useRef<HTMLDivElement>(null)
  const close = useCallback(() => setCommandOpen(false), [])
  const { isTop, hasLayer } = useDismissLayer({ open, close, root, phone })

  useEffect(() => () => setCommandOpen(false), [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = isMac() ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey
      if (mod && e.key.toLowerCase() === "p") {
        e.preventDefault()
        if (open ? isTop() : !hasLayer()) setCommandOpen((o) => !o)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [open, isTop, hasLayer])

  if (!open) return null
  return (
    <Box ref={root} display="contents">
      <CommandList phone={phone} onClose={close} />
    </Box>
  )
}

/** Rendered only while the palette is open, so the command list is not rebuilt on every epic change. */
function CommandList({ phone, onClose }: { phone: boolean; onClose: () => void }) {
  const commands = useCommands()
  const sections = useMemo<ListSection[]>(
    () => [
      {
        key: "commands",
        title: "Commands",
        rows: commands.map((c) => ({
          key: c.id,
          title: c.title,
          subtitle: c.subtitle ?? "",
          href: "",
          searchText: [c.title, c.words].filter(Boolean).join(" "),
          onSelect: c.run,
        })),
      },
    ],
    [commands],
  )

  return (
    <Menu
      sections={sections}
      label="Commands"
      placeholder="Type a command…"
      phone={phone}
      floating
      onClose={onClose}
    />
  )
}
