import { Portal, Tooltip } from "@chakra-ui/react"
import { useEffect, useState, type ReactNode } from "react"

export interface TipTriggerProps {
  onClick: () => void
  onFocus: () => void
  onBlur: () => void
}

interface Props {
  /** The tooltip text. */
  content: ReactNode
  /** Renders the trigger; spread the handlers on it. */
  children: (props: TipTriggerProps) => ReactNode
}

// Controlled, because the installed Chakra Tooltip has no open-on-click prop. A tap toggles it.
export function TipPopup({ content, children }: Props) {
  const [open, setOpen] = useState(false)
  // The menu dismisses on Escape in the capture phase, so only a capture listener on window runs
  // first. While the tooltip is open, Escape closes it and goes no further.
  useEffect(() => {
    if (!open) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return
      e.stopImmediatePropagation()
      setOpen(false)
    }
    window.addEventListener("keydown", onKeyDown, true)
    return () => window.removeEventListener("keydown", onKeyDown, true)
  }, [open])
  return (
    <Tooltip.Root
      open={open}
      onOpenChange={(e) => setOpen(e.open)}
      closeOnClick={false}
      closeOnPointerDown={false}
      openDelay={0}
      closeDelay={0}
      interactive
    >
      <Tooltip.Trigger asChild>
        {children({
          onClick: () => setOpen((o) => !o),
          onFocus: () => setOpen(true),
          onBlur: () => setOpen(false),
        })}
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content
            bg="surface.raised"
            color="text"
            borderWidth="1px"
            borderColor="border"
            borderRadius="md"
            px="2"
            py="1"
            fontSize="xs"
            maxW="64"
          >
            {content}
          </Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip.Root>
  )
}
