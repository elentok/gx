import { Box, Text } from "@chakra-ui/react"
import {
  HostedKindBinary,
  HostedKindImage,
  HostedKindPDF,
  HostedKindSVG,
  type HostedResolved,
} from "../generated/ops"
import { rawUrl } from "./rawUrl"

/** Files over this size show no preview. The hover card reuses it. */
export const MAX_PREVIEW_BYTES = 2 * 1024 * 1024

type Note = { title?: string; text: string }

const TOO_LARGE: Note = {
  title: "Too large to show",
  text: "Files over 2 MB show no preview. Use Raw or Download.",
}

/** The note that replaces the preview, or undefined when the kind branch renders. */
export function previewNote(file: Pick<HostedResolved, "kind" | "size">): Note | undefined {
  if (file.kind === HostedKindPDF) {
    return { text: "PDFs open in the browser's PDF viewer. Use Raw or Download." }
  }
  if ((file.size ?? 0) > MAX_PREVIEW_BYTES) return TOO_LARGE
  if (file.kind === HostedKindBinary) {
    return { text: "No preview for this file type. Use Raw or Download." }
  }
  return undefined
}

function NoteBody({ note }: { note: Note }) {
  return (
    <Box p="5" textAlign="center" color="text.muted">
      {note.title && (
        <Text as="b" display="block" mb="1" color="text">
          {note.title}
        </Text>
      )}
      {note.text}
    </Box>
  )
}

/** Maps the resolve `kind` to its body. Later tickets add one branch per kind. */
export function ViewerBody({
  file,
  path,
  onImageLoad,
  onImageError,
}: {
  file: HostedResolved
  path: string
  onImageLoad?: (width: number, height: number) => void
  onImageError?: () => void
}) {
  const note = previewNote(file)
  if (note) return <NoteBody note={note} />
  if (file.kind === HostedKindImage || file.kind === HostedKindSVG) {
    // Always an <img>, even for SVG: scripts in an image never run.
    return (
      <Box p="4" textAlign="center">
        <img
          src={rawUrl(path)}
          alt={path.split("/").pop()}
          style={{ maxWidth: "100%", height: "auto", borderRadius: "6px" }}
          onLoad={(e) => onImageLoad?.(e.currentTarget.naturalWidth, e.currentTarget.naturalHeight)}
          onError={() => onImageError?.()}
        />
      </Box>
    )
  }
  return (
    <Text p="5" textAlign="center" color="text.muted">
      No preview yet
    </Text>
  )
}
