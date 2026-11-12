import { Box, Spinner, Text } from "@chakra-ui/react"
import { useEffect, useRef, useState, type ComponentProps } from "react"
import { PiNotePencil } from "react-icons/pi"
import { useComposerDraft } from "../drafts"
import {
  CardSourceRound,
  type SessionInfo,
  SessionNext,
  SessionRoundCard,
} from "../generated/server"
import { Button } from "../ui/button/Button"
import { AutoAdvanceBar, AutoCountdown, type RunCall } from "./AutoAdvanceBar"
import type { AutoMenuState } from "./AutoMenu"
import {
  postAutoAdvance,
  postAutoQueue,
  postCancelAutoAdvance,
  postCancelAutoQueue,
  postInterruptSession,
  stateToken,
  type CancelAutoAdvance,
  type InterruptSession,
  type SetAutoAdvance,
  type SetAutoQueue,
} from "./open"
import { codexSkillHint } from "./chat"
import { ComposerImages } from "./ComposerImages"
import { FooterLine } from "./FooterLine"
import { InterruptControl } from "./InterruptControl"
import { useComposerImages } from "./useComposerImages"
import { PromptBox } from "../ui/expandable-text/PromptBox"
import { postAnswer, type PostAnswer, type PostPrompt } from "./prompt"
import type { Answers } from "../ui/question-sheet/round"
import { ReasonStale } from "../ticket/answer"
import { unansweredOf } from "./round"
import { SendPopover } from "./SendPopover"
import { usePromptSend } from "./usePromptSend"
import { useWide } from "../useWide"
import { promptable } from "./view"

// Under the chat, one line: the Auto menu, then Note and the main action. Nothing sends unless the session is Idle.
export function Composer({
  session,
  lastMessage,
  next,
  nextTitle,
  connected,
  post,
  answer = postAnswer,
  roundId = "",
  interrupt = postInterruptSession,
  setAutoAdvance = postAutoAdvance,
  setAutoQueue = postAutoQueue,
  cancelAutoAdvance = postCancelAutoAdvance,
  cancelAutoQueue = postCancelAutoQueue,
  onStale,
  round = [],
  answers = {},
  onRoundSent,
  onReview,
  autoMenu,
}: {
  session: SessionInfo
  /** The id of the last chat message the page saw. */
  lastMessage?: string
  next?: SessionNext
  /** Shown under Continue: the map's next ticket title, or the step label. */
  nextTitle?: string
  connected: boolean
  post?: PostPrompt
  /** Posts the open round's answers; the server words the prompt. */
  answer?: PostAnswer
  /** The server's card id of the open round. */
  roundId?: string
  interrupt?: InterruptSession
  setAutoAdvance?: SetAutoAdvance
  setAutoQueue?: SetAutoQueue
  cancelAutoAdvance?: CancelAutoAdvance
  cancelAutoQueue?: CancelAutoAdvance
  /** The conversation moved under the person: refetch the chat, keep the draft. */
  onStale?: () => void
  /** The open Grilling round: Send answers all of it in one prompt. */
  round?: SessionRoundCard[]
  answers?: Answers
  /** The round went out. */
  onRoundSent?: () => void
  /** A row of the unanswered popover: open the sheet at that question. */
  onReview?: (index: number) => void
  /** The Auto popover's open state, kept by the caller so a remount of the composer does not close it. */
  autoMenu?: AutoMenuState
}) {
  const wide = useWide()
  const [gapsOpen, setGapsOpen] = useState(false)
  const sendRef = useRef<HTMLButtonElement>(null)
  const [text, setText] = useComposerDraft(session.id)
  // Shared text is never hidden behind a closed Note: it opens on load and on every change (another tab).
  const images = useComposerImages(session.id)
  const hasImages = images.images.length > 0
  const shared = text !== "" || hasImages
  // A Working session takes no prompt: Interrupt ends its turn and keeps the session.
  const working = session.status.state === "working" && !promptable(session)
  const [open, setOpen] = useState(shared)
  // Adding an image while the agent works must not force the note open.
  useEffect(() => {
    if (shared && !working) setOpen(true)
  }, [shared, working])
  const hasContent = images.hasContent(text)
  const { send, write, enabled, pending } = usePromptSend({
    session,
    lastMessage,
    connected,
    post,
    onStale,
    toastRefusal: true,
  })

  const run: RunCall = (call) => (connected ? write(call) : Promise.resolve())

  const inRound = round.length > 0
  const sendRound = () => {
    setGapsOpen(false)
    if (!enabled) return
    return write(
      async () => {
        const res = await answer({
          id: session.id,
          source: CardSourceRound,
          card_id: roundId,
          answers: round.map((c) => answers[c.label] ?? {}),
          note: text.trim() || undefined,
          last_message: lastMessage,
          ...images.payload,
        })
        if (res.refused && res.reason === ReasonStale) onStale?.()
        return res
      },
      {
        onDone: () => {
          setText("")
          images.clear()
          onRoundSent?.()
        },
      },
    )
  }
  const unanswered = unansweredOf(round, answers)
  // With gaps, Send's click opens the popover instead of sending; a click while it is open closes it.
  const onSendRound = () => {
    if (gapsOpen) setGapsOpen(false)
    else if (images.blocking) return
    else if (unanswered.length > 0) setGapsOpen(true)
    else sendRound()
  }

  const sendText = () =>
    send({ text, ...images.payload }, () => {
      setText("")
      images.clear()
    })
  // Cmd/Ctrl+Enter runs the main button only when that is Send or Send answers; a phone has no key send.
  const onSubmit = wide
    ? () => {
        if (!enabled) return
        if (inRound) onSendRound()
        else if (images.canSend(text)) sendText()
      }
    : undefined

  const canContinue = !!next?.action.action?.length
  // On the phone the main button takes the row's remaining width; a round with no answers dims Send.
  const slot: Partial<ComponentProps<typeof Button>> = wide
    ? { whiteSpace: "nowrap", flex: "none" }
    : { flex: "1", minW: "0", h: "44px", justifyContent: "center", borderRadius: "10px" }
  // Notes (on a question or in the composer) and images are worth sending without any answer.
  const hasQuestionNote = round.some((c) => !!answers[c.label]?.note?.trim())
  const noAnswers = !wide && unanswered.length === round.length && !hasQuestionNote && !hasContent
  // Interrupt while the agent works; Continue after a run-end line unless a note or round is waiting to go.
  const mainButton = working ? (
    <InterruptControl
      connected={connected}
      pending={pending}
      buttonProps={slot}
      onInterrupt={() => write(() => interrupt({ id: session.id, state: stateToken(session) }))}
    />
  ) : canContinue && !inRound && !hasContent ? (
    <Button
      aria-label={nextTitle ? `Continue ${nextTitle}` : "Continue"}
      {...slot}
      disabled={!enabled}
      onClick={() => send({ continue: true })}
    >
      {pending && <Spinner size="sm" />}
      Continue
    </Button>
  ) : inRound ? (
    <>
      <Button
        ref={sendRef}
        data-send-answers=""
        {...slot}
        disabled={!enabled || images.blocking || noAnswers}
        onClick={onSendRound}
      >
        {pending && <Spinner size="sm" />}
        {wide ? "Send answers" : "Send"}
      </Button>
      <SendPopover
        open={gapsOpen && unanswered.length > 0}
        unanswered={unanswered}
        total={round.length}
        sendDisabled={!enabled || images.blocking}
        anchorRef={sendRef}
        onSendAnyway={sendRound}
        onAnswer={(c) => onReview?.(round.findIndex((r) => r.label === c.label))}
        onClose={() => setGapsOpen(false)}
      />
    </>
  ) : (
    <Button {...slot} disabled={!enabled || !images.canSend(text)} onClick={sendText}>
      {pending && <Spinner size="sm" />}
      Send
    </Button>
  )

  const autoAdvanceBar = autoMenu && (
    <AutoAdvanceBar
      session={session}
      disabled={!connected || pending}
      run={run}
      setAutoAdvance={setAutoAdvance}
      setAutoQueue={setAutoQueue}
      cancelAutoAdvance={cancelAutoAdvance}
      cancelAutoQueue={cancelAutoQueue}
      menu={autoMenu}
      phone={!wide}
    />
  )

  const hint = codexSkillHint(session.harness, text)
  const footerLine = <FooterLine session={session} />

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface"
      pt="3"
      pb="0"
      display="flex"
      flexDirection="column"
      gap="2"
      role="form"
      aria-label="Composer"
    >
      <ComposerImages
        state={images}
        phone={!wide}
        gap="3"
        footer={!open && footerLine}
        afterAdd={
          <>
            <Button
              emphasis="secondary"
              aria-label="Note"
              title={wide ? undefined : "Note"}
              aria-expanded={open}
              {...(wide
                ? {}
                : {
                    w: "44px",
                    h: "44px",
                    px: "0",
                    justifyContent: "center",
                    flex: "none",
                    position: "relative",
                  })}
              onClick={() => setOpen(!open)}
            >
              <PiNotePencil size={wide ? undefined : 20} />
              {wide && " Note"}
              {!wide && (open || text !== "") && (
                <Box
                  data-part="note-dot"
                  position="absolute"
                  top="7px"
                  right="7px"
                  w="8px"
                  h="8px"
                  borderRadius="full"
                  bg="tone.running.fg"
                />
              )}
            </Button>
            {autoAdvanceBar}
          </>
        }
        send={
          <>
            {wide && canContinue && !working && (
              <Text as="span" fontSize="xs" color="text.dim" truncate minW="0">
                {nextTitle}
              </Text>
            )}
            {mainButton}
          </>
        }
      >
        {open && (
          <PromptBox
            rows={3}
            label="Message"
            name="design-note"
            title={inRound ? "Note on this round" : "Message to the agent"}
            placeholder={inRound ? "A note on this round" : "A message to the agent"}
            value={text}
            disabled={!enabled}
            autoFocus
            onChange={setText}
            onSubmit={onSubmit}
            note={hint ? { text: hint } : null}
            footer={footerLine}
            completion={{
              target: { sessionId: session.id },
              harness: session.harness,
              off: !enabled,
            }}
          />
        )}
        {!wide && autoMenu && (
          <AutoCountdown
            strip
            session={session}
            disabled={!connected || pending}
            run={run}
            cancelAutoAdvance={cancelAutoAdvance}
            cancelAutoQueue={cancelAutoQueue}
          />
        )}
      </ComposerImages>
    </Box>
  )
}
