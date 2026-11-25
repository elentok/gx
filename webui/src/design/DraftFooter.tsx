import { Box, Spinner, VisuallyHidden } from "@chakra-ui/react"
import { useId, type MouseEvent } from "react"
import { LuTrash2 } from "react-icons/lu"
import { Button } from "../ui/button/Button"
import { TipPopup } from "../ui/info-tip/TipPopup"
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
}

// `aria-disabled` keeps hover and tap alive, which the Button's `disabled` style would kill.
const blockedLook = { "&[aria-disabled=true]": { opacity: 0.5, cursor: "default" } }

/** The draft page's action bar: attach, remove and Start. */
export function DraftFooter({
  startLabel = "Start design",
  canAttach,
  startReason,
  startDisabled,
  pending,
  onAttach,
  onRemove,
}: Props) {
  const reasonId = useId()
  const reason = startDisabled ? startReason : undefined
  const start = (tip?: object) => (
    <Button
      type="submit"
      tone="running"
      aria-disabled={startDisabled || undefined}
      aria-describedby={reason ? reasonId : undefined}
      onClick={(e) => {
        if (startDisabled) e.preventDefault()
      }}
      whiteSpace="nowrap"
      flexShrink={0}
      css={blockedLook}
      {...tip}
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
          <TipPopup content={reason}>
            {(tip) =>
              start({
                ...tip,
                onClick: (e: MouseEvent) => {
                  e.preventDefault()
                  tip.onClick()
                },
              })
            }
          </TipPopup>
        </>
      ) : (
        start()
      )}
    </Box>
  )
}
