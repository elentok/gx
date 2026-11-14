import { chakra } from "@chakra-ui/react"
import { useEffect } from "react"
import { type HostedResolved, HostedKindMermaid } from "../generated/ops"
import { useFileLinks } from "./FileLinkContext"
import { useCachedText } from "./FileHoverCardText"
import { MermaidDiagram } from "./MermaidDiagram"
import { previewCache } from "./previewCache"
import { rawUrl } from "./rawUrl"

export type PixelSize = { w: number; h: number }

/** The pixel size an image showed at, kept for the page's life so a second hover has it at once. */
export function cachedPixelSize(path: string, modified?: string): PixelSize | undefined {
  return previewCache.get<PixelSize>(`${path}\0px`, modified)
}

/** A Mermaid diagram (rendered on demand) or an image thumbnail. SVG is an `<img>` too, never inline. */
export function FileHoverCardMedia({
  path,
  file,
  onPixelSize,
}: {
  path: string
  file: HostedResolved
  onPixelSize?: (size: PixelSize) => void
}) {
  if (file.kind === HostedKindMermaid) return <MermaidBody path={path} file={file} />
  return <ImageBody path={path} file={file} onPixelSize={onPixelSize} />
}

function MermaidBody({ path, file }: { path: string; file: HostedResolved }) {
  const text = useCachedText(path, file)
  if (text === undefined) return null
  return <MermaidDiagram source={text} />
}

function ImageBody({
  path,
  file,
  onPixelSize,
}: {
  path: string
  file: HostedResolved
  onPixelSize?: (size: PixelSize) => void
}) {
  const markMissing = useFileLinks()?.markMissing
  const known = cachedPixelSize(path, file.modified)
  useEffect(() => {
    if (known) onPixelSize?.(known)
  }, [known, onPixelSize])
  return (
    <chakra.img
      src={rawUrl(path)}
      alt=""
      display="block"
      w="100%"
      h="100%"
      objectFit="contain"
      borderRadius="6px"
      onLoad={(e) => {
        const { naturalWidth: w, naturalHeight: h } = e.currentTarget
        if (!w || !h) return
        previewCache.set(`${path}\0px`, file.modified, { w, h })
        onPixelSize?.({ w, h })
      }}
      onError={() => markMissing?.(path)}
    />
  )
}
