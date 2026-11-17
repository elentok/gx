import { Box, Spinner, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import { Button } from "../ui/button/Button"
import { AttachButton } from "./ComposerImages"

interface Props {
  confirming: boolean
  canAttach: boolean
  /** The Start blocker, shown beside Start on a wide screen; a sheet shows it above the footer. */
  reasonText: ReactNode
  startDisabled: boolean
  pending: boolean
  onAttach: () => void
  onAskRemove: () => void
  onKeep: () => void
  onRemove: () => void
  /** The Start button's text; "Start design" by default. */
  startLabel?: string
  /** The remove question; the Design draft's, which mentions its images, by default. */
  removeText?: string
}

/** The draft page's action bar: attach, remove and Start, or the remove confirmation. */
export function DraftFooter({
  startLabel = "Start design",
  removeText = "Remove this draft and its images?",
  confirming,
  canAttach,
  reasonText,
  startDisabled,
  pending,
  onAttach,
  onAskRemove,
  onKeep,
  onRemove,
}: Props) {
  return (
    <Box
      display="flex"
      alignItems="center"
      gap="2"
      p="3"
      borderTopWidth="1px"
      borderColor="hairline"
      bg="surface.raised"
      flexShrink="0"
    >
      {confirming ? (
        <>
          <Text fontSize="sm" flex="1" minW="0" color="tone.paused.fg" fontWeight="600">
            {removeText}
          </Text>
          <Button type="button" emphasis="ghost" tone="muted" onClick={onKeep}>
            Keep
          </Button>
          <Button type="button" tone="paused" onClick={onRemove}>
            Remove
          </Button>
        </>
      ) : (
        <>
          {canAttach && <AttachButton onClick={onAttach} />}
          <Button type="button" emphasis="ghost" tone="muted" onClick={onAskRemove}>
            Remove
          </Button>
          <Box flex="1" />
          {reasonText}
          <Button type="submit" tone="running" disabled={startDisabled}>
            {pending && <Spinner size="sm" />}
            {startLabel}
          </Button>
        </>
      )}
    </Box>
  )
}
