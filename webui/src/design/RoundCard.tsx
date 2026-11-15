import { Box, chakra, Text } from "@chakra-ui/react"
import { createContext, useContext, useState } from "react"
import { Markdown } from "../markdown/Markdown"
import { Question } from "../ui/question-sheet/Question"
import {
  choicesOf,
  isAnswered,
  type Answer,
  type Answers,
  type QuestionCard,
} from "../ui/question-sheet/round"
import { NoteBox } from "./NoteBox"

/** True while the open round is the final "Write this plan?" approval: the card says so instead of counting. */
export const FinalRoundContext = createContext(false)

const MAX_DOTS = 6
const NO_ANSWER: Answer = {}

// At most MAX_DOTS dots, the window sliding so the current question stays inside it.
function dotWindow(count: number, index: number): [number, number] {
  if (count <= MAX_DOTS) return [0, count]
  const from = Math.min(Math.max(0, index - MAX_DOTS + 1), count - MAX_DOTS)
  return [from, from + MAX_DOTS]
}

/** The Triage page's round card: one question at a time, the person's earlier answers folded above it. */
export function RoundCard({
  round,
  answers,
  onAnswers,
  index,
  onIndex,
}: {
  round: QuestionCard[]
  answers: Answers
  onAnswers: (update: (all: Answers) => Answers) => void
  index: number
  onIndex: (index: number) => void
}) {
  const final = useContext(FinalRoundContext)
  const k = Math.min(index, round.length - 1)
  const card = round[k]
  const answer = answers[card.label] ?? NO_ANSWER
  const [noteOpen, setNoteOpen] = useState(false)
  const set = (a: Answer) => onAnswers((all) => ({ ...all, [card.label]: a }))
  const choices = choicesOf(card)
  // Only single-choice questions get the card's own rows; text and multi-select keep the shared question.
  const plain = card.kind === "text" || card.kind === "multi-select" || choices.length === 0
  const [from, to] = dotWindow(round.length, k)
  return (
    <Box
      role="group"
      aria-label="Questions"
      data-part="question-card"
      borderWidth="1px"
      borderColor="tone.running.fg"
      borderRadius="10px"
      bg="surface.raised"
      p="3"
      display="flex"
      flexDirection="column"
      gap="1.5"
    >
      <Box display="flex" justifyContent="space-between" alignItems="center" fontSize="xs">
        <Text as="span" color="text.dim">
          {final ? "Final question" : `Question ${k + 1} of ${round.length}`}
        </Text>
        <Box display="flex" gap="1" alignItems="center">
          {round.slice(from, to).map((c, i) => {
            const at = from + i
            const done = isAnswered(answers[c.label], c)
            return (
              <chakra.button
                key={c.label}
                type="button"
                aria-label={`Question ${c.label}${done ? ", answered" : ""}`}
                aria-current={at === k ? "step" : undefined}
                w="7px"
                h="7px"
                p="0"
                borderRadius="full"
                cursor="pointer"
                bg={done ? "tone.done.fg" : at === k ? "tone.running.fg" : "border"}
                onClick={() => onIndex(at)}
              />
            )
          })}
        </Box>
      </Box>
      {plain ? (
        <Question
          card={card}
          answer={answer}
          onChange={set}
          onPicked={() => onIndex(Math.min(k + 1, round.length - 1))}
          mode="plain"
        />
      ) : (
        <>
          <Markdown inline fontWeight="600" fontSize="md">
            {card.title}
          </Markdown>
          {card.body && (
            <Markdown fontSize="sm" color="text.dim">
              {card.body}
            </Markdown>
          )}
          <Box display="flex" flexDirection="column" gap="1.5" mt="1.5">
            {choices.map((c) => {
              const on = answer.choice === c.id
              return (
                <chakra.button
                  key={c.id}
                  type="button"
                  aria-pressed={on}
                  data-answer=""
                  data-focus-ring="inset"
                  display="flex"
                  justifyContent="space-between"
                  alignItems="center"
                  gap="2"
                  textAlign="start"
                  minH="44px"
                  px="3"
                  py="2"
                  fontSize="sm"
                  borderRadius="8px"
                  borderWidth="1px"
                  borderColor={
                    on ? "tone.done.fg" : card.recommended === c.id ? "tone.running.fg" : "border"
                  }
                  bg={on ? "tone.done.bg" : "surface"}
                  cursor="pointer"
                  onClick={() => {
                    set({ ...answer, huh: false, choice: on ? undefined : c.id })
                    if (!on) onIndex(Math.min(k + 1, round.length - 1))
                  }}
                >
                  <Markdown inline>{c.label}</Markdown>
                  {card.recommended === c.id && (
                    <Text as="span" fontSize="2xs" color="tone.running.fg" whiteSpace="nowrap">
                      Recommended
                    </Text>
                  )}
                </chakra.button>
              )
            })}
          </Box>
          {noteOpen || answer.note ? (
            <NoteBox
              name={`question ${card.label}`}
              fieldName="question-note"
              title={`Note on ${card.title}`}
              placeholder="Note on this question"
              bg="surface"
              value={answer.note ?? ""}
              onChange={(note) => set({ ...answer, note })}
            />
          ) : (
            <chakra.button
              type="button"
              data-note-toggle=""
              textAlign="start"
              mt="1"
              px="2.5"
              py="2"
              fontSize="sm"
              color="text.dim"
              borderWidth="1px"
              borderStyle="dashed"
              borderRadius="8px"
              cursor="pointer"
              onClick={() => setNoteOpen(true)}
            >
              Add a note…
            </chakra.button>
          )}
        </>
      )}
    </Box>
  )
}
