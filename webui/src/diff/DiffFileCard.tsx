import { Box, Spinner } from "@chakra-ui/react"
import { useCallback, useEffect, useState } from "react"
import type { DiffFile } from "../generated/server"
import { Button } from "../ui/button/Button"
import type { DiffBlobsBody, FetchDiffBlob, FetchDiffCommitPatches, FetchDiffPatch } from "./api"
import { DiffBody, type DiffView } from "./DiffBody"
import { BinaryBody, isBinaryFile } from "./ImageDiff"

export type PatchRange = { project: string; firstSha: string; secondSha: string }

/** A whole-range patch has no label; a narrowed view has one part per commit, labelled by subject. */
type PatchPart = { label?: string; text: string }
type Patch = { key: string; parts: PatchPart[] } | { key: string; failed: true }

const STATUS_TONE: Record<string, string> = { A: "done", D: "deadlocked" }

function Header({ file }: { file: DiffFile }) {
  const renamed = !!file.old_path && file.old_path !== file.path
  const slash = file.path.lastIndexOf("/") + 1
  const name = renamed ? (
    `${file.old_path} → ${file.path}`
  ) : (
    <>
      <Box as="span" color="text.muted">
        {file.path.slice(0, slash)}
      </Box>
      {file.path.slice(slash)}
    </>
  )
  return (
    <Box
      data-testid="file-card-header"
      display="flex"
      alignItems="center"
      gap="3"
      px="3"
      py="2"
      borderBottom="1px solid"
      borderColor="border"
      bg="surface.raised"
      fontFamily="mono"
      fontSize="sm"
    >
      <Box
        as="span"
        data-testid="file-status"
        color={`tone.${STATUS_TONE[file.status] ?? "muted"}.fg`}
        border="1px solid"
        borderColor="currentColor"
        borderRadius="full"
        px="2"
        fontSize="xs"
      >
        {file.status}
      </Box>
      <Box as="span" flex="1" minW="0" overflowWrap="anywhere" color="text">
        {name}
      </Box>
      <Box as="span" color="tone.done.fg">{`+${file.added}`}</Box>
      <Box as="span" color="tone.deadlocked.fg">{`−${file.deleted}`}</Box>
    </Box>
  )
}

function Body({
  file,
  range,
  view,
  fetchPatch,
  fetchCommitPatches,
  commits,
  narrowed,
  loadBlobs,
}: {
  file: DiffFile
  range: PatchRange
  view: DiffView
  fetchPatch: FetchDiffPatch
  fetchCommitPatches?: FetchDiffCommitPatches
  commits?: string[]
  narrowed: boolean
  loadBlobs: () => Promise<DiffBlobsBody>
}) {
  const [nonce, setNonce] = useState(0)
  const [patch, setPatch] = useState<Patch>()
  const narrowKey = commits?.join(",") ?? ""
  const key = `${range.project}\n${range.firstSha}\n${range.secondSha}\n${file.path}\n${narrowKey}\n${nonce}`

  useEffect(() => {
    let current = true
    const load: Promise<PatchPart[]> =
      commits && fetchCommitPatches
        ? fetchCommitPatches(
            range.project,
            range.firstSha,
            range.secondSha,
            file.path,
            commits,
          ).then((ps) => ps.map((p) => ({ label: p.subject, text: p.patch })))
        : fetchPatch(range.project, range.firstSha, range.secondSha, file.path).then((text) => [
            { text },
          ])
    load.then(
      (parts) => current && setPatch({ key, parts }),
      () => current && setPatch({ key, failed: true }),
    )
    return () => {
      current = false
    }
    // `commits` is covered by narrowKey, which the key carries.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    fetchPatch,
    fetchCommitPatches,
    range.project,
    range.firstSha,
    range.secondSha,
    file.path,
    key,
  ])

  const fresh = patch?.key === key ? patch : undefined
  if (!fresh) {
    return (
      <Box role="status" aria-label="Loading patch" p="4" display="flex" justifyContent="center">
        <Spinner color="tone.muted.fg" />
      </Box>
    )
  }
  if ("failed" in fresh) {
    return (
      <Box data-testid="patch-error" p="3" fontSize="sm" color="text.muted">
        Could not load this file's diff.{" "}
        <Button emphasis="secondary" onClick={() => setNonce((n) => n + 1)}>
          Retry
        </Button>
      </Box>
    )
  }
  return (
    <>
      {fresh.parts.map((part, i) => (
        <Box key={i} data-testid={part.label === undefined ? undefined : "commit-patch"}>
          {part.label !== undefined && (
            <Box
              data-testid="commit-patch-label"
              px="3"
              py="1"
              fontSize="sm"
              color="text.muted"
              bg="surface.raised"
              borderTop={i === 0 ? undefined : "1px solid"}
              borderBottom="1px solid"
              borderColor="border"
            >
              {part.label}
            </Box>
          )}
          <DiffBody
            patch={part.text}
            view={view}
            path={file.path}
            large={!!file.large}
            narrowed={narrowed}
            loadBlobs={loadBlobs}
          />
        </Box>
      ))}
    </>
  )
}

function Collapsed({ file, onLoad }: { file: DiffFile; onLoad: () => void }) {
  const text = file.large
    ? `Large diff, collapsed (${file.added + file.deleted} lines)`
    : "Generated file, collapsed"
  return (
    <Box data-testid="collapsed-diff" p="4" fontSize="sm" color="text.muted">
      {text}{" "}
      <Button emphasis="secondary" onClick={onLoad}>
        Load diff
      </Button>
    </Box>
  )
}

/**
 * One file's card. The parent keys it by path so only one is mounted at a time, which also resets
 * the loaded state when the file changes. `commits` narrows the card to those commits' own patches,
 * which have no expand rows.
 */
export function DiffFileCard(props: {
  file: DiffFile
  range: PatchRange
  view: DiffView
  fetchPatch: FetchDiffPatch
  fetchBlob: FetchDiffBlob
  fetchCommitPatches?: FetchDiffCommitPatches
  commits?: string[]
  narrowed?: boolean
}) {
  const { file, range, fetchBlob, commits } = props
  const narrowed = props.narrowed || !!commits
  const [loaded, setLoaded] = useState(false)
  const loadBlobs = useCallback(
    () => fetchBlob(range.project, range.firstSha, range.secondSha, file.path),
    [fetchBlob, range.project, range.firstSha, range.secondSha, file.path],
  )
  let body
  if (isBinaryFile(file)) {
    body = <BinaryBody file={file} range={range} fetchBlob={fetchBlob} />
  } else if ((file.large || file.generated) && !loaded) {
    body = <Collapsed file={file} onLoad={() => setLoaded(true)} />
  } else {
    body = <Body {...props} narrowed={narrowed} loadBlobs={loadBlobs} />
  }
  return (
    <Box
      data-testid="file-card"
      data-file={file.path}
      border="1px solid"
      borderColor="border"
      borderRadius="8px"
      overflow="hidden"
    >
      <Header file={file} />
      {body}
    </Box>
  )
}
