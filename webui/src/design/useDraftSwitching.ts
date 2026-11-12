import type { DesignDraft } from "../generated/server"
import type { DraftsClient } from "./NewDesignForm"
import { openLatest } from "./drafts"
import type { DraftAutosave } from "./useDraftAutosave"

/** What the form does to leave its draft for another; `leaving` is the id of the draft it was on. */
export type SwitchDraft = (next: DesignDraft | null, leaving?: string) => void

/** The draft switcher's actions: pick, new, delete and the list it shows. */
export function useDraftSwitching({
  drafts,
  autosave,
  onSwitch,
  focusSlug,
}: {
  drafts: DraftsClient
  autosave: DraftAutosave
  onSwitch: SwitchDraft
  focusSlug: () => void
}) {
  const { server, chain, timer, phase, blocked, hasContent, flush, stopSaving } = autosave.internals

  /** Saves the open draft (a blank one is deleted instead), then opens `next` and leaves for it. */
  const leaveFor = async (next: () => Promise<DesignDraft | null | undefined>) => {
    clearTimeout(timer.current)
    if (hasContent()) {
      flush()
      await chain.current
    } else {
      // A save in flight may be the one that gives the blank draft its id.
      await chain.current
      if (server.current.id) await drafts.remove(server.current.id).catch(() => {})
    }
    const target = await next()
    if (target === undefined) return
    phase.current = "finished"
    onSwitch(target, server.current.id)
  }

  const switchDraft = (id: string) =>
    leaveFor(async () => {
      const res = await drafts.read(id)
      return res.kind === "ok" ? res.draft : undefined
    })
  const newDraft = () => (hasContent() ? leaveFor(async () => null) : focusSlug())
  /** Deletes a draft; deleting the open one opens the next most recent, or a blank one. */
  const deleteOther = async (id: string) => {
    if (id !== server.current.id) {
      await drafts.remove(id)
      return
    }
    stopSaving()
    await chain.current
    await drafts.remove(id)
    onSwitch(await openLatest(drafts))
  }
  const loadDrafts = async () => {
    clearTimeout(timer.current)
    if (!blocked.current) flush()
    await chain.current
    return drafts.list()
  }

  return { switchDraft, newDraft, deleteOther, loadDrafts }
}
