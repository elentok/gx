import { Box, Spinner } from "@chakra-ui/react"
import { type ReactNode, useCallback, useState } from "react"
import type { DiffFile } from "../generated/server"
import { withKeys } from "../uniqueKeys"
import { Button } from "../ui/button/Button"
import { useKeyedFetch } from "../useKeyedFetch"
import type { DiffBlobsBody, FetchDiffBlob, FetchDiffCommitPatches, FetchDiffPatch } from "./api"
import { DiffBody, type DiffView } from "./DiffBody"
import { BinaryBody, isBinaryFile } from "./ImageDiff"

export type PatchRange = { project: string; firstSha: string; secondSha: string }

/** A whole-range patch has no label; a narrowed view has one part per commit, labelled by subject. */
type PatchPart = { label?: string; text: string }

/** Sticks to the top of the diff pane; the tree row carries the status and counts. */
function PathLine({ file }: { file: DiffFile }) {
  const renamed = !!file.old_path && file.old_path !== file.path
  return (
    <Box
      data-testid="file-path"
      position="sticky"
      top="0"
      zIndex="1"
      bg="surface"
      pb="2"
      fontFamily="mono"
      fontSize="sm"
      color="text.muted"
      overflowWrap="anywhere"
    >
      {renamed ? `${file.old_path} → ${file.path}` : file.path}
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
  const narrowKey = commits?.join(",") ?? ""
  const key = `${range.project}\n${range.firstSha}\n${range.secondSha}\n${file.path}\n${narrowKey}\n${nonce}`

  const fresh = useKeyedFetch<PatchPart[]>(key, () =>
    commits && fetchCommitPatches
      ? fetchCommitPatches(range.project, range.firstSha, range.secondSha, file.path, commits).then(
          (ps) => ps.map((p) => ({ label: p.subject, text: p.patch })),
        )
      : fetchPatch(range.project, range.firstSha, range.secondSha, file.path).then((text) => [
          { text },
        ]),
  )
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
      {withKeys(fresh.value, (part) => part.label ?? "").map(({ item: part, index: i, key }) => (
        <Box key={key} data-testid={part.label === undefined ? undefined : "commit-patch"}>
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
  let body: ReactNode
  if (isBinaryFile(file)) {
    body = <BinaryBody file={file} range={range} fetchBlob={fetchBlob} />
  } else if ((file.large || file.generated) && !loaded) {
    body = <Collapsed file={file} onLoad={() => setLoaded(true)} />
  } else {
    body = <Body {...props} narrowed={narrowed} loadBlobs={loadBlobs} />
  }
  return (
    <Box data-testid="file-card" data-file={file.path}>
      <PathLine file={file} />
      {body}
    </Box>
  )
}
