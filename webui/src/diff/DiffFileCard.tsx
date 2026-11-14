import { Box, Spinner } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import type { DiffFile } from "../generated/server"
import { Button } from "../ui/button/Button"
import type { FetchDiffPatch } from "./api"
import { DiffBody, type DiffView } from "./DiffBody"

export type PatchRange = { project: string; firstSha: string; secondSha: string }

type Patch = { key: string; text: string } | { key: string; failed: true }

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
}: {
  file: DiffFile
  range: PatchRange
  view: DiffView
  fetchPatch: FetchDiffPatch
}) {
  const [nonce, setNonce] = useState(0)
  const [patch, setPatch] = useState<Patch>()
  const key = `${range.project}\n${range.firstSha}\n${range.secondSha}\n${file.path}\n${nonce}`

  useEffect(() => {
    let current = true
    fetchPatch(range.project, range.firstSha, range.secondSha, file.path).then(
      (text) => current && setPatch({ key, text }),
      () => current && setPatch({ key, failed: true }),
    )
    return () => {
      current = false
    }
  }, [fetchPatch, range.project, range.firstSha, range.secondSha, file.path, key])

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
  return <DiffBody patch={fresh.text} view={view} path={file.path} />
}

/** One file's card. The parent keys it by path so only one is mounted at a time. */
export function DiffFileCard(props: {
  file: DiffFile
  range: PatchRange
  view: DiffView
  fetchPatch: FetchDiffPatch
}) {
  const { file } = props
  const skipped = !!(file.large || file.generated || file.binary)
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
      {skipped ? null : <Body {...props} />}
    </Box>
  )
}
