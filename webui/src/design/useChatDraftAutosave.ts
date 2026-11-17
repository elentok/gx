import { useEffect, useRef, useState } from "react"
import { useConnectionContext } from "../connection"
import { EventChatDraftsChanged, type ChatDraft } from "../generated/server"
import type { ChatDraftsClient } from "./chatDrafts"

const AUTOSAVE_MS = 800
const RETRY_MS = 5000

export type ChatSaveStatus = "none" | "saving" | "saved" | "unsaved"

/** `editing` saves; `finished` is Remove, or a gone draft, having taken the draft over. */
type Phase = "editing" | "finished"

/**
 * The open chat draft's brief and its autosave: 800 ms after typing it saves with the draft's
 * revision, a stale revision raises the conflict banner, a failed save retries every 5 s, and
 * leaving the page flushes. `onGone` runs when the draft vanished (started or removed elsewhere).
 */
export function useChatDraftAutosave({
  drafts,
  initial,
  onGone,
}: {
  drafts: ChatDraftsClient
  initial: ChatDraft
  onGone: (started?: string) => void
}) {
  const [facts, setFacts] = useState(initial)
  const [brief, setBrief] = useState(initial.brief)
  const [status, setStatus] = useState<ChatSaveStatus>("saved")
  const [banner, setBanner] = useState(false)

  const latest = useRef(brief)
  // What the server last had, so an unchanged brief saves nothing.
  const server = useRef({ revision: initial.revision, text: initial.brief })
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const chain = useRef<Promise<void>>(Promise.resolve())
  const phase = useRef<Phase>("editing")
  // The banner's ref twin, so a flush in flight sees it before a render.
  const blocked = useRef(false)
  const goneNow = useRef(onGone)
  goneNow.current = onGone

  const leaveGone = (started?: string) => {
    phase.current = "finished"
    clearTimeout(timer.current)
    goneNow.current(started)
  }

  /** Saves the brief; false when it did not settle. */
  const persist = async () => {
    if (phase.current !== "editing") return
    const s = server.current
    const text = latest.current
    if (text === s.text) {
      setStatus("saved")
      return
    }
    let res
    try {
      res = await drafts.save({ id: initial.id, brief: text, revision: s.revision })
    } catch {
      setStatus("unsaved")
      return
    }
    if (phase.current !== "editing") return
    if (res.kind === "ok") {
      Object.assign(s, { revision: res.draft.revision, text })
      setFacts(res.draft)
      setStatus(latest.current === text ? "saved" : "saving")
    } else if (res.kind === "conflict") {
      blocked.current = true
      setBanner(true)
      setStatus("none")
    } else if (res.kind === "gone") leaveGone(res.started)
  }

  const flush = () => {
    clearTimeout(timer.current)
    if (phase.current === "finished" || blocked.current) return
    chain.current = chain.current.then(persist).catch(() => setStatus("unsaved"))
  }

  /** The server's copy, or undefined once the draft is gone (the page then leaves). */
  const readServer = async () => {
    const res = await drafts.read(initial.id)
    if (res.kind === "ok") return res.draft
    if (res.kind === "gone") leaveGone(res.started)
  }
  const resume = () => {
    blocked.current = false
    setBanner(false)
    flush()
  }
  const bannerActions = {
    keepMine: async () => {
      const theirs = await readServer().catch(() => undefined)
      if (!theirs) return
      server.current.revision = theirs.revision
      resume()
    },
    loadTheirs: async () => {
      const theirs = await readServer().catch(() => undefined)
      if (!theirs) return
      Object.assign(server.current, { revision: theirs.revision, text: theirs.brief })
      latest.current = theirs.brief
      setBrief(theirs.brief)
      setFacts(theirs)
      setStatus("saved")
      resume()
    },
  }

  // The sidebar row and the facts follow a change elsewhere; a gone draft sends the page away.
  const conn = useConnectionContext()
  useEffect(() => {
    const off = conn.onEvent((_address, type) => {
      if (type !== EventChatDraftsChanged || phase.current === "finished") return
      drafts
        .read(initial.id)
        .then((res) => {
          if (phase.current === "finished") return
          if (res.kind === "ok") setFacts((was) => ({ ...res.draft, brief: was.brief }))
          else if (res.kind === "gone") leaveGone(res.started)
        })
        .catch(() => {})
    })
    return () => {
      off()
    }
    // The client and connection are fixed for the page's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Only refs are read below, so `flush` is safe to call from a stale closure.
  useEffect(() => {
    const leave = () => flush()
    window.addEventListener("pagehide", leave)
    return () => {
      window.removeEventListener("pagehide", leave)
      leave()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (status !== "unsaved") return
    const id = setInterval(flush, RETRY_MS)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status])

  const edit = (next: string) => {
    setBrief(next)
    latest.current = next
    if (!blocked.current && next !== server.current.text) setStatus("saving")
    clearTimeout(timer.current)
    timer.current = setTimeout(flush, AUTOSAVE_MS)
  }

  /** Removes the draft: stops saving and deletes it once a save in flight is done. */
  const remove = () => {
    phase.current = "finished"
    clearTimeout(timer.current)
    chain.current = chain.current
      .then(async () => {
        await drafts.remove(initial.id)
      })
      .catch(() => {})
  }

  const [starting, setStarting] = useState(false)
  const [startError, setStartError] = useState(initial.start_error)

  /** Starts the chat: `onStarted` gets its session id. A failure keeps the draft and the page. */
  const start = async (onStarted: (session: string) => void) => {
    if (phase.current !== "editing" || starting) return
    setStarting(true)
    try {
      await chain.current
      const res = await drafts.start(initial.id)
      if (res.kind === "started") {
        phase.current = "finished"
        clearTimeout(timer.current)
        onStarted(res.session)
        return
      }
      if (res.kind === "gone") return leaveGone(res.started)
      setStartError(res.message)
      // The failed Start may have left a worktree; the draft says where.
      const again = await drafts.read(initial.id)
      if (again.kind === "ok") setFacts((was) => ({ ...again.draft, brief: was.brief }))
      else if (again.kind === "gone") leaveGone(again.started)
    } catch (e) {
      setStartError(e instanceof Error ? e.message : "Start failed")
    } finally {
      setStarting(false)
    }
  }

  return { facts, brief, edit, status, banner, bannerActions, remove, start, starting, startError }
}
