import type { DesignDraft } from "../generated/server"
import type { DraftsClient } from "./drafts"
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
  const { hasContent, flush, stopSaving, takeOver, idle, currentId } = autosave

  /** Saves the open draft (a blank one is deleted instead), then opens `next` and leaves for it. */
  const leaveFor = async (next: () => Promise<DesignDraft | null | undefined>) => {
    if (hasContent()) {
      flush()
      await idle()
    } else {
      // A save in flight may be the one that gives the blank draft its id.
      await idle()
      const id = currentId()
      if (id) await drafts.remove(id).catch(() => {})
    }
    const target = await next()
    if (target === undefined) return
    takeOver()
    onSwitch(target, currentId())
  }

  const switchDraft = (id: string) =>
    leaveFor(async () => {
      const res = await drafts.read(id)
      return res.kind === "ok" ? res.draft : undefined
    })
  const newDraft = () => (hasContent() ? leaveFor(async () => null) : focusSlug())
  /** Deletes a draft; deleting the open one opens the next most recent, or a blank one. */
  const deleteOther = async (id: string) => {
    if (id !== currentId()) {
      await drafts.remove(id)
      return
    }
    stopSaving()
    await idle()
    await drafts.remove(id)
    onSwitch(await openLatest(drafts))
  }
  const loadDrafts = async () => {
    flush()
    await idle()
    return drafts.list()
  }

  return { switchDraft, newDraft, deleteOther, loadDrafts }
}
