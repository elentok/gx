import { useEffect, useRef, useState } from "react"
import { useLocation } from "react-router"
import type { DesignDraft } from "../generated/server"
import { useConnectionContext, useConnectionState } from "../connection"
import { defaultProject } from "../workspaces/defaultProject"
import type { DraftsClient } from "./drafts"
import { hasText, type Fields } from "./draftFields"
import { slugProblem } from "./draftSlug"
import { useDraftImages, type ImageSlot } from "./useDraftImages"
import { clearUnsaved, stashUnsaved, type Restored } from "./unsaved"

const AUTOSAVE_MS = 800
const RETRY_MS = 5000

export type SaveStatus = "none" | "saving" | "saved" | "unsaved"
export type Banner = "conflict" | undefined

/** `editing` saves; `starting` is Start in flight; `finished` is Start, Remove or a switch having taken the draft over. */
type Phase = "editing" | "starting" | "finished" | "gone"

const targetKey = (d: Fields) => JSON.stringify({ slug: d.slug, project: d.project })

/**
 * The open draft's fields, their autosave and the images that share its id. The prompt saves with the
 * draft's revision; the slug and project have their own revision-free write, so a stale prompt save
 * never carries them. One timer and one status cover both.
 */
export function useDraftAutosave({
  drafts,
  initial,
  restored,
  projects,
  lastProject,
  slot,
}: {
  drafts: DraftsClient
  initial: DesignDraft
  restored?: Restored
  projects: string[]
  lastProject: string
  slot: ImageSlot
}) {
  const { pathname } = useLocation()
  const sessions = useConnectionState(useConnectionContext()).snapshot?.sessions ?? []
  // A saved draft keeps its project; one whose project is gone is moved as a new draft would start.
  const keptProject = projects.includes(initial.project) ? initial.project : undefined
  const [draft, setDraft] = useState<Fields>(() => ({
    slug: initial.slug,
    idea: initial.idea,
    project: keptProject ?? defaultProject({ pathname, sessions, projects, lastProject }),
  }))
  const [label, setLabel] = useState(initial.label)
  const [movedFrom] = useState(() =>
    !keptProject && initial.project ? initial.project : undefined,
  )
  const [moved, setMoved] = useState(movedFrom !== undefined)
  const [status, setStatus] = useState<SaveStatus>(
    restored === "unsaved" ? "unsaved" : !restored && hasText(initial) ? "saved" : "none",
  )

  const latest = useRef(draft)
  // `text` and `target` are what the server last had, so an unchanged field saves nothing. A restored
  // prompt never reached the server. A moved draft starts out of sync, so its first save writes the
  // new project; `unedited` is the target as moved, which that save stamps without a new edited-at.
  const unedited = useRef(movedFrom !== undefined ? targetKey(draft) : undefined)
  const server = useRef<{ id: string; revision: number; text?: string; target: string }>({
    id: initial.id,
    revision: initial.revision,
    text: restored ? undefined : initial.idea,
    target: targetKey(movedFrom !== undefined ? { ...draft, project: movedFrom } : draft),
  })
  // Set after a failed text save until one succeeds; a leave then keeps the text in localStorage.
  const failing = useRef(restored === "unsaved")
  const stashed = useRef(!!restored)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const chain = useRef<Promise<void>>(Promise.resolve())
  const phase = useRef<Phase>("editing")
  // The banner's ref twin, so a flush in flight sees it before a render.
  const blocked = useRef<Banner>(restored === "conflict" ? restored : undefined)
  const [banner, setBanner] = useState<Banner>(blocked.current)
  // Set when a save finds the draft gone; the page then asks where it went.
  const [gone, setGone] = useState(false)
  const pageHidden = useRef(false)
  const mounted = useRef(false)

  /** The draft's id for an image upload, once the saves in flight are done. */
  const ensureDraft = async () => {
    await chain.current
    return server.current.id
  }
  const imgs = useDraftImages({
    slot,
    draftId: () => server.current.id,
    ensureDraft,
    upload: drafts.uploadImage,
    remove: drafts.removeImage,
  })

  /** Keeps the unsaved prompt in localStorage for the next open. */
  const stash = () => {
    const s = server.current
    stashUnsaved(s.id, { revision: s.revision, idea: latest.current.idea })
    stashed.current = true
  }
  /** The prompt reached the server, or Start or Remove took it over. */
  const settle = () => {
    if (stashed.current) clearUnsaved(server.current.id)
    stashed.current = false
  }

  /** A refusal pauses autosave until the person picks a way out of the banner. */
  const raise = () => {
    stash()
    blocked.current = "conflict"
    setBanner("conflict")
    setStatus("none")
  }
  /** A save found the draft gone: nothing saves from here on. */
  const markGone = () => {
    phase.current = "gone"
    setStatus("none")
    setGone(true)
  }

  /** Saves the prompt; false when it did not settle. */
  const persistText = async () => {
    const s = server.current
    const idea = latest.current.idea
    if (idea === s.text) return true
    let res
    try {
      res = await drafts.save({ id: s.id, idea, revision: s.revision })
    } catch {
      // The text stays in memory and is retried on the next keystroke and every RETRY_MS.
      failing.current = true
      stash()
      setStatus("unsaved")
      return false
    }
    failing.current = false
    if (res.kind === "ok") {
      Object.assign(s, { revision: res.draft.revision, text: idea })
      setLabel(res.draft.label)
      settle()
      return true
    }
    if (res.kind === "conflict") raise()
    else markGone()
    return false
  }

  /** Writes the slug and project; false when it did not settle. A slug that is not valid is never written. */
  const persistFields = async () => {
    const s = server.current
    const fields = latest.current
    const key = targetKey(fields)
    if (key === s.target || slugProblem(fields.slug)) return true
    let res
    try {
      res = await drafts.saveFields({
        id: s.id,
        project: fields.project,
        slug: fields.slug,
        ...(key === unedited.current && { keep_edited_at: true }),
      })
    } catch {
      setStatus("unsaved")
      return false
    }
    if (res.kind === "ok") {
      unedited.current = undefined
      s.target = key
      setLabel(res.draft.label)
      return true
    }
    markGone()
    return false
  }

  const persist = async () => {
    if (phase.current !== "editing") return
    const textOk = await persistText()
    const fieldsOk = await persistFields()
    if (textOk && fieldsOk) setStatus(hasText(latest.current) || imgs.count > 0 ? "saved" : "none")
  }

  /** Reads the server's copy; a draft that is gone leaves the page instead. */
  const readServer = async () => {
    const res = await drafts.read(server.current.id)
    if (res.kind === "ok") return res.draft
    markGone()
  }
  const resume = () => {
    blocked.current = undefined
    setBanner(undefined)
    flush()
  }
  const bannerActions = {
    keepMine: async () => {
      const theirs = await readServer()
      if (theirs) {
        server.current.revision = theirs.revision
        resume()
      }
    },
    loadTheirs: async () => {
      const theirs = await readServer()
      if (!theirs) return
      const next = { slug: theirs.slug, idea: theirs.idea, project: theirs.project }
      Object.assign(server.current, {
        revision: theirs.revision,
        text: theirs.idea,
        target: targetKey(next),
      })
      latest.current = next
      setDraft(next)
      setLabel(theirs.label)
      imgs.reset(theirs.images)
      settle()
      resume()
    },
  }

  /** Takes a slug or project changed elsewhere, unless the person has an edit of their own pending. */
  const adoptTarget = (theirs: DesignDraft) => {
    const s = server.current
    if (targetKey(latest.current) !== s.target) return
    const next = { ...latest.current, slug: theirs.slug, project: theirs.project }
    const key = targetKey(next)
    if (key === s.target) return
    s.target = key
    latest.current = next
    setDraft(next)
    setLabel(theirs.label)
  }

  const flush = () => {
    clearTimeout(timer.current)
    if (phase.current === "finished" || blocked.current) return
    chain.current = chain.current.then(persist).catch(() => setStatus("none"))
  }
  /** Start, Remove or a switch has taken the draft over: nothing saves from here on. */
  const takeOver = () => {
    phase.current = "finished"
  }
  const stopSaving = () => {
    takeOver()
    clearTimeout(timer.current)
    settle()
  }
  /** Cancels the pending autosave; resolves once the saves in flight are done. */
  const idle = () => {
    clearTimeout(timer.current)
    return chain.current
  }
  const currentId = () => server.current.id

  // Every effect below reads refs only, so `flush` is safe to call from a stale closure.
  useEffect(() => {
    const leave = () => {
      // The save below may not finish before the page goes, so the fallback is written first.
      if (
        phase.current !== "finished" &&
        (failing.current || blocked.current) &&
        latest.current.idea !== server.current.text
      )
        stash()
      flush()
    }
    const hide = () => {
      pageHidden.current = true
      leave()
    }
    window.addEventListener("pagehide", hide)
    mounted.current = true
    return () => {
      window.removeEventListener("pagehide", hide)
      leave()
      mounted.current = false
      // In-app navigation only: a reload or tab close may come back to the page. The microtask
      // lets React's dev-mode remount cancel the request.
      queueMicrotask(() => {
        if (!mounted.current && !pageHidden.current && phase.current === "editing" && !hasContent())
          drafts.removeIfEmpty(server.current.id).catch(() => {})
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (movedFrom !== undefined) flush()
    // Once, on open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (status !== "unsaved") return
    const id = setInterval(flush, RETRY_MS)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status])

  const patch = (p: Partial<Fields>) => {
    const next = { ...draft, ...p }
    setDraft(next)
    latest.current = next
    const s = server.current
    const dirty = next.idea !== s.text || (targetKey(next) !== s.target && !slugProblem(next.slug))
    if (!blocked.current && dirty) setStatus("saving")
    clearTimeout(timer.current)
    timer.current = setTimeout(flush, AUTOSAVE_MS)
  }
  const pickProject = (project: string) => {
    setMoved(false)
    patch({ project })
    drafts.setLastProject(project).catch(() => {})
  }
  /** Removes the draft: stops saving and deletes it once an in-flight save is done. */
  const remove = () => {
    stopSaving()
    chain.current = chain.current
      .then(async () => {
        await drafts.remove(server.current.id)
      })
      .catch(() => {})
  }
  /** Runs Start's server call with saves held back; a result that is not `refused` finishes the draft. */
  const start = async <T extends { refused?: unknown }>(call: (draftId: string) => Promise<T>) => {
    phase.current = "starting"
    clearTimeout(timer.current)
    try {
      await chain.current
      const res = await call(server.current.id)
      if (!res.refused) {
        phase.current = "finished"
        settle()
      }
      return res
    } finally {
      if (phase.current === "starting") phase.current = "editing"
      // A failed Start keeps the draft, so the text typed meanwhile still saves.
      flush()
    }
  }

  // Images keep a draft with no text from being blank. Reads refs only, for the unmount cleanup.
  const imageCount = useRef(0)
  imageCount.current = imgs.count
  function hasContent() {
    return hasText(latest.current) || imageCount.current > 0
  }

  return {
    draft,
    label,
    patch,
    pickProject,
    status,
    banner,
    bannerActions,
    moved,
    movedFrom,
    imgs,
    gone,
    adoptTarget,
    /** Whether a gone draft should send the page elsewhere: not once Start or Remove has taken it over. */
    mayRedirect: () => phase.current === "editing" || phase.current === "gone",
    remove,
    start,
    flush,
    /** For the switching hook: what a draft switch needs, without the refs behind it. */
    hasContent,
    stopSaving,
    takeOver,
    idle,
    currentId,
  }
}

export type DraftAutosave = ReturnType<typeof useDraftAutosave>
