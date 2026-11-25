import { Box, Text } from "@chakra-ui/react"
import { useState, type ReactNode } from "react"
import { ActionAnswer, type TicketDetail } from "../generated/server"
import { Button } from "../ui/button/Button"
import { Question } from "../ui/question-sheet/Question"
import type { QuestionCard } from "../ui/question-sheet/round"
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
import { clearDraft, useDraft } from "./draft"

// The page footer's button fills the row on a phone and sits at the right edge on a wide screen.
const FOOTER_ALIGN = { base: "stretch", md: "flex-end" }

export const BackButton = ({ onClick }: { onClick: () => void }) => (
  <Button
    emphasis="main"
    alignSelf={FOOTER_ALIGN}
    data-mock-part="back-to-ticket"
    onClick={onClick}
  >
    Back to ticket
  </Button>
)

/** The answer page's scroll body and footer; the banner (stale states) is the body's first item. */
export function AnswerPageFrame({
  form,
  banner,
  footer,
  children,
}: {
  form?: boolean
  banner?: string
  footer: ReactNode
  children?: ReactNode
}) {
  return (
    <Box
      role={form ? "form" : undefined}
      aria-label={form ? "Answer" : undefined}
      flex="1"
      minH="0"
      display="flex"
      flexDirection="column"
    >
      <Box flex="1" minH="0" overflowY="auto" overscrollBehavior="contain">
        <Box maxW="760px" mx="auto" py="4" display="flex" flexDirection="column" gap="4">
          {banner && (
            <Box
              role="status"
              data-mock-part="stale-banner"
              borderWidth="1px"
              borderColor="tone.waiting.fg"
              bg="tone.waiting.bg"
              color="tone.waiting.fg"
              borderRadius="8px"
              py="10px"
              px="12px"
              fontWeight="600"
              fontSize="15px"
            >
              {banner}
            </Box>
          )}
          {children}
        </Box>
      </Box>
      <Box
        data-mock-part="answer-footer"
        borderTopWidth="1px"
        borderColor="border"
        bg="surface.raised"
        py="12px"
        display="flex"
        flexDirection="column"
        gap="2"
      >
        {footer}
      </Box>
    </Box>
  )
}

// The answer area. The server decides whether it shows (the detail's actions).
// "sticky" is the box under the ticket page; "page" is a scroll body with the Send footer outside it.
export function AnswerForm({
  detail,
  post = postAnswer,
  onSent,
  onStale,
  layout = "sticky",
  stale,
  lead,
  children,
}: {
  detail: TicketDetail
  post?: PostAnswer
  onSent?: (sent: Sent) => void
  /** The question changed under the person: refetch so the current one shows. */
  onStale?: () => void
  layout?: "sticky" | "page"
  /** Page layout: the question can no longer be answered. Shows the banner and only a way back. */
  stale?: { banner: string; onBack: () => void }
  /** Page layout: scroll content above the question. */
  lead?: ReactNode
  /** Page layout: scroll content below the question. */
  children?: ReactNode
}) {
  const { latest } = detail
  // A new question clears the pick and the text; the note stays so a stale refusal loses nothing.
  const [answer, setAnswer] = useDraft(detail.address, detail.state)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  const [sentSeq, setSentSeq] = useState<number | null>(null)

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
        clearDraft(detail.address)
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

  const sticky = layout === "sticky"
  const question = <Question card={card} answer={answer} onChange={setAnswer} onPicked={() => {}} />
  const alert = error && <Text role="alert">{error}</Text>
  const sendButton = (
    <Button
      emphasis="main"
      alignSelf={sticky ? "flex-end" : FOOTER_ALIGN}
      disabled={!ready || pending}
      onClick={send}
    >
      Send answer
    </Button>
  )

  if (layout === "page") {
    return (
      <AnswerPageFrame
        form
        banner={stale?.banner}
        footer={
          stale ? (
            <BackButton onClick={stale.onBack} />
          ) : (
            <>
              {alert}
              {sendButton}
            </>
          )
        }
      >
        {lead}
        {question}
        {children}
      </AnswerPageFrame>
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
