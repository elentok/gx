import { Box, Text } from "@chakra-ui/react"
import {
  HostedKindBinary,
  HostedKindCode,
  HostedKindImage,
  HostedKindMarkdown,
  HostedKindMermaid,
  HostedKindPDF,
  HostedKindSVG,
  HostedKindText,
  type HostedResolved,
} from "../generated/ops"
import { Spinner } from "../ui/spinner/Spinner"
import { CodeView } from "./CodeView"
import { languageOf } from "./language"
import { MarkdownView } from "./MarkdownView"
import { MermaidView } from "./MermaidView"
import { rawUrl } from "./rawUrl"
import type { FileResolver } from "./resolve"

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
  text,
  line,
  heading,
  source = false,
  resolver,
  onImageLoad,
  onImageError,
}: {
  /** Markdown and Mermaid only: show the raw text instead of the rendered file. */
  source?: boolean
  /** Markdown only: slug of the heading to scroll to. */
  heading?: string
  resolver?: FileResolver
  file: HostedResolved
  path: string
  /** The file text of a code or text file; undefined while it loads. */
  text?: string
  line?: number
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
  if (file.kind === HostedKindMarkdown) {
    if (text === undefined) return <Spinner label="Loading file" />
    return (
      <MarkdownView path={path} text={text} source={source} resolver={resolver} heading={heading} />
    )
  }
  if (file.kind === HostedKindMermaid) {
    if (text === undefined) return <Spinner label="Loading file" />
    return <MermaidView text={text} source={source} />
  }
  if (file.kind === HostedKindCode || file.kind === HostedKindText) {
    if (text === undefined) return <Spinner label="Loading file" />
    return (
      <CodeView
        text={text}
        language={file.kind === HostedKindCode ? languageOf(path) : undefined}
        line={line}
      />
    )
  }
  return (
    <Text p="5" textAlign="center" color="text.muted">
      No preview yet
    </Text>
  )
}
