import { Box, Text } from "@chakra-ui/react"
import type { SessionRoundCard } from "../generated/server"
import { choicesOf, isAnswered, nameOf, type Answers } from "../ui/question-sheet/round"

// What the person chose, as the line shows it: option labels, else the typed text, else Huh?.
function chosen(card: SessionRoundCard, answers: Answers): string {
  const a = answers[card.label]
  if (!a) return ""
  if (a.huh) return "Huh?"
  const picked = new Set([a.choice, ...(a.choices ?? [])])
  const labels = choicesOf(card)
    .filter((c) => picked.has(c.id))
    .map((c) => (c.text && a.text?.trim() ? a.text.trim() : c.label))
  return labels.length > 0 ? labels.join(", ") : (a.text?.trim() ?? "")
}

/** The cards of a round the person has answered, each folded to one line above the open card. */
export function AnsweredQuestions({
  round,
  answers,
}: {
  round: SessionRoundCard[]
  answers: Answers
}) {
  const done = round.filter((c) => isAnswered(answers[c.label], c))
  if (done.length === 0) return null
  return (
    <Box display="flex" flexDirection="column" gap="1" data-part="answered-question-list">
      {done.map((c) => (
        <Text key={c.label} fontSize="sm" color="text.muted" data-part="answered-question">
          <Text as="span" aria-hidden color="tone.done.fg">
            ✓
          </Text>{" "}
          {nameOf(c)} {c.title} → {chosen(c, answers)}
        </Text>
      ))}
    </Box>
  )
}
