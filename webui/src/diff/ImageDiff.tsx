import { Box, Spinner } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import type { DiffFile } from "../generated/server"
import { useKeyedFetch } from "../useKeyedFetch"
import type { BlobSide, FetchDiffBlob } from "./api"
import type { PatchRange } from "./DiffFileCard"

const IMAGE_MIME: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
}

function ext(path: string): string {
  return path.slice(path.lastIndexOf(".") + 1).toLowerCase()
}

/** True for files that get no patch call: binaries and svg (which is text but never drawn). */
export function isBinaryFile(file: DiffFile): boolean {
  return !!file.binary || ext(file.path) === "svg"
}

/** A png, jpg, gif or webp that was added or changed is shown as before and after. */
export function isImageFile(file: DiffFile): boolean {
  return isBinaryFile(file) && ext(file.path) in IMAGE_MIME && file.status !== "D"
}

export function kb(size: number | undefined): string {
  return `${((size ?? 0) / 1024).toFixed(1)} KB`
}

function Note({ children, testId }: { children: string; testId: string }) {
  return (
    <Box data-testid={testId} p="4" fontSize="sm" color="text.muted">
      {children}
    </Box>
  )
}

function Loading() {
  return (
    <Box role="status" aria-label="Loading file" p="4" display="flex" justifyContent="center">
      <Spinner color="tone.muted.fg" />
    </Box>
  )
}

function ImageBox({
  label,
  side,
  mime,
  tone,
}: {
  label: string
  side: BlobSide
  mime: string
  tone: "done" | "deadlocked"
}) {
  const [url, setUrl] = useState<string>()
  const data = side.kind === "image" ? side.image_base64 : undefined
  useEffect(() => {
    if (!data) return
    const bytes = Uint8Array.from(atob(data), (c) => c.charCodeAt(0))
    const made = URL.createObjectURL(new Blob([bytes], { type: mime }))
    setUrl(made)
    return () => {
      URL.revokeObjectURL(made)
      setUrl(undefined)
    }
  }, [data, mime])
  return (
    <Box
      data-testid={`image-${label}`}
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      gap="2"
      minW="96px"
      maxW="100%"
      minH="96px"
      p="2"
      border="1px solid"
      borderColor={`tone.${tone}.fg`}
      borderRadius="6px"
    >
      {url && <img src={url} alt={label} style={{ maxWidth: "100%", maxHeight: "240px" }} />}
      <Box fontSize="sm" color="text.muted">
        {`${label}: ${side.kind === "absent" ? "none" : kb(side.size)}`}
      </Box>
    </Box>
  )
}

/** The body of a binary or image file. It makes the blob call and no patch call. */
export function BinaryBody({
  file,
  range,
  fetchBlob,
}: {
  file: DiffFile
  range: PatchRange
  fetchBlob: FetchDiffBlob
}) {
  const state = useKeyedFetch(
    `${range.project}\n${range.firstSha}\n${range.secondSha}\n${file.path}`,
    () => fetchBlob(range.project, range.firstSha, range.secondSha, file.path),
  )
  if (!state) return <Loading />
  if ("failed" in state) {
    return <Note testId="blob-error">Could not load this file.</Note>
  }
  const { old: before, new: after } = state.value
  const shown = [before, after].every((s) => s.kind === "absent" || s.kind === "image")
  if (isImageFile(file) && shown) {
    const mime = IMAGE_MIME[ext(file.path)]
    return (
      <Box p="4">
        <Box display="flex" gap="3" flexWrap="wrap">
          <ImageBox label="before" side={before} mime={mime} tone="deadlocked" />
          <ImageBox label="after" side={after} mime={mime} tone="done" />
        </Box>
        <Box mt="3" fontSize="sm" color="text.muted">
          Image file. Shown as before and after, side by side.
        </Box>
      </Box>
    )
  }
  const size = after.kind === "absent" ? before.size : after.size
  return <Note testId="binary-note">{`Binary file, ${kb(size)}`}</Note>
}
