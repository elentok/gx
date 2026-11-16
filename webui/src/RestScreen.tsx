import { Box, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { createPortal } from "react-dom"
import { RestKindBedtime } from "./generated/server"
import { useRestState } from "./rest"

// Above the modal token (1400) and the toast layer, so no sheet or modal shows over it.
const REST_Z_INDEX = 2000

// The screen is always dark, whatever theme the person chose.
const PAGE = "#0a0d11"
const FG = "#e6edf3"
const DIM = "#8b98a5"
const ACCENT = "#4da3ff"

const MASK = "radial-gradient(circle at center, #000 52%, transparent 71%)"

// Everything else in the body goes inert, which covers the app and any modal portalled to the body.
function useInertPage(screen: HTMLElement | null) {
  useEffect(() => {
    if (!screen) return
    const inerted: Element[] = []
    for (const el of Array.from(document.body.children)) {
      if (el === screen || el.hasAttribute("inert")) continue
      el.setAttribute("inert", "")
      inerted.push(el)
    }
    return () => inerted.forEach((el) => el.removeAttribute("inert"))
  }, [screen])
}

// Capture phase on the document, so a shortcut handler underneath never sees the key. Text fields
// on the screen itself keep working.
function useKeyGuard(screen: HTMLElement | null) {
  useEffect(() => {
    if (!screen) return
    const guard = (e: KeyboardEvent) => {
      if (e.target instanceof Node && screen.contains(e.target)) return
      e.preventDefault()
      e.stopPropagation()
    }
    document.addEventListener("keydown", guard, true)
    return () => document.removeEventListener("keydown", guard, true)
  }, [screen])
}

function BedtimeView() {
  const { period, rule } = useRestState()
  if (!period) return null
  return (
    <>
      <Text data-part="message" fontSize="22px" lineHeight="1.3" fontWeight="600" m="0 0 10px">
        {period.message}
      </Text>
      {period.sub_message && (
        <Text data-part="submessage" color={DIM} m="0">
          {period.sub_message}
        </Text>
      )}
      <Text
        data-part="time-label"
        mt="16px"
        fontSize="12px"
        textTransform="uppercase"
        letterSpacing="1.5px"
        color={DIM}
      >
        Back at
      </Text>
      <Text
        data-part="time"
        m="6px 0"
        fontFamily="ui-monospace, monospace"
        fontWeight="700"
        fontSize="40px"
        lineHeight="1"
        color={ACCENT}
      >
        {rule?.bedtime_end}
      </Text>
      {rule && (
        <Text data-part="rule-note" color={DIM} fontSize="12px" m="0">
          Bedtime {rule.bedtime_start} – {rule.bedtime_end}
        </Text>
      )}
    </>
  )
}

export function RestScreen() {
  const { period } = useRestState()
  const [screen, setScreen] = useState<HTMLElement | null>(null)
  useInertPage(screen)
  useKeyGuard(screen)
  if (!period) return null
  return createPortal(
    <Box
      ref={setScreen}
      role="dialog"
      aria-modal="true"
      aria-label="Rest"
      position="fixed"
      inset="0"
      zIndex={REST_Z_INDEX}
      display="flex"
      alignItems="center"
      justifyContent="center"
      p="24px"
      bg={PAGE}
      color={FG}
      overflowY="auto"
    >
      <Box display="flex" flexDirection="column" alignItems="center" textAlign="center">
        <Box
          data-part="mascot-image"
          w={{ base: "260px", md: "300px" }}
          h={{ base: "260px", md: "300px" }}
          flex="none"
          backgroundImage="url(/gx-rest-bedtime.png)"
          backgroundSize="cover"
          backgroundPosition="center"
          style={{ maskImage: MASK, WebkitMaskImage: MASK }}
        />
        {period.kind === RestKindBedtime && <BedtimeView />}
        <Box data-part="unlock" mt="22px">
          <Text
            as="a"
            data-part="unlock-link"
            color={DIM}
            fontSize="13px"
            textDecoration="underline"
            cursor="pointer"
          >
            Unlock anyway
          </Text>
        </Box>
      </Box>
    </Box>,
    document.body,
  )
}
