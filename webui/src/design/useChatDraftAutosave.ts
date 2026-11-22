import { useEffect, useRef, useState } from "react"
import { useConnectionContext } from "../connection"
import {
  ReasonDraftGone,
  ReasonDraftRevisionConflict,
  type ChatDraftPickResult,
} from "../generated/ops"
import { EventChatDraftsChanged, type ChatDraft } from "../generated/server"
import { useLatest } from "../hooks"
import type { ChatDraftsClient } from "./chatDrafts"

const AUTOSAVE_MS = 800
const RETRY_MS = 5000

export type ChatSaveStatus = "none" | "saving" | "saved" | "unsaved"

const SETTING_KEYS = [
  "project",
  "name",
  "harness",
  "model",
  "effort",
  "folder",
  "worktree",
  "dir",
] as const

/** The launch settings of a chat draft, saved with the brief in one revision. */
export type ChatSettings = Pick<ChatDraft, (typeof SETTING_KEYS)[number]>

const settingsOf = (d: ChatDraft): ChatSettings => ({
  project: d.project,
  name: d.name,
  harness: d.harness,
  model: d.model,
  effort: d.effort,
  folder: d.folder,
  worktree: d.worktree,
  dir: d.dir,
})

/** `editing` saves; `finished` is Remove, or a gone draft, having taken the draft over. */
type Phase = "editing" | "finished"

/**
 * The open chat draft's brief and settings and their autosave: 800 ms after an edit it saves the
 * brief and the changed settings with the draft's revision, a stale revision raises the conflict banner, a failed save retries every 5 s, and
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

  const [settings, setSettings] = useState(() => settingsOf(initial))

  const latest = useRef(brief)
  const latestSettings = useRef(settings)
  // What the server last had, so an unchanged draft saves nothing.
  const server = useRef({
    revision: initial.revision,
    text: initial.brief,
    settings: settingsOf(initial),
  })
  const changedKeys = () =>
    SETTING_KEYS.filter((k) => latestSettings.current[k] !== server.current.settings[k])
  const dirty = () => latest.current !== server.current.text || changedKeys().length > 0
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const chain = useRef<Promise<void>>(Promise.resolve())
  const phase = useRef<Phase>("editing")
  // The banner's ref twin, so a flush in flight sees it before a render.
  const blocked = useRef(false)
  const goneNow = useLatest(onGone)

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
    const sent = { ...latestSettings.current }
    const changed = changedKeys()
    if (text === s.text && changed.length === 0) {
      setStatus("saved")
      return
    }
    let res: Awaited<ReturnType<typeof drafts.save>>
    try {
      res = await drafts.save({
        id: initial.id,
        brief: text,
        revision: s.revision,
        ...Object.fromEntries(changed.map((k) => [k, sent[k]])),
      })
    } catch {
      setStatus("unsaved")
      return
    }
    if (phase.current !== "editing") return
    if (res.kind === "ok") {
      Object.assign(s, { revision: res.draft.revision, text, settings: sent })
      setFacts(res.draft)
      setStatus(dirty() ? "saving" : "saved")
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
      const theirSettings = settingsOf(theirs)
      Object.assign(server.current, {
        revision: theirs.revision,
        text: theirs.brief,
        settings: theirSettings,
      })
      latest.current = theirs.brief
      latestSettings.current = theirSettings
      setBrief(theirs.brief)
      setSettings(theirSettings)
      setFacts(theirs)
      setStatus("saved")
      resume()
    },
  }

  /**
   * Runs a history pick behind the saves in flight, with the revision the next save would send.
   * A success takes the returned brief, name and revision and keeps the page's other settings. A
   * conflict raises the banner and a gone draft leaves the page. Other refusals change nothing.
   */
  const pickHistory = (call: (revision: number) => Promise<ChatDraftPickResult>) => {
    clearTimeout(timer.current)
    const run = chain.current.then(async (): Promise<ChatDraftPickResult | undefined> => {
      if (phase.current !== "editing") return
      const res = await call(server.current.revision)
      if (res.refused || !res.draft) {
        if (res.reason === ReasonDraftRevisionConflict) {
          blocked.current = true
          setBanner(true)
          setStatus("none")
        } else if (res.reason === ReasonDraftGone) leaveGone()
        return res
      }
      const picked = res.draft
      Object.assign(server.current, { revision: picked.revision, text: picked.brief })
      // The saved settings include the name, so the next flush must not resend it.
      server.current.settings = { ...server.current.settings, name: picked.name }
      latest.current = picked.brief
      latestSettings.current = { ...latestSettings.current, name: picked.name }
      setBrief(picked.brief)
      setSettings(latestSettings.current)
      setFacts(picked)
      setStatus("saved")
      return res
    })
    chain.current = run.then(
      () => {},
      () => {},
    )
    return run
  }

  // The sidebar row and the facts follow a change elsewhere; a gone draft sends the page away.
  const conn = useConnectionContext()
  // The client and connection are fixed for the page's life, so the effects below read them and
  // `flush` through a ref. `flush` and `leaveGone` read refs only, so a call from an old render is safe.
  const latestFns = useLatest({ conn, drafts, flush, leaveGone })
  useEffect(() => {
    const off = latestFns.current.conn.onEvent((_address, type) => {
      if (type !== EventChatDraftsChanged || phase.current === "finished") return
      latestFns.current.drafts
        .read(initial.id)
        .then((res) => {
          if (phase.current === "finished") return
          if (res.kind === "ok") setFacts((was) => ({ ...res.draft, brief: was.brief }))
          else if (res.kind === "gone") latestFns.current.leaveGone(res.started)
        })
        .catch(() => {})
    })
    return () => {
      off()
    }
  }, [initial.id])

  useEffect(() => {
    const leave = () => latestFns.current.flush()
    window.addEventListener("pagehide", leave)
    return () => {
      window.removeEventListener("pagehide", leave)
      leave()
    }
  }, [])

  useEffect(() => {
    if (status !== "unsaved") return
    const id = setInterval(() => latestFns.current.flush(), RETRY_MS)
    return () => clearInterval(id)
  }, [status])

  const touched = () => {
    if (!blocked.current && dirty()) setStatus("saving")
    clearTimeout(timer.current)
    timer.current = setTimeout(flush, AUTOSAVE_MS)
  }
  const edit = (next: string) => {
    setBrief(next)
    latest.current = next
    touched()
  }
  const editSettings = (patch: Partial<ChatSettings>) => {
    latestSettings.current = { ...latestSettings.current, ...patch }
    setSettings(latestSettings.current)
    touched()
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

  return {
    facts,
    brief,
    edit,
    settings,
    editSettings,
    status,
    banner,
    bannerActions,
    pickHistory,
    remove,
    start,
    starting,
    startError,
  }
}
