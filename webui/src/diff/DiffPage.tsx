import { Box, chakra } from "@chakra-ui/react"
import { type KeyboardEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react"
import { useSearchParams } from "react-router"
import { fetchArchivedEpicDetail, fetchEpicDetail, type FetchEpicDetail } from "../epic/detail"
import type { DiffFile } from "../generated/server"
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
import { ALL_COMMITS, CommitList, NarrowChip, narrowingLabel, TRUNCATED_TEXT } from "./CommitList"
import type { DiffView } from "./DiffBody"
import { DiffFileCard, type PatchRange } from "./DiffFileCard"
import { diffQuery, parseDiffUrl } from "./diffUrl"
import { useDiffData } from "./useDiffData"

export type { DiffView }

type Loaded = Keyed<DiffFilesResult>

const NO_RANGE_TEXT = "gx has no exact range for this epic. Type two refs above to compare them."

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
  scope,
  index,
  visible,
  onStep,
}: {
  scope: string
  index: number
  visible: DiffFile[]
  onStep: (d: 1 | -1) => void
}) {
  const added = visible.reduce((n, f) => n + f.added, 0)
  const deleted = visible.reduce((n, f) => n + f.deleted, 0)
  const hint = (k: string) => (
    <Box as="span" aria-hidden="true" fontFamily="mono" color="text.muted" mr="2">
      {k}
    </Box>
  )
  return (
    <Box data-testid="file-nav" display="flex" alignItems="center" gap="2">
      <Button emphasis="secondary" disabled={visible.length === 0} onClick={() => onStep(-1)}>
        {hint("p")}
        Prev
      </Button>
      <Button emphasis="secondary" disabled={visible.length === 0} onClick={() => onStep(1)}>
        {hint("n")}
        Next
      </Button>
      <Box
        as="span"
        fontSize="sm"
        color="text.muted"
        minW="0"
        overflow="hidden"
        textOverflow="ellipsis"
        whiteSpace="nowrap"
      >
        {`${scope} · ${visible.length === 0 ? "" : `${index + 1} of ${visible.length} · `}${visible.length} files · `}
        <Box as="span" color="tone.done.fg">{`+${added}`}</Box>{" "}
        <Box as="span" color="tone.deadlocked.fg">{`−${deleted}`}</Box>
      </Box>
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

function FilesBrowser({
  files,
  range,
  fetchPatch,
  fetchBlob,
  fetchCommitPatches,
  commits,
  commitList,
  scope,
  emptyNotice,
  view,
  selectedPath,
  onSelect,
  phone,
}: {
  /** What the view is narrowed to, as the commit disclosure names it. */
  scope: string
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
      {commitList}
      <FileFilter value={filter} onChange={setFilter} count={filterFiles(files, filter).length} />
      <FileTree
        files={files}
        filter={filter}
        selected={selected}
        onSelect={pick}
        dirCounts={!phone}
      />
      {emptyNotice && (
        <Box data-testid="narrow-notice" mt="3" fontSize="sm" color="text.muted">
          {emptyNotice}
        </Box>
      )}
    </>
  )
  const right = (
    <Box minW="0">
      <FileNav scope={scope} index={index} visible={order} onStep={step} />
      <Box data-testid="diff-area" data-view={view} data-file={selected} mt="4">
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
  const result = loaded.value
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
  const [view, setView] = useState<DiffView>(device === "phone" ? "unified" : "split")
  const [draft, setDraft] = useState({ first: url.first, second: url.second })
  const [nonce, setNonce] = useState(0)

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

  const setNarrowing = (commits: string[] | undefined) =>
    setParams(diffQuery({ ...url, ticket: undefined, commits }))
  const chipLabel = narrowingLabel(groups, wanted)
  const emptyNotice = emptyNarrowing
    ? returned?.truncated
      ? TRUNCATED_TEXT
      : "No commits of this ticket in this range"
    : undefined

  const subtitle = [
    url.epic,
    returned
      ? `${returned.commits.length} ${returned.commits.length === 1 ? "commit" : "commits"}`
      : null,
    files === null ? null : `${files} files`,
  ]
    .filter(Boolean)
    .join(" · ")
  return (
    <DetailPage title={{ kind: "Diff", name: url.epic }}>
      <PageHeader title="Diff" subtitle={subtitle} />
      <Box
        data-testid="ref-bar"
        mt="4"
        display="flex"
        flexWrap={device === "phone" ? "nowrap" : "wrap"}
        alignItems="center"
        gap="2"
      >
        <RefInput
          phone={device === "phone"}
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
          phone={device === "phone"}
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
      {chipLabel && <NarrowChip label={chipLabel} onClear={() => setNarrowing(undefined)} />}
      {notice}
      {canFetch && !fresh && !noRange && <FullScreenLoader label="Loading diff" />}
      {!notice && fresh && "value" in fresh && fresh.value.kind === "files" && files ? (
        <FilesBrowser
          files={wanted ? narrowedFiles : fresh.value.files.files}
          scope={chipLabel ?? ALL_COMMITS}
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
            firstSha: fresh.value.files.first_sha,
            secondSha: fresh.value.files.second_sha,
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
