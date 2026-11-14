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
import { type HostedResolved, HostedFound } from "../generated/ops"
import { createResolver, type FileResolver, type ResolveBase } from "./resolve"

export type FileLinkContextValue = {
  /** Queues candidates; every registration of one tick becomes one resolve call. */
  register(candidates: string[]): void
  /** The server verdict for a candidate, once it arrived. */
  verdict(candidate: string): HostedResolved | undefined
  /** True for a canonical path marked missing after it was found. */
  isMissing(path: string): boolean
  markMissing(path: string): void
  /** The file the page shows, for fragment-only links. */
  file?: string
}

const FileLinkCtx = createContext<FileLinkContextValue | null>(null)

/** Null without a provider: callers then render exactly as before. */
export function useFileLinks() {
  return useContext(FileLinkCtx)
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

  const ask = useCallback(
    (candidates: string[], recheckMissing: boolean) => {
      if (candidates.length === 0) return
      void active.resolve(candidates, { recheckMissing }).then((got) => {
        if (got.size === 0) return
        setVerdicts((prev) => new Map([...prev, ...got]))
      })
    },
    [active],
  )

  const register = useCallback(
    (candidates: string[]) => {
      for (const c of candidates) if (!latest.current.has(c)) pending.current.add(c)
      if (scheduled.current || pending.current.size === 0) return
      scheduled.current = true
      queueMicrotask(() => {
        scheduled.current = false
        const batch = [...pending.current]
        pending.current.clear()
        ask(batch, false)
      })
    },
    [ask],
  )

  const lastKey = useRef(recheckKey)
  useEffect(() => {
    if (lastKey.current === recheckKey) return
    lastKey.current = recheckKey
    const again = [...latest.current.values()]
      .filter((r) => r.verdict !== HostedFound)
      .map((r) => r.candidate)
    ask(again, true)
  }, [recheckKey, ask])

  const value = useMemo<FileLinkContextValue>(
    () => ({
      register,
      verdict: (candidate) => verdicts.get(candidate),
      isMissing: (path) => missing.has(path),
      markMissing: (path) => setMissing((prev) => new Set(prev).add(path)),
      file,
    }),
    [register, verdicts, missing, file],
  )
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
