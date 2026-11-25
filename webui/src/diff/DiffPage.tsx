import { Box, chakra } from "@chakra-ui/react"
import { type KeyboardEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react"
import { useSearchParams } from "react-router"
import { fetchArchivedEpicDetail, fetchEpicDetail, type FetchEpicDetail } from "../epic/detail"
import type { DiffFile } from "../generated/server"
import { useLatest } from "../hooks"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { isTextTarget } from "../shell/keys"
import { FileFilter, FileTree } from "./FileTreePanel"
import { filterFiles, treeOrder } from "./fileTree"
import { Button } from "../ui/button/Button"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import type { Keyed } from "../useKeyedFetch"
import { useDevice } from "../useWide"
import { useReadyWhen } from "../useReady"
import {
  fetchDiffFiles,
  fetchDiffBlob,
  fetchDiffCommitPatches,
  fetchDiffCommits,
  fetchDiffPatch,
  type DiffFilesResult,
  type FetchDiffBlob,
  type FetchDiffCommitPatches,
  type FetchDiffCommits,
  type FetchDiffFiles,
  type FetchDiffPatch,
} from "./api"
import { CommitList, TRUNCATED_TEXT } from "./CommitList"
import type { DiffView } from "./DiffBody"
import { DiffFileCard, type PatchRange } from "./DiffFileCard"
import { diffQuery, parseDiffUrl } from "./diffUrl"
import { useDiffData } from "./useDiffData"
import { readView, writeView } from "./viewPref"

export type { DiffView }

type Loaded = Keyed<DiffFilesResult>

const NO_RANGE_TEXT = "gx has no exact range for this epic. Type two refs to compare them."

function RefInput({
  label,
  value,
  invalid,
  phone,
  onChange,
  onKeyDown,
}: {
  phone: boolean
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
      flex={phone ? "1 1 0" : "1 1 140px"}
      minW="0"
      h="40px"
      px={phone ? "2" : "3"}
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
      p="2px"
      bg="surface.raised"
      borderRadius="6px"
    >
      {(["unified", "split"] as const).map((v) => (
        <chakra.button
          key={v}
          type="button"
          aria-pressed={v === view}
          onClick={() => onChange(v)}
          h="28px"
          px="3"
          fontSize="xs"
          borderRadius="4px"
          bg={v === view ? "sidebar.current" : "transparent"}
          color={v === view ? "text" : "text.dim"}
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
  action,
  children,
}: {
  kind: string
  title: string
  tone: "muted" | "waiting" | "deadlocked"
  action?: ReactNode
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
      {action && <Box mt="3">{action}</Box>}
    </Box>
  )
}

function RangeChip({ base, tip }: { base: string; tip: string }) {
  return (
    <Box
      data-testid="range-chip"
      display="inline-flex"
      alignItems="center"
      gap="1.5"
      h="28px"
      px="3"
      borderRadius="full"
      bg="surface.raised"
      color="text.muted"
      fontSize="sm"
      minW="0"
      maxW="100%"
    >
      <Box as="span" fontFamily="mono" fontSize="xs" truncate minW="0">
        {base}
      </Box>
      <Box as="span" aria-hidden="true" color="text.faint">
        →
      </Box>
      <Box as="span" fontFamily="mono" fontSize="xs" color="text" truncate minW="0">
        {tip}
      </Box>
    </Box>
  )
}

function Counts({ files }: { files: DiffFile[] }) {
  const added = files.reduce((n, f) => n + f.added, 0)
  const deleted = files.reduce((n, f) => n + f.deleted, 0)
  return (
    <Box data-testid="diff-counts" as="span" fontSize="sm" color="text.dim">
      {`${files.length} files `}
      <Box as="span" color="tone.done.fg">{`+${added}`}</Box>{" "}
      <Box as="span" color="tone.deadlocked.fg">{`−${deleted}`}</Box>
    </Box>
  )
}

function Tabs({
  tab,
  fileCount,
  onChange,
}: {
  tab: "files" | "diff"
  fileCount: number
  onChange: (t: "files" | "diff") => void
}) {
  return (
    <Box role="tablist" display="flex" mt="4" borderBottom="1px solid" borderColor="border">
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
          mb="-1px"
          borderBottom="2px solid"
          borderColor={t === tab ? "text" : "transparent"}
          color={t === tab ? "text" : "text.muted"}
        >
          {t === "files" ? `Files ${fileCount}` : "Diff"}
        </chakra.button>
      ))}
    </Box>
  )
}

/** The file filter shows on its own once the view lists more files than this. */
const FILTER_MIN_FILES = 15

function FilesBrowser({
  files,
  range,
  fetchPatch,
  fetchBlob,
  fetchCommitPatches,
  commits,
  commitList,
  emptyNotice,
  view,
  selectedPath,
  onSelect,
  phone,
}: {
  files: DiffFile[]
  range: PatchRange
  fetchPatch: FetchDiffPatch
  fetchBlob: FetchDiffBlob
  fetchCommitPatches: FetchDiffCommitPatches
  /** The SHAs the view is narrowed to, in range order. */
  commits: string[] | undefined
  commitList: ReactNode
  emptyNotice: string | undefined
  view: DiffView
  selectedPath: string | undefined
  onSelect: (path: string) => void
  phone: boolean
}) {
  const [filter, setFilter] = useState("")
  const [tab, setTab] = useState<"files" | "diff">("files")
  const order = useMemo(() => treeOrder(filterFiles(files, filter)), [files, filter])
  const all = useMemo(() => treeOrder(files), [files])
  const selected =
    filter !== "" && order.length === 0
      ? undefined
      : (all.find((f) => f.path === selectedPath)?.path ?? order[0]?.path ?? all[0]?.path)
  const index = order.findIndex((f) => f.path === selected)
  const selectedFile = all.find((f) => f.path === selected)

  const step = (d: 1 | -1) => {
    if (order.length === 0) return
    const from = index === -1 ? (d === 1 ? -1 : 0) : index
    onSelect(order[(from + d + order.length) % order.length].path)
  }
  const stepRef = useLatest(step)
  // Sticky: a filter that has text must not vanish when a narrowing shrinks the list.
  const [filterShown, setFilterShown] = useState(false)
  const [focusTick, setFocusTick] = useState(0)
  const filterRef = useRef<HTMLInputElement>(null)
  if (!filterShown && files.length > FILTER_MIN_FILES) setFilterShown(true)
  useEffect(() => {
    if (focusTick > 0) filterRef.current?.focus()
  }, [focusTick])
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTextTarget(e.target)) return
      if (e.key === "n") stepRef.current(1)
      else if (e.key === "p") stepRef.current(-1)
      else if (e.key === "/") {
        e.preventDefault()
        setFilterShown(true)
        setFocusTick((t) => t + 1)
      }
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
      {commitList}
      {filterShown && <FileFilter ref={filterRef} value={filter} onChange={setFilter} />}
      <FileTree files={files} filter={filter} selected={selected} onSelect={pick} />
      {emptyNotice && (
        <Box data-testid="narrow-notice" mt="3" fontSize="sm" color="text.muted">
          {emptyNotice}
        </Box>
      )}
    </>
  )
  const right = (
    <Box minW="0">
      <Box data-testid="diff-area" data-view={view} data-file={selected}>
        {!selectedFile && order.length === 0 && (
          <Box data-testid="no-files" fontSize="sm" color="text.muted">
            No files in this selection.
          </Box>
        )}
        {selectedFile && (
          <DiffFileCard
            key={selectedFile.path}
            file={selectedFile}
            range={range}
            view={view}
            fetchPatch={fetchPatch}
            fetchBlob={fetchBlob}
            fetchCommitPatches={fetchCommitPatches}
            commits={commits}
          />
        )}
      </Box>
    </Box>
  )
  if (phone) {
    return (
      <>
        <Tabs tab={tab} fileCount={files.length} onChange={setTab} />
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
  onRetry: () => void,
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
        <Notice
          kind="failed"
          tone="deadlocked"
          title="Could not load the diff"
          action={
            <Button emphasis="secondary" onClick={onRetry}>
              Try again
            </Button>
          }
        >
          The server did not answer.
        </Notice>
      ),
      files: null,
    }
  }
  const result = loaded.value
  if (result.kind === "unknown-ref") {
    const name = result.side === "first" ? first : second
    return {
      notice: (
        <Notice kind="unknown-ref" tone="deadlocked" title={`Unknown ref "${name}"`}>
          The project has no branch, tag or commit by that name. Fix the ref with the edit link.
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
          This clone does not have the commits of this range. Fetch them, then return to this
          window.
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

export function DiffPage({
  fetchFiles = fetchDiffFiles,
  fetchPatch = fetchDiffPatch,
  fetchBlob = fetchDiffBlob,
  fetchCommits = fetchDiffCommits,
  fetchCommitPatches = fetchDiffCommitPatches,
  fetchEpic = fetchEpicDetail,
  fetchArchivedEpic = fetchArchivedEpicDetail,
}: {
  fetchFiles?: FetchDiffFiles
  fetchPatch?: FetchDiffPatch
  fetchBlob?: FetchDiffBlob
  fetchCommits?: FetchDiffCommits
  fetchCommitPatches?: FetchDiffCommitPatches
  fetchEpic?: FetchEpicDetail
  fetchArchivedEpic?: FetchEpicDetail
}) {
  const [params, setParams] = useSearchParams()
  const url = parseDiffUrl(params)
  const device = useDevice()
  const [view, setView] = useState<DiffView>(readView)
  const changeView = (v: DiffView) => {
    setView(v)
    writeView(v)
  }
  const [draft, setDraft] = useState({ first: url.first, second: url.second })
  const [nonce, setNonce] = useState(0)
  const [editing, setEditing] = useState(false)

  const { project, first, second } = url
  const {
    canFetch,
    files: loaded,
    returned,
    wanted,
    narrowing,
    emptyNarrowing,
    groups,
    narrowedFiles,
  } = useDiffData(url, nonce, { fetchFiles, fetchCommits, fetchEpic, fetchArchivedEpic })

  useEffect(() => {
    setDraft({ first, second })
  }, [first, second])

  const noRange = !!url.epic && !first && !second
  const fresh = canFetch ? loaded : undefined
  const retry = () => setNonce((n) => n + 1)
  const { notice, files, invalid } = noticeOf(fresh, noRange, first, second, retry)
  const settled = noRange || !canFetch || !!fresh
  useReadyWhen(settled, "ready", "network")

  const inputsOpen = editing || !first || !second || !!invalid
  const closeEdit = () => {
    setEditing(false)
    setDraft({ first, second })
  }
  const apply = () => {
    const next = { ...url, first: draft.first.trim(), second: draft.second.trim() }
    setEditing(false)
    if (next.first === first && next.second === second) retry()
    else setParams(diffQuery(next))
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") apply()
    else if (e.key === "Escape") closeEdit()
  }

  const setNarrowing = (commits: string[] | undefined) =>
    setParams(diffQuery({ ...url, ticket: undefined, commits }))
  const emptyNotice = emptyNarrowing
    ? returned?.truncated
      ? TRUNCATED_TEXT
      : "No commits of this ticket in this range"
    : undefined

  const loadedFiles =
    !notice && files && fresh && "value" in fresh && fresh.value.kind === "files"
      ? fresh.value.files
      : undefined
  const shown = loadedFiles ? (wanted ? narrowedFiles : loadedFiles.files) : undefined
  return (
    <DetailPage title={{ kind: "Diff", name: url.epic }}>
      <Box data-testid="diff-header">
        <PageHeader
          title="Diff"
          inline={
            <Box display="flex" flexWrap="wrap" alignItems="center" gap="3" minW="0" flex="1 1 0">
              {inputsOpen ? (
                <>
                  <RefInput
                    phone={device === "phone"}
                    label="Base ref"
                    value={draft.first}
                    invalid={invalid === "first"}
                    onChange={(v) => setDraft((d) => ({ ...d, first: v }))}
                    onKeyDown={onKeyDown}
                  />
                  <Box as="span" aria-hidden="true" color="text.muted" fontFamily="mono">
                    →
                  </Box>
                  <RefInput
                    phone={device === "phone"}
                    label="Tip ref"
                    value={draft.second}
                    invalid={invalid === "second"}
                    onChange={(v) => setDraft((d) => ({ ...d, second: v }))}
                    onKeyDown={onKeyDown}
                  />
                </>
              ) : (
                <RangeChip base={first} tip={second} />
              )}
              <chakra.button
                type="button"
                onClick={() => (editing ? closeEdit() : setEditing(true))}
                color="text.dim"
                fontSize="sm"
                textDecoration="underline"
              >
                Edit refs
              </chakra.button>
              {shown && <Counts files={shown} />}
            </Box>
          }
          actions={<ViewToggle view={view} onChange={changeView} />}
        />
      </Box>
      {notice}
      {canFetch && !fresh && !noRange && <FullScreenLoader label="Loading diff" />}
      {loadedFiles && shown ? (
        <FilesBrowser
          files={shown}
          commits={wanted ? narrowing : undefined}
          commitList={
            returned && (
              <CommitList
                groups={groups}
                count={returned.commits.length}
                truncated={returned.truncated}
                narrowing={wanted}
                onNarrow={setNarrowing}
              />
            )
          }
          emptyNotice={emptyNotice}
          fetchCommitPatches={fetchCommitPatches}
          range={{
            project,
            firstSha: loadedFiles.first_sha,
            secondSha: loadedFiles.second_sha,
          }}
          fetchPatch={fetchPatch}
          fetchBlob={fetchBlob}
          view={view}
          selectedPath={url.file}
          onSelect={(file) => setParams(diffQuery({ ...url, file }))}
          phone={device === "phone"}
        />
      ) : null}
    </DetailPage>
  )
}
