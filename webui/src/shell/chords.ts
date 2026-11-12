import { canManage } from "../agents/sessionWrites"
import type { SessionInfo } from "../generated/server"
import { isMac } from "../platform"
import { openNewAgent, openNewDesign } from "./newItemOpen"

type Modifier = "cmd" | "ctrl" | "alt" | "shift"
const MODIFIERS: readonly Modifier[] = ["cmd", "ctrl", "alt", "shift"]

export type ParsedKey = { mods: ReadonlySet<Modifier>; code: string; char: string }

/** Parses a Leader key setting such as "cmd+e". Null for an empty or malformed setting. */
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

/** The letter of a chord key from `code`, since Mac Option rewrites `key` ("Dead"). */
export function letterOf(e: Pick<KeyboardEvent, "code">): string | null {
  const m = /^Key([A-Z])$/.exec(e.code)
  return m ? m[1].toLowerCase() : null
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

export type Chord = {
  key: string
  label: (ctx: ChordContext) => string
  run: () => void
  danger?: boolean
  available: (ctx: ChordContext) => boolean
}

/** The session id of an `/agents/:id` page, or undefined for any other path. */
export const agentIdOf = (path: string): string | undefined =>
  /^\/agents\/([^/]+)\/?$/.exec(path)?.[1]

const canRemove = (ctx: ChordContext) =>
  agentIdOf(ctx.path) !== undefined && ctx.session !== undefined && canManage(ctx.session)

/** The chord table. `remove` is the Remove opener, handed in so this module does not depend on it. */
export function chordTable(remove: () => void): Chord[] {
  return [
    { key: "c", label: () => "New chat", run: openNewAgent, available: () => true },
    { key: "d", label: () => "New design", run: openNewDesign, available: () => true },
    {
      key: "x",
      label: ({ session: s }) => (s && (s.ticket || s.fix_of || s.flow) ? "Remove" : "Remove chat"),
      run: remove,
      danger: true,
      available: canRemove,
    },
  ]
}
