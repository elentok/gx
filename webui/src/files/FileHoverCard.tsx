import { chakra, HoverCard, Portal } from "@chakra-ui/react"
import { type ReactElement, useState } from "react"
import type { PixelSize } from "./FileHoverCardMedia"
import { usePreviewMode } from "./FileLink"
import { FilePreviewBody, FilePreviewHeader } from "./FilePreviewBody"
import { useFileLinks } from "./FileLinkContext"
import { viewerHref } from "./viewerUrl"

/** The hover card around a file link. Touch gets none: the sheet from the phone ticket takes over. */
export function FileHoverCard({
  path,
  line,
  heading,
  children,
}: {
  path: string
  line?: number
  heading?: string
  children: ReactElement
}) {
  const ctx = useFileLinks()
  const [pixels, setPixels] = useState<PixelSize>()
  const mode = usePreviewMode(path)
  const file = ctx?.byPath(path)
  if (mode !== "card" || !ctx || !file) return children
  return (
    <HoverCard.Root
      openDelay={100}
      closeDelay={100}
      positioning={{ placement: "bottom-start" }}
      onOpenChange={(e) => {
        if (e.open) void ctx.recheck(path)
      }}
    >
      <HoverCard.Trigger asChild>{children}</HoverCard.Trigger>
      <Portal>
        <HoverCard.Positioner>
          <HoverCard.Content
            w="380px"
            maxW="calc(100vw - 16px)"
            p="0"
            overflow="hidden"
            bg="surface.raised"
            color="text"
            borderWidth="1px"
            borderColor="border"
            borderRadius="10px"
            boxShadow="lg"
            data-file-hover-card=""
          >
            <FilePreviewHeader
              path={path}
              file={file}
              line={line}
              pixels={pixels}
              missing={ctx.isMissing(path)}
            />
            <FilePreviewBody
              path={path}
              file={file}
              missing={ctx.isMissing(path)}
              line={line}
              onPixelSize={setPixels}
            />
            <chakra.div px="3" py="2" borderTopWidth="1px" textAlign="end" fontSize="sm">
              <chakra.a
                href={viewerHref(path, { line, heading })}
                data-file-open=""
                color="tone.done.fg"
                textDecoration="underline"
              >
                Open
              </chakra.a>
            </chakra.div>
          </HoverCard.Content>
        </HoverCard.Positioner>
      </Portal>
    </HoverCard.Root>
  )
}
