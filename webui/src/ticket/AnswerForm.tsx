import { Box, Text } from "@chakra-ui/react"
import { useState, type ReactNode } from "react"
import { ActionAnswer, type TicketDetail } from "../generated/server"
import { Button } from "../ui/button/Button"
import { Question } from "../ui/question-sheet/Question"
import type { Answer, QuestionCard } from "../ui/question-sheet/round"
import {
  answerBody,
  isReady,
  isTextKind,
  postAnswer,
  ReasonStale,
  refusalText,
  sentOf,
  type PostAnswer,
  type Sent,
} from "./answer"

// The answer area. The server decides whether it shows (the detail's actions).
// "sticky" is the box under the ticket page; "page" is a scroll body with the Send footer outside it.
export function AnswerForm({
  detail,
  post = postAnswer,
  onSent,
  onStale,
  layout = "sticky",
  lead,
  children,
}: {
  detail: TicketDetail
  post?: PostAnswer
  onSent?: (sent: Sent) => void
  /** The question changed under the person: refetch so the current one shows. */
  onStale?: () => void
  layout?: "sticky" | "page"
  /** Page layout: scroll content above the question. */
  lead?: ReactNode
  /** Page layout: scroll content below the question. */
  children?: ReactNode
}) {
  const { latest } = detail
  const [answer, setAnswer] = useState<Answer>({})
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  const [sentSeq, setSentSeq] = useState<number | null>(null)
  const [seenState, setSeenState] = useState(detail.state)

  // A new question clears the pick and the text; the note stays so a stale refusal loses nothing.
  if (seenState !== detail.state) {
    setSeenState(detail.state)
    setAnswer((a) => ({ note: a.note }))
  }

  if (!latest || !detail.actions.includes(ActionAnswer) || sentSeq === latest.seq) return null

  const isText = isTextKind(latest.kind)
  const ready = isReady(answer, isText)

  const send = async () => {
    if (!ready || pending) return
    setPending(true)
    setError("")
    try {
      const res = await post(answerBody(detail, answer, isText))
      if ("refused" in res) {
        setError(refusalText(res))
        if (res.reason === ReasonStale) onStale?.()
      } else {
        setSentSeq(latest.seq)
        onSent?.(sentOf(latest.seq, detail.address, res, answer.choice || answer.text))
      }
    } catch {
      setError("Could not send the answer. Try again.")
    } finally {
      setPending(false)
    }
  }

  const card: QuestionCard = {
    label: String(latest.seq),
    title: latest.summary,
    body: latest.text,
    kind: latest.kind ?? "text",
    yes: latest.yes,
    no: latest.no,
    options: latest.options,
    recommended: latest.recommended,
    links: latest.links,
  }

  const question = <Question card={card} answer={answer} onChange={setAnswer} onPicked={() => {}} />
  const alert = error && <Text role="alert">{error}</Text>
  const sendButton = (
    <Button emphasis="main" alignSelf="flex-end" disabled={!ready || pending} onClick={send}>
      Send answer
    </Button>
  )

  if (layout === "page") {
    return (
      <Box role="form" aria-label="Answer" flex="1" minH="0" display="flex" flexDirection="column">
        <Box flex="1" minH="0" overflowY="auto" overscrollBehavior="contain">
          <Box maxW="760px" mx="auto" py="4" display="flex" flexDirection="column" gap="4">
            {lead}
            {question}
            {children}
          </Box>
        </Box>
        <Box
          borderTopWidth="1px"
          borderColor="border"
          bg="surface.raised"
          py="12px"
          display="flex"
          flexDirection="column"
          gap="2"
        >
          {alert}
          {sendButton}
        </Box>
      </Box>
    )
  }

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface.raised"
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      aria-label="Answer"
      role="form"
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      borderRadius="12px"
      px="3"
    >
      {/* A long question scrolls inside this area so the Send button stays on screen. */}
      <Box overflowY="auto" minH="0" maxH="calc(100dvh - 160px)" overscrollBehavior="contain">
        {question}
      </Box>
      {alert}
      {sendButton}
    </Box>
  )
}
