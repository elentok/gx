import { Box } from "@chakra-ui/react"
import { useState } from "react"
import { MAX_IMAGES, MSG_MAX_IMAGES } from "./imageLimits"
import { ImageStrip } from "./ImageStrip"
import { ImageViewer } from "./ImageViewer"
import type { DraftImages as State } from "./useDraftImages"

function lineText(s: State): { text: string; red: boolean } | undefined {
  if (s.error === MSG_MAX_IMAGES) return { text: MSG_MAX_IMAGES, red: true }
  if (s.failed)
    return {
      text:
        s.failed === 1
          ? "1 image failed. Remove it or retry before starting."
          : `${s.failed} images failed. Remove them or retry before starting.`,
      red: true,
    }
  if (s.uploading) return { text: `${s.uploading} uploading`, red: false }
  if (s.error) return { text: s.error, red: true }
  if (s.images.length) return { text: `${s.count} of ${MAX_IMAGES} images`, red: false }
  return undefined
}

/**
 * The thumbnail row and the status line under it. Every device has the footer paperclip, so the row shows only with
 * images and the line only with something to say.
 * Paste and the file input are wired by the form, not here.
 */
export function DraftImages({ state }: { state: State }) {
  const [viewing, setViewing] = useState<number>()
  const line = lineText(state)
  // The viewer lists ready images only, so a thumbnail's index shifts past the others.
  const open = (i: number) =>
    setViewing(state.images.slice(0, i).filter((x) => x.state === "ready").length)
  return (
    <>
      {state.images.length > 0 && (
        <ImageStrip
          images={state.images}
          onOpen={open}
          onRetry={state.retry}
          onRemove={state.remove}
        />
      )}
      {line && (
        <Box
          pl="0.5"
          pb="2"
          fontSize="12px"
          color={line.red ? "tone.deadlocked.fg" : "text.dim"}
          flexShrink="0"
          data-mock-part="image-line"
        >
          {line.text}
        </Box>
      )}
      {viewing !== undefined && (
        <ImageViewer
          images={state.readyIds.map((id) => ({ id }))}
          source={(img) => state.urlOf(img.id ?? "")}
          start={viewing}
          onClose={() => setViewing(undefined)}
        />
      )}
    </>
  )
}
