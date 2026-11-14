import { chakra, Flex, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import type { HostedResolved } from "../generated/ops"
import { sizeText } from "../health/formatBytes"
import { FileHoverCardMedia, type PixelSize } from "./FileHoverCardMedia"
import { FileHoverCardText } from "./FileHoverCardText"
import { previewVerdict } from "./viewerBody"

export function FilePreviewHeader({
  path,
  file,
  line,
  pixels,
  missing,
}: {
  path: string
  file?: HostedResolved
  line?: number
  pixels?: PixelSize
  missing?: boolean
}) {
  return (
    <Flex gap="3" justify="space-between" align="baseline" px="3" py="2" borderBottomWidth="1px">
      <Flex fontFamily="mono" fontSize="xs" minW="0" title={path}>
        <Text truncate>{path}</Text>
        {line ? <Text flexShrink={0}>:{line}</Text> : null}
      </Flex>
      {!missing && file?.size !== undefined && (
        <Text fontSize="xs" color="text.muted" flexShrink={0}>
          {sizeText(file.size, "binary")}
          {pixels ? ` · ${pixels.w}×${pixels.h}` : ""}
        </Text>
      )}
    </Flex>
  )
}

/** The dashed centred box the mock uses for a body with nothing to render. */
function Placeholder({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Flex
      h="100%"
      borderWidth="1px"
      borderStyle="dashed"
      borderRadius="md"
      align="center"
      justify="center"
      textAlign="center"
      p="2"
      fontSize="sm"
      color="text.muted"
    >
      <chakra.div>
        <chakra.b color="text">{title}</chakra.b>
        <br />
        {children}
      </chakra.div>
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
  const { body } = previewVerdict(file, missing)
  return (
    <chakra.div
      data-preview-body={body}
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
      {body === "missing" && (
        <Placeholder title="File not found">It may have been deleted or moved.</Placeholder>
      )}
      {body === "html" && (
        <Placeholder title="HTML page">
          No live preview. Pages run scripts, so they open only in the viewer.
          <br />
          <chakra.span color="tone.waiting.fg">Sandboxed</chakra.span>
        </Placeholder>
      )}
      {body === "note" && <Note>No preview</Note>}
      {(body === "markdown" || body === "code" || body === "text") && (
        <FileHoverCardText path={path} file={file} line={line} />
      )}
      {(body === "mermaid" || body === "image") && (
        <FileHoverCardMedia path={path} file={file} onPixelSize={onPixelSize} />
      )}
    </chakra.div>
  )
}
