import { Box, chakra } from "@chakra-ui/react"
import { type KeyboardEvent, type ReactNode, useEffect, useState } from "react"
import { useSearchParams } from "react-router"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { Button } from "../ui/button/Button"
import { EmptyState } from "../ui/empty-state/EmptyState"
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
    <Box data-testid="view-toggle" role="group" aria-label="Diff layout" display="flex" gap="1">
      {(["split", "unified"] as const).map((v) => (
        <Button
          key={v}
          size="md"
          tone={v === view ? "running" : "muted"}
          emphasis={v === view ? "secondary" : "ghost"}
          aria-pressed={v === view}
          onClick={() => onChange(v)}
        >
          {v === "split" ? "Split" : "Unified"}
        </Button>
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
    <Box data-testid="notice" data-kind={kind} mt="6">
      <EmptyState tone={tone} title={title}>
        {children}
      </EmptyState>
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
  const { DiffRange: range, files } = result.files
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
      {!notice && fresh && files ? (
        // The file tree, file view and commit list land here in later tickets.
        <Box data-testid="diff-area" data-view={view} mt="6" />
      ) : null}
    </DetailPage>
  )
}
