import { chakra, HoverCard, Portal } from "@chakra-ui/react"
import type { ReactElement } from "react"
import { useCoarsePointer } from "../useWide"
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
  const coarse = useCoarsePointer()
  const file = ctx?.byPath(path)
  if (coarse || !ctx || !file) return children
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
          <HoverCard.Content w="380px" p="0" overflow="hidden" data-file-hover-card="">
            <FilePreviewHeader path={path} file={file} />
            <FilePreviewBody file={file} missing={ctx.isMissing(path)} />
            <chakra.div px="3" py="2" borderTopWidth="1px" textAlign="end" fontSize="sm">
              <chakra.a href={viewerHref(path, { line, heading })} data-file-open="">
                Open
              </chakra.a>
            </chakra.div>
          </HoverCard.Content>
        </HoverCard.Positioner>
      </Portal>
    </HoverCard.Root>
  )
}
