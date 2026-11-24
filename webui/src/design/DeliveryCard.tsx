import { Box, Text } from "@chakra-ui/react"
import { useRef, useState } from "react"
import { copyText } from "../clipboard"
import {
  type Delivery,
  DeliveryMaybeSent,
  DeliverySendFailed,
  DeliveryUnknownCommand,
  type SessionInfo,
  type SessionRequest,
  type SessionResult,
} from "../generated/server"
import { Button } from "../ui/button/Button"
import { refusalText, useWrite } from "../useWrite"
import { isBlockingCard, postRetry, type PostRetry } from "./BlockingCard"
import {
  postDismissSession,
  postResumeSession,
  postStopSession,
  rowToken,
  type ResumeSession,
  type StopSession,
} from "./open"
import type { FetchScreen } from "./screen"
import { ScreenModal } from "./ScreenModal"
import { promptable } from "./view"

type SessionWrite = (body: SessionRequest) => Promise<SessionResult>

/** What a button's `run` can reach: the card's own state and its writes. */
interface CardActions {
  session: SessionInfo
  write: ReturnType<typeof useWrite>["run"]
  dismiss: SessionWrite
  resend: PostRetry
  onDone?: () => void
  showScreen: () => void
  confirmRestart: () => void
}

interface CardButton {
  label: string
  run: (a: CardActions) => void
}

const ROW_BUTTON = {
  emphasis: "secondary",
  h: "28px",
  px: "10px",
  fontSize: "14px",
  fontWeight: "400",
  borderRadius: "4px",
} as const

const RESEND: CardButton = {
  label: "Resend",
  run: (a) => a.write(() => a.resend({ id: a.session.id })),
}
const RESTART: CardButton = { label: "Restart agent", run: (a) => a.confirmRestart() }
const SHOW_SCREEN: CardButton = { label: "Show screen", run: (a) => a.showScreen() }
const DISMISS: CardButton = {
  label: "Dismiss",
  run: (a) => a.write(() => a.dismiss({ id: a.session.id }), { onDone: a.onDone }),
}

interface KindLook {
  title: string
  color: string
  buttons: CardButton[]
}

/** One entry per delivery kind; a new kind adds its title, colour and buttons here. */
const KINDS: Record<Delivery, KindLook> = {
  [DeliveryMaybeSent]: {
    title: "May not have been sent",
    color: "tone.waiting.fg",
    buttons: [SHOW_SCREEN, DISMISS],
  },
  [DeliverySendFailed]: {
    title: "Send failed",
    color: "tone.deadlocked.fg",
    buttons: [SHOW_SCREEN, DISMISS],
  },
  [DeliveryUnknownCommand]: {
    title: "Send failed",
    color: "tone.deadlocked.fg",
    buttons: [RESEND, RESTART, SHOW_SCREEN, DISMISS],
  },
}

const kindLook = (kind?: string): KindLook | undefined =>
  kind && Object.hasOwn(KINDS, kind) ? KINDS[kind as Delivery] : undefined

/** Whether the session carries a gx card for a send that gave up or may not have landed. */
export const isDeliveryCard = (s: SessionInfo) =>
  !!s.actionable?.gx_raised && !!s.actionable.reason && !!kindLook(s.actionable.kind)

/** The reason split around the first copy of the command, which the mock sets in monospace. */
function reasonParts(reason: string, command: string) {
  const at = command ? reason.indexOf(command) : -1
  if (at < 0) return { before: reason, code: "", after: "" }
  return { before: reason.slice(0, at), code: command, after: reason.slice(at + command.length) }
}

/** The one card that sits at the tail of a session's chat, or null when there is none. */
export type TailCardKind = "blocking" | "delivery" | "stopped"

export function tailCard(s: SessionInfo): TailCardKind | null {
  if (isBlockingCard(s)) return "blocking"
  if (isDeliveryCard(s)) return "delivery"
  return s.actionable?.gx_raised && s.actionable.reason ? "stopped" : null
}

/** The card of a send that did not land: why, the text as sent with Copy, and Show screen and Dismiss. */
export function DeliveryCard({
  session,
  dismiss = postDismissSession,
  resend = postRetry,
  stop = postStopSession,
  resume = postResumeSession,
  fetchScreen,
  onDone,
}: {
  session: SessionInfo
  dismiss?: SessionWrite
  resend?: PostRetry
  stop?: StopSession
  resume?: ResumeSession
  fetchScreen?: FetchScreen
  onDone?: () => void
}) {
  const look = kindLook(session.actionable.kind) as KindLook
  const text = session.actionable.copy_text ?? ""
  const reason = reasonParts(session.actionable.reason ?? "", text)
  const box = useRef<HTMLDivElement>(null)
  const [screen, setScreen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const { run, pending, refusal, error } = useWrite({ toastRefusal: false })
  const problem = refusal ? refusalText(refusal) : error
  const restart = async () => {
    setConfirming(false)
    const stopped = await stop({ id: session.id, state: rowToken(session) })
    return stopped.refused ? stopped : resume({ id: session.id })
  }
  const actions: CardActions = {
    session,
    write: run,
    dismiss,
    resend,
    onDone,
    showScreen: () => setScreen(true),
    confirmRestart: () => setConfirming(true),
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
        {reason.before}
        {reason.code && (
          <Text as="code" fontFamily="mono" fontSize="12.5px">
            {reason.code}
          </Text>
        )}
        {reason.after}
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
          <Button {...ROW_BUTTON} onClick={() => void copyText(text, box.current)}>
            Copy
          </Button>
        </Box>
      )}
      {problem && <Text role="alert">{problem}</Text>}
      <Box data-mock-part="buttons" display="flex" gap="2" flexWrap="wrap" alignItems="center">
        {look.buttons.map((button) =>
          button === RESTART && confirming ? (
            <Box key={button.label} display="flex" alignItems="center" gap="2">
              <Text fontSize="sm">Restart the agent?</Text>
              <Button {...ROW_BUTTON} onClick={() => setConfirming(false)}>
                Keep
              </Button>
              <Button {...ROW_BUTTON} onClick={() => run(restart)}>
                Restart
              </Button>
            </Box>
          ) : (
            <Button
              key={button.label}
              {...ROW_BUTTON}
              disabled={button === RESEND && (pending || !promptable(session))}
              onClick={() => button.run(actions)}
            >
              {button.label}
            </Button>
          ),
        )}
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
