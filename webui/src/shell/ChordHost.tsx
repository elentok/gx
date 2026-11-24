import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useOnChange } from "../hooks"
import { useConnectionContext, useConnectionState } from "../connection"
import { useLatest } from "../hooks"
import { leaderKeyOf } from "../leaderKey"
import { hasLayer } from "../ui/modal/layers"
import { ChordBox } from "./ChordBox"
import { openHistory } from "../history/historyOpener"
import { chordTable, letterOf, matchesKey, type Chord } from "./chords"
import { useOpenEntityActions } from "./useOpenEntityActions"

const MODIFIER_KEYS = new Set(["Control", "Meta", "Alt", "Shift"])
const BOX = '[role="menu"][aria-label="Chords"]'

/**
 * Owns the Chord box: one capture-phase keydown listener opens it on the Leader key and, while it
 * is open, takes every key so no field or other window listener sees it. There is no timer.
 * `chords` replaces the real table (tests pass their own).
 */
export function ChordHost({ chords }: { chords?: Chord[] }) {
  const { snapshot } = useConnectionState(useConnectionContext())
  const leader = leaderKeyOf(snapshot)
  const { path: pathname, session, actions } = useOpenEntityActions()
  const [open, setOpen] = useState(false)
  const ctx = useMemo(() => ({ path: pathname, session }), [pathname, session])

  // The listener is mounted once and reads everything at event time. Every chord is the registry's
  // action for the open entity.
  const live = useLatest({ leader, ctx })
  const openRef = useRef(false)
  const close = useCallback(() => {
    openRef.current = false
    setOpen(false)
  }, [])

  const table =
    chords ??
    chordTable({
      actions,
      history: () => {
        openHistory()
      },
    })
  const tableRef = useLatest(table)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const { leader: key } = live.current
      const c = live.current.ctx
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
