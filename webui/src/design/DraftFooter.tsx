import { Box, Spinner } from "@chakra-ui/react"
import type { ReactNode } from "react"
import { Button } from "../ui/button/Button"
import { AttachButton } from "./ComposerImages"

interface Props {
  canAttach: boolean
  /** The Start blocker, shown beside Start on a wide screen; a sheet shows it above the footer. */
  reasonText: ReactNode
  startDisabled: boolean
  pending: boolean
  onAttach: () => void
  /** Runs the registry Remove; its confirm opens on the button. */
  onRemove: (button: HTMLElement) => void
  /** The Start button's text; "Start design" by default. */
  startLabel?: string
}

/** The draft page's action bar: attach, remove and Start. */
export function DraftFooter({
  startLabel = "Start design",
  canAttach,
  reasonText,
  startDisabled,
  pending,
  onAttach,
  onRemove,
}: Props) {
  return (
    <Box display="flex" alignItems="center" gap="2" pb="1" flexShrink="0">
      {canAttach && <AttachButton onClick={onAttach} />}
      <Button type="button" emphasis="secondary" onClick={(e) => onRemove(e.currentTarget)}>
        Remove
      </Button>
      <Box flex="1" />
      {reasonText}
      <Button type="submit" tone="running" disabled={startDisabled}>
        {pending && <Spinner size="sm" />}
        {startLabel}
      </Button>
    </Box>
  )
}
