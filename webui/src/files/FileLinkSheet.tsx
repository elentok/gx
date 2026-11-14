import { Button, Flex } from "@chakra-ui/react"
import { useState } from "react"
import { copyText } from "../clipboard"
import { BottomSheet } from "../ui/modal/BottomSheet"
import { showToast } from "../ui/toaster"
import type { PixelSize } from "./FileHoverCardMedia"
import { useFileLinks } from "./FileLinkContext"
import { FilePreviewBody, FilePreviewHeader } from "./FilePreviewBody"
import { viewerHref, viewerLink } from "./viewerUrl"

/** The phone's file link sheet: the card's header and body, with Open and Copy link. */
export function FileLinkSheet({
  path,
  line,
  heading,
  onClose,
}: {
  path: string
  line?: number
  heading?: string
  onClose: () => void
}) {
  const ctx = useFileLinks()
  const [pixels, setPixels] = useState<PixelSize>()
  const file = ctx?.byPath(path)
  if (!ctx || !file) return null
  const copy = async () => {
    const ok = await copyText(viewerLink(path, { line, heading }), null)
    showToast(
      ok ? { type: "info", title: "Link copied" } : { type: "error", title: "Could not copy" },
    )
  }
  return (
    <BottomSheet label={path} onClose={onClose} data-file-sheet="" bg="surface.raised" color="text">
      <FilePreviewHeader path={path} file={file} line={line} pixels={pixels} />
      <FilePreviewBody
        path={path}
        file={file}
        missing={ctx.isMissing(path)}
        line={line}
        onPixelSize={setPixels}
      />
      <Flex gap="3" p="3" borderTopWidth="1px">
        <Button asChild flex="1" colorPalette="blue" variant="solid">
          <a href={viewerHref(path, { line, heading })} data-file-open="" onClick={onClose}>
            Open
          </a>
        </Button>
        <Button flex="1" variant="outline" color="text" onClick={() => void copy()}>
          Copy link
        </Button>
      </Flex>
    </BottomSheet>
  )
}
