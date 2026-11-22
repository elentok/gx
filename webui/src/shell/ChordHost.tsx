import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useOnChange } from "../hooks"
import { useNavigate } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import { leaderKeyOf } from "../leaderKey"
import { hasLayer } from "../ui/modal/layers"
import { ChordBox } from "./ChordBox"
import { hasHistoryOpener, openHistory as openComposerHistory } from "../history/historyOpener"
import { chatDraftPageActions, draftPageActions } from "./draftPageActions"
import {
  chatDraftIdOf,
  chordTable,
  draftIdOf,
  letterOf,
  matchesKey,
  useOpenChat,
  type Chord,
} from "./chords"
import { openModelPicker } from "./modelPickerOpen"
import { openRemoveConfirm } from "./removeConfirmOpen"
import { openRename } from "./renameOpen"

const MODIFIER_KEYS = new Set(["Control", "Meta", "Alt", "Shift"])
const BOX = '[role="menu"][aria-label="Chords"]'

/**
 * Owns the Chord box: one capture-phase keydown listener opens it on the Leader key and, while it
 * is open, takes every key so no field or other window listener sees it. There is no timer.
 * `chords` replaces the real table (tests pass their own).
 */
export function ChordHost({ chords }: { chords?: Chord[] }) {
  const navigate = useNavigate()
  const { snapshot } = useConnectionState(useConnectionContext())
  const leader = leaderKeyOf(snapshot)
  const { path: pathname, session } = useOpenChat()
  const [open, setOpen] = useState(false)
  const baseCtx = useMemo(() => ({ path: pathname, session }), [pathname, session])
  // Read the opener registration when the box renders open; the key handler reads it at event time.
  const ctx = open ? { ...baseCtx, historyReady: hasHistoryOpener() } : baseCtx

  // The listener is mounted once and reads everything at event time.
  const live = useRef({ leader, ctx, session, navigate })
  live.current = { leader, ctx, session, navigate }
  const openRef = useRef(false)
  const close = useCallback(() => {
    openRef.current = false
    setOpen(false)
  }, [])

  const table = useMemo(
    () =>
      chords ??
      chordTable({
        remove: () => {
          const { session: s, navigate: nav, ctx: c } = live.current
          if (draftIdOf(c.path)) draftPageActions()?.remove()
          else if (s) openRemoveConfirm({ session: s, onRemoved: () => nav("/agents") })
        },
        rename: () => {
          const { session: s, ctx: c } = live.current
          if (draftIdOf(c.path)) draftPageActions()?.rename()
          else if (s) openRename({ session: s })
        },
        switchModel: () => {
          const { session: s } = live.current
          if (s) openModelPicker({ session: s })
        },
        history: () => {
          const { path } = live.current.ctx
          if (draftIdOf(path)) draftPageActions()?.openHistory()
          else if (chatDraftIdOf(path)) chatDraftPageActions()?.openHistory()
          else openComposerHistory()
        },
      }),
    [chords],
  )
  const tableRef = useRef(table)
  tableRef.current = table

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const { leader: key } = live.current
      const c = { ...live.current.ctx, historyReady: hasHistoryOpener() }
      if (!openRef.current) {
        if (!matchesKey(e, key) || hasLayer()) return
        e.preventDefault()
        e.stopImmediatePropagation()
        openRef.current = true
        setOpen(true)
        return
      }
      // Cmd+K sends a bare Cmd keydown first. Closing on it would let the K reach the Jump menu.
      if (MODIFIER_KEYS.has(e.key)) return
      e.preventDefault()
      e.stopImmediatePropagation()
      // Shift alone makes the capital: Leader c and Leader C are two chords.
      const plain = letterOf(e)
      const letter =
        e.metaKey || e.ctrlKey || e.altKey
          ? null
          : e.shiftKey
            ? (plain?.toUpperCase() ?? null)
            : plain
      const hit = letter
        ? tableRef.current.find((ch) => ch.key === letter && ch.available(c))
        : undefined
      close()
      hit?.run()
    }
    const onPointer = (e: Event) => {
      if (openRef.current && !(e.target as Element | null)?.closest?.(BOX)) close()
    }
    const onBlur = () => openRef.current && close()
    window.addEventListener("keydown", onKey, true)
    document.addEventListener("pointerdown", onPointer, true)
    window.addEventListener("blur", onBlur)
    return () => {
      window.removeEventListener("keydown", onKey, true)
      document.removeEventListener("pointerdown", onPointer, true)
      window.removeEventListener("blur", onBlur)
    }
  }, [close])

  // A page change closes the box with no action.
  useOnChange(pathname, () => {
    close()
  })

  if (!open) return null
  return <ChordBox leader={leader} ctx={ctx} chords={table} onClose={close} />
}
