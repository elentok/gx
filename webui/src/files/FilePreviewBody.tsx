import { chakra, Flex, Text } from "@chakra-ui/react"
import {
  type HostedResolved,
  HostedKindBinary,
  HostedKindCode,
  HostedKindHTML,
  HostedKindImage,
  HostedKindMarkdown,
  HostedKindMermaid,
  HostedKindPDF,
  HostedKindSVG,
  HostedKindText,
} from "../generated/ops"
import { sizeText } from "../health/formatBytes"
import { FileHoverCardMedia, type PixelSize } from "./FileHoverCardMedia"
import { FileHoverCardText } from "./FileHoverCardText"

export const PREVIEW_MAX_BYTES = 2 * 1024 * 1024

export type PreviewState = "missing" | "html" | "none" | "other"

/** Which body a file gets. "other" is for the text and media bodies that later tickets add. */
export function previewState(file: HostedResolved, missing: boolean): PreviewState {
  if (missing) return "missing"
  if (file.kind === HostedKindHTML) return "html"
  if (file.kind === HostedKindPDF || file.kind === HostedKindBinary) return "none"
  if ((file.size ?? 0) > PREVIEW_MAX_BYTES) return "none"
  return "other"
}

function isTextPreview(file: HostedResolved): boolean {
  return (
    file.kind === HostedKindMarkdown || file.kind === HostedKindCode || file.kind === HostedKindText
  )
}

function isMediaPreview(file: HostedResolved): boolean {
  return (
    file.kind === HostedKindMermaid || file.kind === HostedKindImage || file.kind === HostedKindSVG
  )
}

export function FilePreviewHeader({
  path,
  file,
  line,
  pixels,
}: {
  path: string
  file?: HostedResolved
  line?: number
  pixels?: PixelSize
}) {
  return (
    <Flex gap="3" justify="space-between" align="baseline" px="3" py="2" borderBottomWidth="1px">
      <Flex fontFamily="mono" fontSize="xs" minW="0" title={path}>
        <Text truncate>{path}</Text>
        {line ? <Text flexShrink={0}>:{line}</Text> : null}
      </Flex>
      {file?.size !== undefined && (
        <Text fontSize="xs" color="text.muted" flexShrink={0}>
          {sizeText(file.size, "binary")}
          {pixels ? ` · ${pixels.w}×${pixels.h}` : ""}
        </Text>
      )}
    </Flex>
  )
}

function Note({ children }: { children: string }) {
  return (
    <Text fontSize="sm" color="text.muted">
      {children}
    </Text>
  )
}

/** The fixed 170px preview area with a fade at the bottom edge. The hover card and the sheet share it. */
export function FilePreviewBody({
  path,
  file,
  missing,
  line,
  onPixelSize,
}: {
  path: string
  file: HostedResolved
  missing: boolean
  line?: number
  onPixelSize?: (size: PixelSize) => void
}) {
  const state = previewState(file, missing)
  return (
    <chakra.div
      data-preview-body={state}
      position="relative"
      h="170px"
      overflow="hidden"
      p="3"
      _after={{
        content: '""',
        position: "absolute",
        insetInline: 0,
        bottom: 0,
        height: "28px",
        pointerEvents: "none",
        background: "linear-gradient(to bottom, transparent, {colors.surface.raised})",
      }}
    >
      {state === "missing" && <Note>File not found. It may have been deleted or moved.</Note>}
      {state === "html" && (
        <Flex direction="column" gap="2" align="flex-start">
          <Note>
            HTML page. No live preview. Pages run scripts, so they open only in the viewer.
          </Note>
          <chakra.span
            fontSize="xs"
            borderWidth="1px"
            borderRadius="full"
            px="2"
            color="text.muted"
          >
            Sandboxed
          </chakra.span>
        </Flex>
      )}
      {state === "none" && <Note>No preview</Note>}
      {state === "other" && isTextPreview(file) && (
        <FileHoverCardText path={path} file={file} line={line} />
      )}
      {state === "other" && isMediaPreview(file) && (
        <FileHoverCardMedia path={path} file={file} onPixelSize={onPixelSize} />
      )}
    </chakra.div>
  )
}
