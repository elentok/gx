import { Spinner } from "@chakra-ui/react"
import { useEffect, useRef, useState, type ComponentProps } from "react"
import { Button } from "../ui/button/Button"
import { useWide } from "../useWide"
import { escOpensInterrupt } from "./escOpensInterrupt"
import { InterruptPopover } from "./InterruptPopover"

// The Interrupt button, its confirm and the confirm's open state. Rendered only while the composer is Working.
export function InterruptControl({
  connected,
  pending,
  onInterrupt,
  buttonProps,
}: {
  connected: boolean
  /** A write is in flight. */
  pending: boolean
  /** Runs once the confirm is accepted; only called while connected. */
  onInterrupt: () => Promise<unknown>
  /** Layout props for the button (the Design composer's slot). */
  buttonProps?: Partial<Omit<ComponentProps<typeof Button>, "ref">>
}) {
  const [open, setOpen] = useState<false | "click" | "key">(false)
  const ref = useRef<HTMLButtonElement>(null)
  const wide = useWide()
  const disabled = !connected || pending
  useEffect(() => {
    if (!wide || disabled) return
    // Capture phase, so we judge what is on top before this same Escape closes it. For the same reason a
    // `defaultPrevented` check would always read false here: a field that uses Esc to cancel an edit is
    // found through escOpensInterrupt's layer and DOM checks instead.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || open) return
      if (escOpensInterrupt(e.target)) setOpen("key")
    }
    window.addEventListener("keydown", onKey, true)
    return () => window.removeEventListener("keydown", onKey, true)
  }, [wide, disabled, open])
  return (
    <>
      <Button
        ref={ref}
        emphasis="secondary"
        {...buttonProps}
        disabled={disabled}
        title={wide ? "Interrupt (Esc)" : undefined}
        aria-expanded={open !== false}
        onClick={() => setOpen(open ? false : "click")}
      >
        {pending && <Spinner size="sm" />}
        Interrupt
      </Button>
      <InterruptPopover
        open={open !== false}
        initialFocus={open === "key" ? "confirm" : "cancel"}
        disabled={disabled}
        anchorRef={ref}
        onConfirm={() => {
          setOpen(false)
          return connected ? onInterrupt() : Promise.resolve()
        }}
        onClose={() => setOpen(false)}
      />
    </>
  )
}
