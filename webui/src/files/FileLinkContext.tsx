import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { type HostedResolved, HostedFound, HostedMissing } from "../generated/ops"
import { createResolver, type FileResolver, type ResolveBase } from "./resolve"

export type FileLinkContextValue = {
  /** Queues candidates; every registration of one tick becomes one resolve call. */
  register(candidates: string[]): void
  /** The server verdict for a candidate, once it arrived. */
  verdict(candidate: string): HostedResolved | undefined
  /** True for a canonical path marked missing after it was found. */
  isMissing(path: string): boolean
  /** Marks a path missing: its links dim and every card says "File not found". */
  markMissing(path: string): void
  /** The found verdict whose canonical path is `path`. */
  byPath(path: string): HostedResolved | undefined
  /** Asks the server once more about one canonical path; a missing verdict marks it missing. */
  recheck(path: string): Promise<void>
  /** The file the page shows, for fragment-only links. */
  file?: string
}

const FileLinkCtx = createContext<FileLinkContextValue | null>(null)

/** Null without a provider: callers then render exactly as before. */
export function useFileLinks() {
  return useContext(FileLinkCtx)
}

/** Keeps its children plain text inside a provider, e.g. a link cannot nest in a button. */
export function NoFileLinks({ children }: { children: ReactNode }) {
  return <FileLinkCtx.Provider value={null}>{children}</FileLinkCtx.Provider>
}

export type FileLinkProviderProps = {
  base?: ResolveBase
  /** Tests inject a stub; the default is the real client for `base`. */
  resolver?: FileResolver
  /** When it changes, candidates that were missing are asked again. */
  recheckKey?: string
  file?: string
  children: ReactNode
}

export function FileLinkProvider({
  base,
  resolver,
  recheckKey,
  file,
  children,
}: FileLinkProviderProps) {
  const { session, project, dir } = base ?? {}
  const active = useMemo(
    () => resolver ?? createResolver({ session, project, dir }),
    [resolver, session, project, dir],
  )
  const [verdicts, setVerdicts] = useState<ReadonlyMap<string, HostedResolved>>(new Map())
  const [missing, setMissing] = useState<ReadonlySet<string>>(new Set())
  const pending = useRef(new Set<string>())
  const scheduled = useRef(false)
  const latest = useRef(verdicts)
  latest.current = verdicts

  const rechecking = useRef(new Set<string>())

  // Registrations and rechecks of one tick share one resolve call.
  const flushSoon = useCallback(() => {
    if (scheduled.current || pending.current.size === 0) return
    scheduled.current = true
    queueMicrotask(() => {
      scheduled.current = false
      const batch = [...pending.current]
      const recheckMissing = new Set(rechecking.current)
      pending.current.clear()
      rechecking.current.clear()
      void active.resolve(batch, { recheckMissing }).then((got) => {
        if (got.size === 0) return
        setVerdicts((prev) => new Map([...prev, ...got]))
      })
    })
  }, [active])

  const register = useCallback(
    (candidates: string[]) => {
      for (const c of candidates) if (!latest.current.has(c)) pending.current.add(c)
      flushSoon()
    },
    [flushSoon],
  )

  const lastKey = useRef(recheckKey)
  useEffect(() => {
    if (lastKey.current === recheckKey) return
    lastKey.current = recheckKey
    for (const r of latest.current.values()) {
      if (r.verdict === HostedFound) continue
      pending.current.add(r.candidate)
      rechecking.current.add(r.candidate)
    }
    flushSoon()
  }, [recheckKey, flushSoon])

  const markMissing = useCallback(
    (path: string) => setMissing((prev) => (prev.has(path) ? prev : new Set(prev).add(path))),
    [],
  )
  const recheck = useCallback(
    async (path: string) => {
      const got = await active.resolve([path], { fresh: true })
      const r = got.get(path)
      if (r?.verdict === HostedMissing) markMissing(path)
    },
    [active, markMissing],
  )

  const value = useMemo<FileLinkContextValue>(() => {
    const found = new Map<string, HostedResolved>()
    for (const r of verdicts.values()) {
      if (r.verdict === HostedFound && r.path) found.set(r.path, r)
    }
    return {
      register,
      verdict: (candidate) => verdicts.get(candidate),
      isMissing: (path) => missing.has(path),
      markMissing,
      byPath: (path) => found.get(path),
      recheck,
      file,
    }
  }, [register, verdicts, missing, markMissing, recheck, file])
  return <FileLinkCtx.Provider value={value}>{children}</FileLinkCtx.Provider>
}

/** Registers a candidate while the caller is mounted and returns its verdict if found. */
export function useFoundFile(candidate: string | undefined): HostedResolved | undefined {
  const ctx = useFileLinks()
  const register = ctx?.register
  useEffect(() => {
    if (candidate) register?.([candidate])
  }, [candidate, register])
  const r = candidate ? ctx?.verdict(candidate) : undefined
  return r?.verdict === HostedFound ? r : undefined
}
