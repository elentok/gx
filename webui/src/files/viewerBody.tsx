import { Box, Text } from "@chakra-ui/react"
import {
  HostedKindBinary,
  HostedKindCode,
  HostedKindHTML,
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
import { HtmlView } from "./HtmlView"
import { languageOf } from "./language"
import { MarkdownView } from "./MarkdownView"
import { MermaidView } from "./MermaidView"
import { rawUrl } from "./rawUrl"
import type { FileResolver } from "./resolve"

/** Files over this size show no preview. */
export const MAX_PREVIEW_BYTES = 2 * 1024 * 1024

export type Note = { title?: string; text: string }

const TOO_LARGE: Note = {
  title: "Too large to show",
  text: "Files over 2 MB show no preview. Use Raw or Download.",
}

/** What body a file gets. The viewer and the hover card both render from this. */
export type PreviewVerdict =
  | { body: "missing" }
  | { body: "note"; note: Note }
  | { body: "html" }
  | { body: "image" }
  | { body: "markdown" }
  | { body: "mermaid" }
  | { body: "code" }
  | { body: "text" }

export function previewVerdict(
  file: Pick<HostedResolved, "kind" | "size">,
  missing: boolean,
): PreviewVerdict {
  if (missing) return { body: "missing" }
  if (file.kind === HostedKindPDF) {
    return {
      body: "note",
      note: { text: "PDFs open in the browser's PDF viewer. Use Raw or Download." },
    }
  }
  if ((file.size ?? 0) > MAX_PREVIEW_BYTES) return { body: "note", note: TOO_LARGE }
  switch (file.kind) {
    case HostedKindImage:
    case HostedKindSVG:
      return { body: "image" }
    case HostedKindMarkdown:
      return { body: "markdown" }
    case HostedKindHTML:
      return { body: "html" }
    case HostedKindMermaid:
      return { body: "mermaid" }
    case HostedKindCode:
      return { body: "code" }
    case HostedKindText:
      return { body: "text" }
    case HostedKindBinary:
    case undefined:
      return { body: "note", note: { text: "No preview for this file type. Use Raw or Download." } }
  }
}

/** The note that replaces the preview, or undefined when a kind body renders. */
export function previewNote(file: Pick<HostedResolved, "kind" | "size">): Note | undefined {
  const verdict = previewVerdict(file, false)
  return verdict.body === "note" ? verdict.note : undefined
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

/** Renders the body that `previewVerdict` picks for the resolve `kind`. */
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
  const verdict = previewVerdict(file, false)
  switch (verdict.body) {
    case "missing":
      return null
    case "note":
      return <NoteBody note={verdict.note} />
    case "image":
      // Always an <img>, even for SVG: scripts in an image never run.
      return (
        <Box p="4" textAlign="center">
          <img
            src={rawUrl(path)}
            alt={path.split("/").pop()}
            style={{ maxWidth: "100%", height: "auto", borderRadius: "6px" }}
            onLoad={(e) =>
              onImageLoad?.(e.currentTarget.naturalWidth, e.currentTarget.naturalHeight)
            }
            onError={() => onImageError?.()}
          />
        </Box>
      )
    case "html":
      return <HtmlView path={path} text={text} source={source} />
    case "markdown":
      if (text === undefined) return <Spinner label="Loading file" />
      return (
        <MarkdownView
          path={path}
          text={text}
          source={source}
          resolver={resolver}
          heading={heading}
        />
      )
    case "mermaid":
      if (text === undefined) return <Spinner label="Loading file" />
      return <MermaidView text={text} source={source} />
    case "code":
    case "text":
      if (text === undefined) return <Spinner label="Loading file" />
      return (
        <CodeView
          text={text}
          language={verdict.body === "code" ? languageOf(path) : undefined}
          line={line}
        />
      )
  }
}
