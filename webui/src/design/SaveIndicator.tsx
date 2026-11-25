import { Box, VisuallyHidden } from "@chakra-ui/react"
import { useEffect, useRef, useState } from "react"
import { LuCheck, LuSave, LuX } from "react-icons/lu"

/** "restored" is "saved" without a save the person saw: a history pick or load-theirs. It shows nothing. */
export type SaveStatus = "none" | "saving" | "saved" | "unsaved" | "restored"

const SHOW_AFTER_MS = 300
const MIN_SHOWN_MS = 600
const CHECK_MS = 1000

const LABELS: Record<Exclude<SaveStatus, "restored">, string> = {
  none: "",
  saving: "Saving the draft",
  saved: "Draft saved",
  unsaved: "Not saved, retrying",
}

/**
 * A floppy disk that shows only while a save request runs. It owns all timing: a save shorter than
 * `SHOW_AFTER_MS` shows nothing, a shown icon stays `MIN_SHOWN_MS`, and a seen save ends with a check.
 */
export function SaveIndicator({ status }: { status: SaveStatus }) {
  const [shown, setShown] = useState<Exclude<SaveStatus, "restored">>("none")
  // When the saving or error icon appeared (0 when none has), kept out of render state so the
  // status effect below reads the latest value without re-running.
  const shownAt = useRef(0)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const prev = useRef<SaveStatus>(status)

  useEffect(() => {
    const was = prev.current
    prev.current = status
    clearTimeout(timer.current)
    const later = (ms: number, fn: () => void) => {
      timer.current = setTimeout(fn, ms)
    }
    const showCheck = () => {
      shownAt.current = 0
      setShown("saved")
      later(CHECK_MS, () => setShown("none"))
    }
    const seen = shownAt.current !== 0
    if (status === "unsaved") {
      shownAt.current = Date.now()
      setShown("unsaved")
    } else if (status === "saving") {
      // A retry from "unsaved" never gets here: the hooks keep "unsaved" until a save works.
      later(SHOW_AFTER_MS, () => {
        shownAt.current = Date.now()
        setShown("saving")
      })
    } else if (status === "saved" && (seen || was === "unsaved")) {
      const left = MIN_SHOWN_MS - (Date.now() - shownAt.current)
      if (was === "unsaved" || left <= 0) showCheck()
      else later(left, showCheck)
    } else {
      shownAt.current = 0
      setShown("none")
    }
  }, [status])

  useEffect(() => () => clearTimeout(timer.current), [])

  const Badge = shown === "unsaved" ? LuX : LuCheck
  return (
    <Box
      role="status"
      aria-live="polite"
      position="relative"
      w="20px"
      h="20px"
      flexShrink="0"
      color={shown === "unsaved" ? "tone.error.fg" : "text.dim"}
      data-save-shown={shown}
    >
      <VisuallyHidden>{LABELS[shown]}</VisuallyHidden>
      {shown !== "none" && (
        <Box
          aria-hidden
          position="absolute"
          inset="0"
          animation="saveFadeIn 150ms ease-out"
          css={{
            "@media (prefers-reduced-motion: reduce)": { animation: "none" },
          }}
        >
          <LuSave size={18} style={{ margin: 1 }} />
          {shown !== "saving" && (
            <Box
              position="absolute"
              right="-2px"
              bottom="-2px"
              color={shown === "saved" ? "tone.done.fg" : "tone.error.fg"}
            >
              <Badge size={11} strokeWidth={3} />
            </Box>
          )}
        </Box>
      )}
    </Box>
  )
}
