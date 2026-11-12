import { Box, Spinner } from "@chakra-ui/react"
import type { ReactNode } from "react"
import type { ComposerImage } from "./useComposerImages"

function Thumb({
  image,
  readOnly,
  onOpen,
  onRetry,
  onRemove,
}: {
  image: ComposerImage
  readOnly: boolean
  onOpen?: () => void
  onRetry: () => void
  onRemove: () => void
}) {
  // A read-only strip shows what is saved, so it has no upload states.
  const state = readOnly ? "ready" : image.state
  const failed = state === "failed"
  const retry = failed && image.canRetry
  const openable = state === "ready" && !!onOpen
  return (
    <Box position="relative" flex="none" w="72px" h="72px" data-state={state}>
      <Box
        as={openable ? "button" : "div"}
        {...(openable ? { type: "button" } : {})}
        w="full"
        h="full"
        borderRadius="6px"
        bg="surface.raised"
        borderWidth="1px"
        borderColor="border"
        bgImage={image.url ? `url(${image.url})` : undefined}
        bgSize="cover"
        bgPos="center"
        cursor={openable ? "pointer" : undefined}
        aria-label={openable ? "Open image" : undefined}
        onClick={openable ? onOpen : undefined}
      />
      {state === "uploading" && (
        <Box
          position="absolute"
          inset="0"
          borderRadius="6px"
          bg="blackAlpha.600"
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          <Spinner size="sm" aria-label="Uploading" />
        </Box>
      )}
      {failed && (
        <Box
          as={retry ? "button" : "div"}
          {...(retry ? { type: "button" } : {})}
          position="absolute"
          inset="0"
          p="1"
          pt="16px"
          overflow="hidden"
          borderRadius="6px"
          borderWidth="1px"
          borderColor="tone.deadlocked.fg"
          bg="tone.deadlocked.bg"
          color="tone.deadlocked.fg"
          fontSize="10px"
          lineHeight="1.2"
          textAlign="center"
          display="flex"
          alignItems="center"
          justifyContent="center"
          cursor={retry ? "pointer" : "default"}
          onClick={retry ? onRetry : undefined}
        >
          {retry ? `${image.message}, tap to retry` : image.message}
        </Box>
      )}
      {!readOnly && (
        <Box
          as="button"
          {...{ type: "button" }}
          position="absolute"
          top="-7px"
          right="-7px"
          w="26px"
          h="26px"
          borderRadius="full"
          bg="surface.raised"
          borderWidth="1px"
          borderColor="border"
          fontSize="13px"
          color={failed ? "tone.deadlocked.fg" : undefined}
          cursor="pointer"
          aria-label="Remove image"
          onClick={onRemove}
        >
          ✕
        </Box>
      )}
    </Box>
  )
}

/** The scrolling row of 72px thumbnails. `readOnly` drops Remove, retry and the upload states; `onOpen` makes ready thumbnails clickable. */
export function ImageStrip({
  images,
  readOnly = false,
  onOpen,
  onRetry = () => {},
  onRemove = () => {},
  trailing,
}: {
  images: ComposerImage[]
  readOnly?: boolean
  onOpen?: (index: number) => void
  onRetry?: (key: string) => void
  onRemove?: (key: string) => void
  /** Sits after the last thumbnail in the same scrolling row. */
  trailing?: ReactNode
}) {
  return (
    // Read-only has nothing hanging outside the row, so it never needs a vertical scrollbar.
    <Box
      display="flex"
      flex="none"
      gap="10px"
      overflowX="auto"
      overflowY={readOnly ? "hidden" : undefined}
      pt="2"
      pb="1"
      pl="0.5"
      pr="1"
      aria-label="Images"
    >
      {images.map((image, i) => (
        <Thumb
          key={image.key}
          image={image}
          readOnly={readOnly}
          onOpen={onOpen && (() => onOpen(i))}
          onRetry={() => onRetry(image.key)}
          onRemove={() => onRemove(image.key)}
        />
      ))}
      {trailing}
    </Box>
  )
}
