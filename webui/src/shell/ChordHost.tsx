import { useEffect, useMemo, useRef, useState } from "react"
import { useLocation, useNavigate } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import { useLeaderKey } from "../leaderKey"
import { hasLayer } from "../ui/modal/layers"
import { ChordBox } from "./ChordBox"
import { chordTable, letterOf, matchesKey, type Chord } from "./chords"
import { openRemoveConfirm } from "./removeConfirmOpen"

const MODIFIER_KEYS = new Set(["Control", "Meta", "Alt", "Shift"])
const BOX = '[role="menu"][aria-label="Chords"]'

/**
 * Owns the Chord box: one capture-phase keydown listener opens it on the Leader key and, while it
 * is open, takes every key so no field or other window listener sees it. There is no timer.
 * `chords` replaces the real table (tests pass their own).
 */
export function ChordHost({ chords }: { chords?: Chord[] }) {
  const leader = useLeaderKey()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { snapshot } = useConnectionState(useConnectionContext())
  const sessionId = /^\/agents\/([^/]+)\/?$/.exec(pathname)?.[1]
  const session = sessionId ? snapshot?.sessions?.find((s) => s.id === sessionId) : undefined
  const ctx = useMemo(() => ({ path: pathname, session }), [pathname, session])

  const [open, setOpen] = useState(false)
  // The listener is mounted once and reads everything at event time.
  const live = useRef({ leader, ctx, session, navigate })
  live.current = { leader, ctx, session, navigate }
  const openRef = useRef(false)
  const close = () => {
    openRef.current = false
    setOpen(false)
  }

  const table = useMemo(
    () =>
      chords ??
      chordTable(() => {
        const { session: s, navigate: nav } = live.current
        if (s) openRemoveConfirm({ session: s, onRemoved: () => nav("/agents") })
      }),
    [chords],
  )
  const tableRef = useRef(table)
  tableRef.current = table

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const { leader: key, ctx: c } = live.current
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
      const letter = e.metaKey || e.ctrlKey || e.altKey || e.shiftKey ? null : letterOf(e)
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
  }, [])

  // A page change closes the box with no action.
  useEffect(() => {
    openRef.current = false
    setOpen(false)
  }, [pathname])

  if (!open) return null
  return <ChordBox leader={leader} ctx={ctx} chords={table} onClose={close} />
}
