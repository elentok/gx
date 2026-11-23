import { Box, Text } from "@chakra-ui/react"
import { useRef, useState } from "react"
import { copyText } from "../clipboard"
import {
  DeliveryMaybeSent,
  DeliverySendFailed,
  type SessionInfo,
  type SessionRequest,
  type SessionResult,
} from "../generated/server"
import { Button } from "../ui/button/Button"
import { refusalText, useWrite } from "../useWrite"
import { postDismissSession } from "./open"
import type { FetchScreen } from "./screen"
import { ScreenModal } from "./ScreenModal"

export type DismissSession = (body: SessionRequest) => Promise<SessionResult>

type ButtonName = "Show screen" | "Dismiss"

interface KindLook {
  title: string
  color: string
  buttons: ButtonName[]
}

/** One entry per delivery kind; a new kind adds its title, colour and buttons here. */
const KINDS: Record<string, KindLook> = {
  [DeliveryMaybeSent]: {
    title: "May not have been sent",
    color: "tone.waiting.fg",
    buttons: ["Show screen", "Dismiss"],
  },
  [DeliverySendFailed]: {
    title: "Send failed",
    color: "tone.deadlocked.fg",
    buttons: ["Show screen", "Dismiss"],
  },
}

/** Whether the session carries a gx card for a send that gave up or may not have landed. */
export const isDeliveryCard = (s: SessionInfo) =>
  !!s.actionable?.gx_raised &&
  !!s.actionable.reason &&
  !!s.actionable.kind &&
  s.actionable.kind in KINDS

/** The card of a send that did not land: why, the text as sent with Copy, and Show screen and Dismiss. */
export function DeliveryCard({
  session,
  dismiss = postDismissSession,
  fetchScreen,
  onDone,
}: {
  session: SessionInfo
  dismiss?: DismissSession
  fetchScreen?: FetchScreen
  onDone?: () => void
}) {
  const look = KINDS[session.actionable.kind ?? ""]
  const text = session.actionable.copy_text ?? ""
  const box = useRef<HTMLDivElement>(null)
  const [screen, setScreen] = useState(false)
  const { run, pending, refusal, error } = useWrite({ toastRefusal: false })
  const problem = refusal ? refusalText(refusal) : error
  const click = (name: ButtonName) => {
    if (name === "Show screen") setScreen(true)
    else run(() => dismiss({ id: session.id }), { onDone })
  }
  return (
    <Box
      role="region"
      aria-label={look.title}
      data-mock-part="card"
      borderWidth="1px"
      borderColor={look.color}
      borderRadius="md"
      p="2.5"
      display="flex"
      flexDirection="column"
      gap="1.5"
    >
      <Text fontWeight="700" color={look.color}>
        {look.title}
      </Text>
      <Text fontSize="sm" data-mock-part="reason">
        {session.actionable.reason}
      </Text>
      {text && (
        <Box
          data-mock-part="copy"
          display="flex"
          alignItems="center"
          gap="2"
          bg="surface"
          borderWidth="1px"
          borderColor="border"
          borderRadius="sm"
          px="2"
          py="1"
        >
          <Text
            ref={box}
            flex="1"
            minW="0"
            fontFamily="mono"
            fontSize="12.5px"
            whiteSpace="pre-wrap"
          >
            {text}
          </Text>
          <Button
            emphasis="secondary"
            fontSize="12.5px"
            onClick={() => void copyText(text, box.current)}
          >
            Copy
          </Button>
        </Box>
      )}
      {problem && <Text role="alert">{problem}</Text>}
      <Box data-mock-part="buttons" display="flex" gap="2" flexWrap="wrap">
        {look.buttons.map((name) => (
          <Button
            key={name}
            emphasis="secondary"
            disabled={name === "Dismiss" && pending}
            onClick={() => click(name)}
          >
            {name}
          </Button>
        ))}
      </Box>
      {screen && (
        <ScreenModal
          target={session.id}
          fetcher={fetchScreen}
          keys={!session.queue}
          onClose={() => setScreen(false)}
        />
      )}
    </Box>
  )
}
