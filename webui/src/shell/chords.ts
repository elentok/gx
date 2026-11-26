import { useLocation } from "react-router"
import type { ReadyAction } from "../actions/useActions"
import { hasHistoryOpener } from "../history/historyOpener"
import { designSession } from "../design/view"
import { useConnectionContext, useConnectionState } from "../connection"
import type { SessionInfo } from "../generated/server"
import { isMac } from "../platform"
import { getBigPrompt, toggleBigPrompt } from "../ui/expandable-text/bigPromptStore"
import { focusSidebar } from "./focusSidebar"
import { startNewChatDraft, startNewDesign } from "../design/newDesign"
import { openNewAgent } from "./newItemOpen"

type Modifier = "cmd" | "ctrl" | "alt" | "shift"
const MODIFIERS: readonly Modifier[] = ["cmd", "ctrl", "alt", "shift"]

export type ParsedKey = { mods: ReadonlySet<Modifier>; code: string; char: string }

/**
 * Parses a Leader key setting such as "cmd+e". Null for an empty or malformed setting.
 * checkLeaderKey in config/registry/check.go is the other copy of this grammar and must change
 * with it. The parseKey table in chords.test.ts mirrors TestLeaderKeyEntries.
 */
export function parseKey(setting: string | undefined): ParsedKey | null {
  if (!setting) return null
  const parts = setting.trim().toLowerCase().split("+")
  const char = parts.pop() ?? ""
  if (!/^[a-z0-9]$/.test(char)) return null
  const mods = new Set<Modifier>()
  for (const p of parts) {
    if (!MODIFIERS.includes(p as Modifier)) return null
    mods.add(p as Modifier)
  }
  return { mods, char, code: /\d/.test(char) ? `Digit${char}` : `Key${char.toUpperCase()}` }
}

const PUNCTUATION: Record<string, string> = { Comma: ",", Period: "." }

/**
 * The chord key of an event from `code`, since a layout or Mac Option rewrites `key`. Shift makes
 * the capital letter, and nothing of a punctuation key.
 */
export function chordKeyOf(e: Pick<KeyboardEvent, "code" | "shiftKey">): string | null {
  const letter = /^Key([A-Z])$/.exec(e.code)?.[1]
  if (letter) return e.shiftKey ? letter : letter.toLowerCase()
  return e.shiftKey ? null : (PUNCTUATION[e.code] ?? null)
}

/** True when the event is exactly the setting's key and modifiers. Matches on `code`, never `key`. */
export function matchesKey(
  e: Pick<KeyboardEvent, "code" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
  setting: string | undefined,
): boolean {
  const k = parseKey(setting)
  if (!k) return false
  return (
    e.code === k.code &&
    e.metaKey === k.mods.has("cmd") &&
    e.ctrlKey === k.mods.has("ctrl") &&
    e.altKey === k.mods.has("alt") &&
    e.shiftKey === k.mods.has("shift")
  )
}

const MAC_SYMBOLS: Record<Modifier, string> = { ctrl: "⌃", alt: "⌥", shift: "⇧", cmd: "⌘" }
const MAC_ORDER: readonly Modifier[] = ["ctrl", "alt", "shift", "cmd"]
const WORDS: Record<Modifier, string> = { cmd: "Cmd", ctrl: "Ctrl", alt: "Alt", shift: "Shift" }

/** The Leader key for the Chord box chip: "⌘E" on a Mac, "Ctrl+E" elsewhere. Empty for an unparsable setting. */
export function displayKey(setting: string | undefined): string {
  const k = parseKey(setting)
  if (!k) return ""
  const char = k.char.toUpperCase()
  if (isMac()) {
    return (
      MAC_ORDER.filter((m) => k.mods.has(m))
        .map((m) => MAC_SYMBOLS[m])
        .join("") + char
    )
  }
  const mods = MODIFIERS.filter((m) => k.mods.has(m))
  return [...mods.map((m) => WORDS[m]), char].join("+")
}

export type ChordContext = { path: string; session?: SessionInfo }

/** "entity" chords act on the open page's entity and come from the registry; "app" chords work anywhere. */
export type ChordGroup = "entity" | "app"

export type Chord = {
  key: string
  label: (ctx: ChordContext) => string
  run: () => void
  danger?: boolean
  available: (ctx: ChordContext) => boolean
  /** Absent means "app". */
  group?: ChordGroup
}

/** The session id of an `/agents/:id` page, or undefined for any other path. */
export const agentIdOf = (path: string): string | undefined =>
  /^\/agents\/([^/]+)\/?$/.exec(path)?.[1]

/** The id of a `/drafts/:id` page, or undefined for any other path. */
export const draftIdOf = (path: string): string | undefined =>
  /^\/drafts\/([^/]+)\/?$/.exec(path)?.[1]

/** The id of a `/chat-drafts/:id` page, or undefined for any other path. */
export const chatDraftIdOf = (path: string): string | undefined =>
  /^\/chat-drafts\/([^/]+)\/?$/.exec(path)?.[1]

/** A draft page or a composer that offers History has registered its opener. */
const canHistory = () => hasHistoryOpener()

/** The path and the session of the open `/agents/:id` page. `session` is undefined off that page. */
export function useOpenChat(): { path: string; session: SessionInfo | undefined } {
  const { pathname } = useLocation()
  const { snapshot } = useConnectionState(useConnectionContext())
  const id = agentIdOf(pathname)
  // A Design's address holds a colon, so it is the whole path after `/d/`.
  const design = /^\/d\/(.+)$/.exec(pathname)?.[1]
  const sessions = snapshot?.sessions ?? []
  const session = id
    ? sessions.find((s) => s.id === id)
    : design
      ? designSession(design, sessions)
      : undefined
  return { path: pathname, session }
}

/** What an entity chord needs of a registry action; `useActions` gives the real ones. */
export type EntityAction = Pick<
  ReadyAction,
  "id" | "label" | "chord" | "danger" | "disabled" | "run"
>

/** One chord per registry action that has a letter and can run now, in registry order. */
export function entityChords(actions: EntityAction[]): Chord[] {
  return actions.flatMap((a) =>
    a.chord && !a.disabled
      ? [
          {
            key: a.chord,
            label: () => a.label,
            run: a.run,
            danger: a.danger,
            available: () => true,
            group: "entity" as const,
          },
        ]
      : [],
  )
}

/** The browser's session history, as far as the Back and Forward chords need it. */
export type HistoryNav = {
  canBack: boolean
  canForward: boolean
  back: () => void
  forward: () => void
}

/** The steps along the sidebar. A step is absent when it has no target. */
export type ItemNav = {
  next?: () => void
  prev?: () => void
}

/**
 * The chord table: the open entity's registry chords, then the app-level ones. `history` opens the
 * History popup, handed in so this module does not depend on it. `nav` is absent in a browser that
 * cannot report its history, and then there are no Back and Forward chords. Without `items` there
 * are no Next item and Previous item chords.
 */
export function chordTable({
  actions,
  history,
  nav,
  items,
}: {
  actions: EntityAction[]
  history: () => void
  nav?: HistoryNav
  items?: ItemNav
}): Chord[] {
  return [
    ...entityChords(actions),
    ...appChords(history),
    ...(nav ? navChords(nav) : []),
    ...(items ? itemChords(items) : []),
  ]
}

function itemChords({ next, prev }: ItemNav): Chord[] {
  return [
    { key: "j", label: () => "Next item", run: () => next?.(), available: () => !!next },
    { key: "k", label: () => "Previous item", run: () => prev?.(), available: () => !!prev },
  ]
}

function navChords(nav: HistoryNav): Chord[] {
  return [
    { key: ",", label: () => "Back", run: nav.back, available: () => nav.canBack },
    { key: ".", label: () => "Forward", run: nav.forward, available: () => nav.canForward },
  ]
}

function appChords(history: () => void): Chord[] {
  return [
    {
      key: "c",
      label: () => "New chat draft",
      run: () => void startNewChatDraft(),
      available: () => true,
    },
    { key: "C", label: () => "New chat", run: openNewAgent, available: () => true },
    {
      key: "d",
      label: () => "New design",
      run: () => void startNewDesign(),
      available: () => true,
    },
    { key: "s", label: () => "Focus sidebar", run: focusSidebar, available: () => true },
    // Global on purpose: the big prompt box toggle works on every page (see GLOSSARY, prompt-box-vi-mode spec).
    {
      key: "b",
      label: () => (getBigPrompt() ? "Normal prompt box" : "Big prompt box"),
      run: toggleBigPrompt,
      available: () => true,
    },
    { key: "h", label: () => "History", run: history, available: canHistory },
  ]
}
