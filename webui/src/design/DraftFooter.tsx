import { Box, Spinner, VisuallyHidden } from "@chakra-ui/react"
import { useId } from "react"
import { LuTrash2 } from "react-icons/lu"
import { Button } from "../ui/button/Button"
import { TipPopup, type TipTriggerProps } from "../ui/info-tip/TipPopup"
import { AttachButton } from "./ComposerImages"

interface Props {
  canAttach: boolean
  /** Why Start is blocked: the tooltip text. No reason means no tooltip. */
  startReason?: string
  startDisabled: boolean
  pending: boolean
  onAttach: () => void
  /** Runs the registry Remove; its confirm opens on the button. */
  onRemove: (button: HTMLElement) => void
  /** The Start button's text; "Start design" by default. */
  startLabel?: string
  /** Start for a page with no `<form>`: the button is then a plain button that calls this. */
  onStart?: () => void
}

// Chakra's `_disabled` also matches `aria-disabled`, and the Button sets `pointer-events: none`
// there, so a blocked Start replaces it to keep pointer events on for hover and tap.
const blockedLook = { opacity: 0.5, cursor: "default", pointerEvents: "auto" } as const

/** The draft page's action bar: attach, remove and Start. */
export function DraftFooter({
  startLabel = "Start design",
  canAttach,
  startReason,
  startDisabled,
  pending,
  onAttach,
  onRemove,
  onStart,
}: Props) {
  const reasonId = useId()
  const reason = startDisabled ? startReason : undefined
  const start = (tip?: TipTriggerProps) => (
    <Button
      type={onStart ? "button" : "submit"}
      tone="running"
      aria-disabled={startDisabled || undefined}
      aria-describedby={reason ? reasonId : undefined}
      whiteSpace="nowrap"
      flexShrink={0}
      _disabled={blockedLook}
      {...tip}
      onClick={(e) => {
        if (!startDisabled) {
          onStart?.()
          return
        }
        e.preventDefault()
        tip?.onClick()
      }}
    >
      {pending && <Spinner size="sm" />}
      {startLabel}
    </Button>
  )
  return (
    <Box display="flex" alignItems="center" gap="2" pb="1" flexShrink="0">
      {canAttach && <AttachButton onClick={onAttach} />}
      <Button
        type="button"
        emphasis="secondary"
        aria-label="Remove draft"
        title="Remove draft"
        w="40px"
        px="0"
        justifyContent="center"
        onClick={(e) => onRemove(e.currentTarget)}
      >
        <LuTrash2 />
      </Button>
      <Box flex="1" />
      {reason ? (
        <>
          <VisuallyHidden id={reasonId}>{reason}</VisuallyHidden>
          <TipPopup content={reason}>{start}</TipPopup>
        </>
      ) : (
        start()
      )}
    </Box>
  )
}
