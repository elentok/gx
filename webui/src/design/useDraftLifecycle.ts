import { useEffect } from "react"
import { useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import { EventDesignDraftsChanged } from "../generated/server"
import { useLatest } from "../hooks"
import { designHref } from "../hrefs"
import type { DraftsClient } from "./drafts"
import type { DraftAutosave } from "./useDraftAutosave"

/** The note the Epics page shows after a draft vanished. */
export const REMOVED_NOTE = "This draft was removed."
const LEASE_MS = 30_000

const designPath = (address: string) => designHref(address).slice(1)

/** Leaves a gone draft's page: for where it was started as (`startedPath` turns its id into a route; the Design's by default), else for the Epics page with a note. */
export function useLeaveGone(
  startedPath: (started: string) => string = designPath,
  startedState?: Record<string, unknown>,
) {
  const navigate = useNavigate()
  return (started?: string) =>
    started
      ? navigate(startedPath(started), { replace: true, state: startedState })
      : navigate("/epics", { replace: true, state: { note: REMOVED_NOTE } })
}

/**
 * The open draft page's link to the server: it renews the lease on open and every 30 s, and
 * re-reads the draft when a ping fails, the draft rows change or a save finds it gone.
 */
export function useDraftLifecycle({
  drafts,
  autosave,
}: {
  drafts: DraftsClient
  autosave: DraftAutosave
}) {
  const conn = useConnectionContext()
  const leave = useLeaveGone()
  const { currentId, stopSaving, mayRedirect, adoptTarget, gone } = autosave
  const reread = async () => {
    if (!mayRedirect()) return
    const res = await drafts.read(currentId()).catch(() => undefined)
    if (!mayRedirect()) return
    if (res?.kind === "ok") adoptTarget(res.draft)
    else if (res?.kind === "gone") {
      stopSaving()
      leave(res.started)
    }
  }
  // The client and connection are fixed for the page's life, so the open effect reads them through a ref.
  const latest = useLatest({ reread, ping: () => drafts.ping(currentId()), conn })

  useEffect(() => {
    const check = () => void latest.current.reread()
    const ping = () => latest.current.ping().catch(check)
    ping()
    const timer = setInterval(ping, LEASE_MS)
    const off = latest.current.conn.onEvent((_address, type) => {
      if (type === EventDesignDraftsChanged) check()
    })
    return () => {
      clearInterval(timer)
      off()
    }
  }, [])

  useEffect(() => {
    if (gone) void latest.current.reread()
  }, [gone])
}
