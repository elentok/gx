import { Box, Text } from "@chakra-ui/react"
import { memo, useCallback, useEffect, useRef, useState } from "react"
import type { SessionRoundCard } from "../generated/server"
import { NO_SIDEBAR_SWIPE_ATTR } from "../shell/useSidebarSwipe"
import { Button } from "../ui/button/Button"
import { Question, QuestionDots } from "../ui/question-sheet/Question"
import {
  isAnswered,
  type Answer,
  type Answers,
  type QuestionCard,
  type QuestionMode,
} from "../ui/question-sheet/round"

const NO_ANSWER: Answer = {}

// One slide. Memoized with stable callbacks, so typing in one question does not re-render all the others.
const Slide = memo(function Slide({
  card,
  k,
  count,
  index,
  answer,
  big,
  mode,
  onAnswer,
  onPicked,
}: {
  card: QuestionCard
  k: number
  count: number
  index: number
  answer: Answer
  big: boolean
  mode: QuestionMode
  onAnswer: (label: string, a: Answer) => void
  onPicked: (k: number) => void
}) {
  return (
    <Box
      flex="0 0 100%"
      minW="0"
      css={{ scrollSnapAlign: "start" }}
      role="group"
      aria-label={`Question ${card.label} of ${count}`}
      aria-hidden={k !== index}
    >
      <Question
        big={big}
        mode={mode}
        card={card}
        answer={answer}
        onChange={(a) => onAnswer(card.label, a)}
        onPicked={() => onPicked(k)}
      />
    </Box>
  )
})

// A Grilling round docked above the composer. The caller keys it by round, so a new round opens a fresh sheet,
// and owns the answers, since the composer's Send reads them.
export function QuestionSheet({
  round,
  answers,
  onAnswers,
  focus,
  initialIndex = 0,
  onIndex,
  questions = false,
  mode = "round",
}: {
  round: (SessionRoundCard & { context?: string; contextHref?: string })[]
  answers: Answers
  onAnswers: (update: (all: Answers) => Answers) => void
  /** A new object opens the sheet at that question. */
  focus?: { index: number }
  /** The question the sheet opens at, and a listener for every move, so the caller can keep a draft. */
  initialIndex?: number
  onIndex?: (index: number) => void
  /** The Questions view: the sheet fills its column, renders big and cannot fold. The page owns it. */
  questions?: boolean
  /** What each card offers besides its options. Only a printed round can be sent back as Huh?. */
  mode?: QuestionMode
}) {
  const [open, setOpen] = useState(true)
  useEffect(() => {
    if (questions) setOpen(true)
  }, [questions])
  const [index, setIndex] = useState(() => Math.min(initialIndex, round.length - 1))
  useEffect(() => onIndex?.(index), [index, onIndex])
  const row = useRef<HTMLDivElement>(null)
  const body = useRef<HTMLDivElement>(null)
  // A move starts at the question's title, not wherever the last one was left.
  const toTop = () => {
    if (body.current) body.current.scrollTop = 0
  }
  useEffect(toTop, [index])
  // Set while a smooth scroll we started is still moving, so its in-between frames don't set the question.
  const scrolling = useRef<number | undefined>(undefined)

  // Focus waits a render, so a folded sheet is shown (and the question visible) by the time it lands.
  const [toFocus, setToFocus] = useState<{ index: number }>()
  useEffect(() => {
    if (!focus) return
    setIndex(focus.index)
    toTop()
    setOpen(true)
    setToFocus(focus)
  }, [focus])
  useEffect(() => {
    if (!toFocus) return
    row.current?.children[toFocus.index]
      ?.querySelector<HTMLElement>("button[data-answer], textarea")
      ?.focus({ preventScroll: true })
  }, [toFocus])

  const answersOf = (k: number) =>
    Array.from(row.current?.children[k]?.querySelectorAll<HTMLElement>("button[data-answer]") ?? [])
  // The selected answer of question k, else its first one.
  const focusAnswer = (k: number) => {
    const all = answersOf(k)
    const el = all.find((b) => b.getAttribute("aria-pressed") === "true") ?? all[0]
    el?.focus({ preventScroll: true })
  }
  const move = (to: number, first: boolean) => {
    setOpen(true)
    setIndex(to)
    if (first) setToFocus({ index: to })
    // Prev lands on the selected answer, which the focus effect's first-button rule would miss.
    else requestAnimationFrame(() => focusAnswer(to))
  }
  const openNote = () => {
    const q = row.current?.children[index]
    const find = () => q?.querySelector<HTMLTextAreaElement>("textarea[name=question-note]")
    if (!find()) q?.querySelector<HTMLElement>("button[data-note-toggle]")?.click()
    requestAnimationFrame(() => find()?.focus({ preventScroll: true }))
  }
  const onKeyDown = (e: React.KeyboardEvent) => {
    const t = e.target as HTMLElement
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      const send = document.querySelector<HTMLButtonElement>("button[data-send-answers]")
      if (send && !send.disabled) {
        e.preventDefault()
        send.click()
      }
      return
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return
    if (t.closest("textarea, input")) {
      if (e.key === "Escape") {
        e.preventDefault()
        e.stopPropagation()
        focusAnswer(index)
      }
      return
    }
    const nav = (to: number, first: boolean) => {
      e.preventDefault()
      if (to >= 0 && to < round.length) move(to, first)
    }
    switch (e.key) {
      case "n":
        return nav(index + 1, true)
      case "p":
        return nav(index - 1, false)
      case "N":
        e.preventDefault()
        return openNote()
      case "j":
      case "k":
      case "ArrowDown":
      case "ArrowUp": {
        const all = answersOf(index)
        const at = all.indexOf(t)
        const down = e.key === "j" || e.key === "ArrowDown"
        e.preventDefault()
        if (at < 0) focusAnswer(index)
        else all[Math.max(0, Math.min(all.length - 1, at + (down ? 1 : -1)))]?.focus()
      }
    }
  }

  // Keep the scroll position on the current question; scrolling by hand sets the question.
  useEffect(() => {
    const el = row.current
    if (
      el &&
      typeof el.scrollTo === "function" &&
      el.clientWidth > 0 &&
      Math.round(el.scrollLeft / el.clientWidth) !== index
    ) {
      window.clearTimeout(scrolling.current)
      scrolling.current = window.setTimeout(() => (scrolling.current = undefined), 600)
      el.scrollTo({ left: index * el.clientWidth, behavior: "smooth" })
    }
  }, [index, open])
  useEffect(() => () => window.clearTimeout(scrolling.current), [])
  const onScroll = () => {
    const el = row.current
    if (!el || el.clientWidth === 0 || scrolling.current !== undefined) return
    setIndex(Math.min(round.length - 1, Math.max(0, Math.round(el.scrollLeft / el.clientWidth))))
  }

  const onAnswersRef = useRef(onAnswers)
  onAnswersRef.current = onAnswers
  const onAnswer = useCallback(
    (label: string, a: Answer) => onAnswersRef.current((all) => ({ ...all, [label]: a })),
    [],
  )
  const last = round.length - 1
  const onPicked = useCallback((k: number) => setIndex(Math.min(k + 1, last)), [last])

  const done = round.filter((c) => isAnswered(answers[c.label], c)).length
  const goto = (k: number) => {
    setIndex(k)
    setOpen(true)
  }

  return (
    <Box
      role="group"
      aria-label="Questions"
      onKeyDown={onKeyDown}
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      borderRadius="12px"
      bg="surface.raised"
      overflow="hidden"
      display="flex"
      flexDirection="column"
      // In the Questions view the card never shrinks below a readable height, even with the keyboard up.
      minH={questions ? "220px" : "0"}
      flex={questions ? "1" : undefined}
    >
      <Box px="3" pt="2" pb="2" display="flex" alignItems="center" gap="2" flex="none">
        <Box flex="1">
          <QuestionDots
            round={round}
            answers={answers}
            current={open ? index : undefined}
            onPick={goto}
          />
        </Box>
        <Text fontSize="xs" color="text.dim">
          {done} of {round.length} answered
        </Text>
        {!questions && (
          <Button
            emphasis="ghost"
            tone="muted"
            size="sm"
            aria-expanded={open}
            aria-label={open ? "Fold questions" : "Open questions"}
            onClick={() => setOpen((o) => !o)}
          >
            {open ? "▾" : "▴"}
          </Button>
        )}
      </Box>
      <Box display={open ? "flex" : "none"} flexDirection="column" flex="1" minH="0">
        <Box ref={body} data-testid="question-body" px="3" minH="0" flex="1" overflowY="auto">
          <Box
            ref={row}
            {...{ [NO_SIDEBAR_SWIPE_ATTR]: "" }}
            onScroll={onScroll}
            display="flex"
            overflowX="auto"
            css={{
              scrollSnapType: "x mandatory",
              overscrollBehaviorX: "contain",
              scrollbarWidth: "none",
              "&::-webkit-scrollbar": { display: "none" },
            }}
          >
            {round.map((c, k) => (
              <Slide
                key={c.label}
                card={c}
                k={k}
                count={round.length}
                index={index}
                answer={answers[c.label] ?? NO_ANSWER}
                big={questions}
                mode={mode}
                onAnswer={onAnswer}
                onPicked={onPicked}
              />
            ))}
          </Box>
        </Box>
        <Box display="flex" justifyContent="space-between" flex="none" px="3" pt="2" pb="3">
          <Button
            emphasis="ghost"
            tone="muted"
            disabled={index === 0}
            onClick={() => setIndex(index - 1)}
          >
            ← Prev
          </Button>
          <Button
            emphasis="ghost"
            tone="muted"
            disabled={index === round.length - 1}
            onClick={() => setIndex(index + 1)}
          >
            Next →
          </Button>
        </Box>
      </Box>
    </Box>
  )
}
