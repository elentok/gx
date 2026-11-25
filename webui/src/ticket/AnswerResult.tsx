import { Box, Text } from "@chakra-ui/react"
import { useRef, useState } from "react"
import { useOnChange } from "../hooks"
import type { TicketDetail } from "../generated/server"
import { Markdown } from "../markdown/Markdown"
import { Button } from "../ui/button/Button"
import {
  OutcomeAdopted,
  OutcomeQueued,
  isAnsweredRound,
  showsSentCard,
  type Sent,
  wasAdopted,
} from "./answer"

const dim = { fontSize: "xs", color: "text.dim" } as const
// The answer and the note of the sent card, with the mock's green accent bar.
const sentLine = {
  fontSize: "15px",
  borderLeftWidth: "3px",
  borderLeftColor: "tone.done.fg",
  pl: "2.5",
} as const

function status(sent: Sent): { head: string; tail: string; tone: "running" | "waiting" | "done" } {
  switch (sent.outcome) {
    case OutcomeQueued:
      return {
        head: "Queued",
        tail: sent.position ? `#${sent.position} in queue` : "in queue",
        tone: "waiting",
      }
    case OutcomeAdopted:
      return { head: "Sent", tail: "The agent had already finished", tone: "done" }
    default:
      return { head: "Delivered", tail: "Agent is working", tone: "running" }
  }
}

// The answer is the last web message of the exchange (it also covers the fixed "Huh?" message);
// the round's own answer is the fallback for a detail that carries no chat.
function answerText(detail: TicketDetail, sent: Sent | null): string {
  const answer = detail.latest?.answer
  const fromChat = [...detail.chat].reverse().find((m) => m.from === "web")?.text
  return (
    fromChat ||
    answer?.choice ||
    answer?.text ||
    (wasAdopted(sent) ? sent?.answer : "") ||
    "no option picked"
  )
}

function StatusLine({ shown }: { shown: ReturnType<typeof status> | null }) {
  if (!shown) return null
  return (
    <Box
      role="status"
      aria-label="Answer result"
      display="flex"
      alignItems="center"
      gap="1.5"
      whiteSpace="nowrap"
      {...dim}
    >
      <Box
        data-testid="status-dot"
        data-tone={shown.tone}
        w="2"
        h="2"
        rounded="full"
        bg={`tone.${shown.tone}.fg`}
      />
      <Text as="span">{shown.head} ·</Text>
      <Text as="span" color={`tone.${shown.tone}.fg`}>
        {shown.tail}
      </Text>
    </Box>
  )
}

// What the person picked: "A: <label>" for an option, else the text as the old card shows it.
function sentAnswer(detail: TicketDetail, sent: Sent | null): string {
  const { latest } = detail
  const at = latest?.options?.findIndex((o) => o.id === latest.answer?.choice) ?? -1
  if (at >= 0) return `${String.fromCharCode(65 + at)}: ${latest!.options![at].label}`
  return answerText(detail, sent)
}

// The card after a person answered: title, answer, note, status and a toggle for the full question.
function SentCard({
  detail,
  sent,
  shown,
}: {
  detail: TicketDetail
  sent: Sent | null
  shown: ReturnType<typeof status> | null
}) {
  const latest = detail.latest!
  const [open, setOpen] = useState(false)
  const note = latest.answer?.note
  return (
    <Box
      role="group"
      aria-label="Answered question"
      display="flex"
      flexDirection="column"
      gap="2.5"
      p="3"
      borderWidth="1px"
      borderColor="tone.done.fg"
      rounded="xl"
      bg="surface.raised"
    >
      <Text
        fontFamily="condensed"
        fontSize="13px"
        fontWeight="600"
        textTransform="uppercase"
        color="text.dim"
      >
        Answer sent
      </Text>
      <Text
        fontSize="18px"
        fontWeight="600"
        css={{
          display: "-webkit-box",
          WebkitLineClamp: 3,
          WebkitBoxOrient: "vertical",
          overflow: "hidden",
        }}
      >
        {latest.summary}
      </Text>
      <Text {...sentLine}>{sentAnswer(detail, sent)}</Text>
      {note && <Text {...sentLine}>Note: {note}</Text>}
      <StatusLine shown={shown} />
      <Box
        css={{
          "& button": {
            paddingInline: 0,
            fontSize: "15px",
            color: "var(--chakra-colors-tone-done-fg)",
          },
        }}
      >
        <Button emphasis="ghost" size="sm" aria-expanded={open} onClick={() => setOpen(!open)}>
          {open ? "▾ Show question" : "▸ Show question"}
        </Button>
      </Box>
      {open && latest.text && <Markdown epic={detail.epic}>{latest.text}</Markdown>}
    </Box>
  )
}

// The compact card of an answered round, in place of the reading view. The status line is the send
// outcome this page knows; a reload has none.
export function AnswerResult({ detail, sent }: { detail: TicketDetail; sent: Sent | null }) {
  const latest = detail.latest
  const [open, setOpen] = useState(false)
  const [cut, setCut] = useState(false)
  const body = useRef<HTMLDivElement>(null)
  // The text is a deliberate re-measure trigger.
  useOnChange(
    [latest?.text, open],
    () => {
      const el = body.current
      if (!el || open) return
      const measure = () => setCut(el.scrollHeight > el.clientHeight)
      measure()
      // A rotation or width change moves the line count without touching the text.
      if (typeof ResizeObserver === "undefined") return
      const observer = new ResizeObserver(measure)
      observer.observe(el)
      return () => observer.disconnect()
    },
    { immediate: true },
  )

  // An adopted send records no answer, and the round it answered may no longer be the latest.
  if (!latest || !isAnsweredRound(detail, sent)) return null
  const note = latest.answer?.note
  const shown = sent && (wasAdopted(sent) || sent.seq === latest.seq) ? status(sent) : null
  if (showsSentCard(latest, sent)) return <SentCard detail={detail} sent={sent} shown={shown} />
  // A round a recovery verb closed (how: "resume") keeps this compact layout: it was never
  // answered by a person, so it must not read "Answer sent".
  const toggle = cut || !!latest.handoff || open
  return (
    <Box
      role="group"
      aria-label="Answered question"
      display="flex"
      flexDirection="column"
      gap="1.5"
    >
      <Text
        fontFamily="condensed"
        fontSize="xs"
        fontWeight="600"
        textTransform="uppercase"
        color="fg.muted"
      >
        {latest.by === "gx" ? "GX ASKED" : "THE AGENT ASKED"}
      </Text>
      {latest.text && (
        <Box
          ref={body}
          css={
            open
              ? undefined
              : {
                  display: "-webkit-box",
                  WebkitLineClamp: 3,
                  WebkitBoxOrient: "vertical",
                  overflow: "hidden",
                }
          }
        >
          <Markdown epic={detail.epic}>{latest.text}</Markdown>
        </Box>
      )}
      {open && latest.handoff && (
        <Markdown epic={detail.epic} smallHeadings>
          {latest.handoff}
        </Markdown>
      )}
      {toggle && (
        <Box>
          <Button emphasis="ghost" size="sm" aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? "▾ Show less" : "▸ Show more"}
          </Button>
        </Box>
      )}
      <Text mt="2">{latest.summary}</Text>
      {latest.prompt && latest.prompt !== latest.summary && <Text>{latest.prompt}</Text>}
      <Box display="flex" gap="2" alignItems="baseline" px="2.5" py="2" rounded="md" bg="surface">
        <Text {...dim}>You</Text>
        <Text fontWeight="700">{answerText(detail, sent)}</Text>
      </Box>
      {note && <Text color="fg.muted">{note}</Text>}
      <StatusLine shown={shown} />
      {latest.closed_reason && <Text {...dim}>Closed: {latest.closed_reason}</Text>}
    </Box>
  )
}
