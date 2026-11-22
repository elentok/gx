import { useEffect, useState, type RefObject } from "react"
import {
  ReasonDraftGone,
  ReasonDraftRevisionConflict,
  ReasonHistoryEntryGone,
} from "../generated/ops"
import type { DraftHistoryEntry } from "../generated/ops"
import { useLatest } from "../hooks"
import { registerHistoryOpener } from "./historyOpener"

type PickResult<D> = { refused?: boolean; reason?: string; draft?: D; notices: string[] }

/** Registers `open` as the chord's History channel while mounted. `enabled` false registers nothing. */
export function useHistoryOpener(open: () => void, enabled = true) {
  const latest = useLatest(open)
  useEffect(() => {
    if (!enabled) return
    return registerHistoryOpener(() => latest.current())
  }, [enabled])
}

/**
 * Runs a history pick behind the saves in flight, with the revision the next save would send.
 * A success goes to `apply`, a conflict to `onConflict` and a gone draft to `onGone`. Other
 * refusals change nothing.
 */
export function usePickHistory<D, R extends PickResult<D>>({
  timer,
  chain,
  phase,
  revision,
  apply,
  onConflict,
  onGone,
}: {
  timer: RefObject<ReturnType<typeof setTimeout> | undefined>
  chain: RefObject<Promise<void>>
  phase: RefObject<string>
  revision: () => number
  apply: (draft: D) => void
  onConflict: () => void
  onGone: () => void
}) {
  return (call: (revision: number) => Promise<R>) => {
    clearTimeout(timer.current)
    const run = chain.current.then(async (): Promise<R | undefined> => {
      if (phase.current !== "editing") return
      const res = await call(revision())
      if (res.refused || !res.draft) {
        if (res.reason === ReasonDraftRevisionConflict) onConflict()
        else if (res.reason === ReasonDraftGone) onGone()
        return res
      }
      apply(res.draft)
      return res
    })
    chain.current = run.then(
      () => {},
      () => {},
    )
    return run
  }
}

/**
 * The page side of a restore: the popup's open state, the notices a pick left and the restore
 * handler. `pick` runs the autosave's pick for the entry; `onPicked` and `focus` run after a success.
 */
export function useDraftHistory<D>({
  pick,
  onPicked,
  focus,
}: {
  pick: (entry: DraftHistoryEntry) => Promise<PickResult<D> | undefined>
  onPicked?: (draft: D | undefined) => void
  focus: () => void
}) {
  const [open, setOpen] = useState(false)
  const [notices, setNotices] = useState<string[]>([])
  const show = () => setOpen(true)
  useHistoryOpener(show)
  const restore = async (entry: DraftHistoryEntry) => {
    const res = await pick(entry)
    if (!res) return
    if (!res.refused) {
      onPicked?.(res.draft)
      setNotices(res.notices)
    }
    if (res.refused && res.reason === ReasonHistoryEntryGone) return
    setOpen(false)
    if (!res.refused) focus()
  }
  return { open, show, hide: () => setOpen(false), notices, restore }
}
