import { Box, chakra } from "@chakra-ui/react"
import { type KeyboardEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react"
import { useSearchParams } from "react-router"
import type { DiffFile } from "../generated/server"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { isTextTarget } from "../shell/keys"
import { FileFilter, FileTree } from "./FileTreePanel"
import { filterFiles, treeOrder } from "./fileTree"
import { Button } from "../ui/button/Button"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useDevice } from "../useWide"
import { useReadyWhen } from "../useReady"
import { fetchDiffFiles, type DiffFilesResult, type FetchDiffFiles } from "./api"
import { diffQuery, parseDiffUrl } from "./diffUrl"

export type DiffView = "split" | "unified"

type Loaded = { key: string; result: DiffFilesResult } | { key: string; failed: true }

const NO_RANGE_TEXT = "gx has no exact range for this epic. Type two refs above to compare them."

function RefInput({
  label,
  value,
  invalid,
  onChange,
  onKeyDown,
}: {
  label: string
  value: string
  invalid: boolean
  onChange: (v: string) => void
  onKeyDown: (e: KeyboardEvent) => void
}) {
  return (
    <chakra.input
      aria-label={label}
      aria-invalid={invalid || undefined}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={onKeyDown}
      spellCheck={false}
      autoCapitalize="off"
      autoCorrect="off"
      flex="1 1 140px"
      minW="0"
      h="40px"
      px="3"
      fontFamily="mono"
      fontSize="sm"
      bg="surface.raised"
      color="text"
      border="1px solid"
      borderColor={invalid ? "tone.deadlocked.fg" : "border"}
      borderRadius="8px"
      _focusVisible={{ outline: "none", borderColor: invalid ? "tone.deadlocked.fg" : "text.dim" }}
    />
  )
}

function ViewToggle({ view, onChange }: { view: DiffView; onChange: (v: DiffView) => void }) {
  return (
    <Box
      data-testid="view-toggle"
      role="group"
      aria-label="Diff layout"
      display="flex"
      border="1px solid"
      borderColor="border"
      borderRadius="8px"
      overflow="hidden"
    >
      {(["split", "unified"] as const).map((v) => (
        <chakra.button
          key={v}
          type="button"
          aria-pressed={v === view}
          onClick={() => onChange(v)}
          h="38px"
          px="3"
          fontSize="sm"
          bg={v === view ? "tone.running.bg" : "transparent"}
          color={v === view ? "text" : "text.muted"}
        >
          {v === "split" ? "Split" : "Unified"}
        </chakra.button>
      ))}
    </Box>
  )
}

function Notice({
  kind,
  title,
  tone,
  children,
}: {
  kind: string
  title: string
  tone: "muted" | "waiting" | "deadlocked"
  children: string
}) {
  return (
    <Box
      data-testid="notice"
      data-kind={kind}
      data-tone={tone}
      mt="4"
      px="4"
      py="3"
      bg="surface.raised"
      border="1px solid"
      borderColor={tone === "muted" ? "border" : `tone.${tone}.fg`}
      borderRadius="8px"
    >
      <Box fontWeight="600" fontSize="md" color="text">
        {title}
      </Box>
      <Box mt="1" fontSize="sm" color="text.muted">
        {children}
      </Box>
    </Box>
  )
}

function FileNav({
  index,
  total,
  onStep,
}: {
  index: number
  total: number
  onStep: (d: 1 | -1) => void
}) {
  return (
    <Box data-testid="file-nav" display="flex" alignItems="center" gap="2">
      <Button emphasis="secondary" onClick={() => onStep(-1)}>
        Prev
      </Button>
      <Button emphasis="secondary" onClick={() => onStep(1)}>
        Next
      </Button>
      <Box as="span" fontSize="sm" color="text.muted">{`${index + 1} of ${total}`}</Box>
    </Box>
  )
}

function Tabs({
  tab,
  onChange,
}: {
  tab: "files" | "diff"
  onChange: (t: "files" | "diff") => void
}) {
  return (
    <Box role="tablist" display="flex" gap="2" mt="4">
      {(["files", "diff"] as const).map((t) => (
        <chakra.button
          key={t}
          type="button"
          role="tab"
          aria-selected={t === tab}
          onClick={() => onChange(t)}
          flex="1"
          h="40px"
          fontSize="sm"
          border="1px solid"
          borderColor="border"
          borderRadius="8px"
          bg={t === tab ? "tone.running.bg" : "transparent"}
          color={t === tab ? "text" : "text.muted"}
        >
          {t === "files" ? "Files" : "Diff"}
        </chakra.button>
      ))}
    </Box>
  )
}

function FilesBrowser({
  files,
  view,
  selectedPath,
  onSelect,
  phone,
}: {
  files: DiffFile[]
  view: DiffView
  selectedPath: string | undefined
  onSelect: (path: string) => void
  phone: boolean
}) {
  const [filter, setFilter] = useState("")
  const [tab, setTab] = useState<"files" | "diff">("files")
  const order = useMemo(() => treeOrder(filterFiles(files, filter)), [files, filter])
  const all = useMemo(() => treeOrder(files), [files])
  const selected = all.find((f) => f.path === selectedPath)?.path ?? order[0]?.path ?? all[0]?.path
  const index = order.findIndex((f) => f.path === selected)

  const step = (d: 1 | -1) => {
    if (order.length === 0) return
    const from = index === -1 ? (d === 1 ? -1 : 0) : index
    onSelect(order[(from + d + order.length) % order.length].path)
  }
  const stepRef = useRef(step)
  stepRef.current = step
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTextTarget(e.target)) return
      if (e.key === "n") stepRef.current(1)
      else if (e.key === "p") stepRef.current(-1)
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  const pick = (path: string) => {
    onSelect(path)
    setTab("diff")
  }
  const left = (
    <>
      <FileFilter value={filter} onChange={setFilter} />
      <FileTree files={files} filter={filter} selected={selected} onSelect={pick} />
    </>
  )
  const right = (
    <Box minW="0">
      <FileNav index={index} total={order.length} onStep={step} />
      {/* The diff render ticket draws the selected file here. */}
      <Box data-testid="diff-area" data-view={view} data-file={selected} mt="4" />
    </Box>
  )
  if (phone) {
    return (
      <>
        <Tabs tab={tab} onChange={setTab} />
        <Box mt="4">{tab === "files" ? left : right}</Box>
      </>
    )
  }
  return (
    <Box mt="6" display="grid" gridTemplateColumns="300px minmax(0, 1fr)" gap="4">
      <Box data-testid="diff-left" minW="0">
        {left}
      </Box>
      {right}
    </Box>
  )
}

function noticeOf(
  loaded: Loaded | undefined,
  noRange: boolean,
  first: string,
  second: string,
): { notice: ReactNode; files: number | null; invalid?: "first" | "second" } {
  if (noRange) {
    return {
      notice: (
        <Notice kind="no-diff-recorded" tone="waiting" title="No diff recorded">
          {NO_RANGE_TEXT}
        </Notice>
      ),
      files: null,
    }
  }
  if (!loaded) return { notice: null, files: null }
  if ("failed" in loaded) {
    return {
      notice: (
        <Notice kind="failed" tone="deadlocked" title="Could not load the diff">
          The server did not answer. Press Reload to try again.
        </Notice>
      ),
      files: null,
    }
  }
  const { result } = loaded
  if (result.kind === "unknown-ref") {
    const name = result.side === "first" ? first : second
    return {
      notice: (
        <Notice kind="unknown-ref" tone="deadlocked" title={`Unknown ref "${name}"`}>
          The project has no branch, tag or commit by that name. Fix the ref above and press Reload.
        </Notice>
      ),
      files: null,
      invalid: result.side,
    }
  }
  const { files, ...range } = result.files
  if (range.commits_unavailable) {
    return {
      notice: (
        <Notice
          kind="commits-unavailable"
          tone="waiting"
          title="Commits not available in this clone"
        >
          This clone does not have the commits of this range. Fetch them, then press Reload.
        </Notice>
      ),
      files: null,
    }
  }
  if (range.no_common_history) {
    return {
      notice: (
        <Notice kind="no-common-history" tone="muted" title="No common history">
          {`${first} and ${second} share no ancestor, so there is nothing to compare.`}
        </Notice>
      ),
      files: null,
    }
  }
  if (files.length === 0) {
    return {
      notice: (
        <Notice kind="no-changes" tone="muted" title="No changes">
          {`Nothing differs between ${first} and ${second}.`}
        </Notice>
      ),
      files: 0,
    }
  }
  return { notice: null, files: files.length }
}

export function DiffPage({ fetchFiles = fetchDiffFiles }: { fetchFiles?: FetchDiffFiles }) {
  const [params, setParams] = useSearchParams()
  const url = parseDiffUrl(params)
  const device = useDevice()
  const [view, setView] = useState<DiffView>(device === "phone" ? "unified" : "split")
  const [draft, setDraft] = useState({ first: url.first, second: url.second })
  const [nonce, setNonce] = useState(0)
  const [loaded, setLoaded] = useState<Loaded>()

  const { project, first, second } = url
  const key = `${project}\n${first}\n${second}\n${nonce}`
  const canFetch = !!project && !!first && !!second

  useEffect(() => {
    setDraft({ first, second })
  }, [first, second])

  useEffect(() => {
    if (!canFetch) return
    let current = true
    fetchFiles(project, first, second).then(
      (result) => current && setLoaded({ key, result }),
      () => current && setLoaded({ key, failed: true }),
    )
    return () => {
      current = false
    }
  }, [fetchFiles, canFetch, project, first, second, key])

  const noRange = !!url.epic && !first && !second
  const fresh = canFetch && loaded?.key === key ? loaded : undefined
  const { notice, files, invalid } = noticeOf(fresh, noRange, first, second)
  const settled = noRange || !canFetch || !!fresh
  useReadyWhen(settled, "ready", "network")

  const reload = () => {
    const next = { ...url, first: draft.first.trim(), second: draft.second.trim() }
    if (next.first === first && next.second === second) setNonce((n) => n + 1)
    else setParams(diffQuery(next))
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") reload()
  }

  const subtitle = [url.epic, files === null ? null : `${files} files`].filter(Boolean).join(" · ")
  return (
    <DetailPage title={{ kind: "Diff", name: url.epic }}>
      <PageHeader title="Diff" subtitle={subtitle} />
      <Box data-testid="ref-bar" mt="4" display="flex" flexWrap="wrap" alignItems="center" gap="2">
        <RefInput
          label="Base ref"
          value={draft.first}
          invalid={invalid === "first"}
          onChange={(v) => setDraft((d) => ({ ...d, first: v }))}
          onKeyDown={onKeyDown}
        />
        <Box as="span" aria-hidden="true" color="text.muted" fontFamily="mono">
          ...
        </Box>
        <RefInput
          label="Tip ref"
          value={draft.second}
          invalid={invalid === "second"}
          onChange={(v) => setDraft((d) => ({ ...d, second: v }))}
          onKeyDown={onKeyDown}
        />
        <Button emphasis="secondary" onClick={reload}>
          Reload
        </Button>
        <ViewToggle view={view} onChange={setView} />
      </Box>
      {notice}
      {canFetch && !fresh && !noRange && <FullScreenLoader label="Loading diff" />}
      {!notice && fresh && "result" in fresh && files ? (
        <FilesBrowser
          files={fresh.result.kind === "files" ? fresh.result.files.files : []}
          view={view}
          selectedPath={url.file}
          onSelect={(file) => setParams(diffQuery({ ...url, file }))}
          phone={device === "phone"}
        />
      ) : null}
    </DetailPage>
  )
}
