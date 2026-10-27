import { Box, chakra, Text, Textarea } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import type { SessionRoundCard } from "../generated/server";
import { Markdown } from "../markdown/Markdown";
import { Button } from "../ui/button/Button";
import { NoteBox } from "./NoteBox";
import {
  choicesOf,
  isAnswered,
  nameOf,
  type Answer,
  type Answers,
} from "./round";

function Choice({
  label,
  on,
  recommended,
  onClick,
}: {
  label: string;
  on: boolean;
  recommended: boolean;
  onClick: () => void;
}) {
  return (
    <chakra.button
      type="button"
      aria-pressed={on}
      display="flex"
      alignItems="center"
      gap="2"
      textAlign="start"
      px="4"
      minH="44px"
      borderRadius="12px"
      borderWidth="1px"
      borderColor={on ? "tone.done.fg" : "transparent"}
      bg={on ? "tone.done.fg" : "tone.muted.bg"}
      color={on ? "surface" : "text"}
      fontWeight="500"
      cursor="pointer"
      onClick={onClick}
    >
      <Markdown inline flex="1" fontSize="base">
        {label}
      </Markdown>
      {recommended && (
        <Text fontSize="xs" color={on ? "surface" : "tone.waiting.fg"}>
          ★ recommended
        </Text>
      )}
    </chakra.button>
  );
}

// One question of a round. `big` is the Questions-view size.
export function Question({
  card,
  answer,
  onChange,
  onPicked,
  big,
}: {
  card: SessionRoundCard;
  answer: Answer;
  onChange: (a: Answer) => void;
  onPicked: () => void;
  big?: boolean;
}) {
  const [noteOpen, setNoteOpen] = useState(!!answer.note);
  const text = card.kind === "text" || choicesOf(card).length === 0;
  return (
    <Box display="flex" flexDirection="column" gap={big ? "3" : "2"}>
      <Markdown
        inline
        fontSize={big ? "title" : "subtitle"}
        fontWeight="600"
      >{`${nameOf(card)} - ${card.title}`}</Markdown>
      {card.body && <Markdown fontSize={big ? "lg" : "md"}>{card.body}</Markdown>}
      {text ? (
        <>
          {card.note && (
            <Text fontSize="xs" color="text.dim">
              {card.note}
            </Text>
          )}
          <Textarea
            size="sm"
            borderRadius="12px"
            rows={big ? 5 : 2}
            aria-label={`Answer to question ${card.label}`}
            placeholder="Your answer"
            value={answer.text ?? ""}
            onChange={(e) => onChange({ ...answer, text: e.target.value })}
          />
        </>
      ) : (
        <Box display="flex" flexDirection="column" gap="2">
          {choicesOf(card).map((c) => {
            const on = answer.choice === c.id;
            return (
              <Choice
                key={c.id}
                label={c.label}
                on={on}
                recommended={card.recommended === c.id}
                onClick={() => {
                  onChange({ ...answer, choice: on ? undefined : c.id });
                  if (!on) onPicked();
                }}
              />
            );
          })}
        </Box>
      )}
      <Box>
        <Button
          emphasis="ghost"
          tone="muted"
          size="sm"
          aria-label={`Note on question ${card.label}`}
          aria-pressed={noteOpen}
          onClick={() => setNoteOpen((o) => !o)}
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" aria-hidden="true">
            <path d="M3 13l.7-3L10.5 3.2a1.4 1.4 0 0 1 2 2L5.7 12 3 13z" />
          </svg>
          Note
        </Button>
      </Box>
      {noteOpen && (
        <NoteBox
          name={`question ${card.label}`}
          title={`Note on ${nameOf(card)} - ${card.title}`}
          placeholder="Note on this question"
          value={answer.note ?? ""}
          onChange={(note) => onChange({ ...answer, note })}
        />
      )}
    </Box>
  );
}

// A circle per question: ✓ once answered, filled for the current one. Clicking one opens it.
export function QuestionDots({
  round,
  answers,
  current,
  onPick,
}: {
  round: SessionRoundCard[];
  answers: Answers;
  current?: number;
  onPick: (k: number) => void;
}) {
  return (
    <Box display="flex" gap="1.5" flexWrap="wrap">
      {round.map((c, k) => {
        const answered = isAnswered(answers[c.label]);
        const on = k === current;
        return (
          <chakra.button
            key={c.label}
            type="button"
            aria-label={`Question ${c.label}${answered ? ", answered" : ""}`}
            aria-current={on ? "step" : undefined}
            w="7"
            h="7"
            borderRadius="full"
            borderWidth="2px"
            fontSize={c.label.length > 2 ? "2xs" : "xs"}
            fontWeight="600"
            cursor="pointer"
            borderColor={
              answered ? "tone.done.fg" : on ? "tone.waiting.fg" : "border"
            }
            bg={
              answered ? "tone.done.fg" : on ? "tone.waiting.fg" : "transparent"
            }
            color={answered || on ? "surface" : "text.muted"}
            onClick={() => onPick(k)}
          >
            {answered ? "✓" : c.label}
          </chakra.button>
        );
      })}
    </Box>
  );
}

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
}: {
  round: SessionRoundCard[];
  answers: Answers;
  onAnswers: (update: (all: Answers) => Answers) => void;
  /** A new object opens the sheet at that question. */
  focus?: { index: number };
  /** The question the sheet opens at, and a listener for every move, so the caller can keep a draft. */
  initialIndex?: number;
  onIndex?: (index: number) => void;
  /** The Questions view: the sheet fills its column, renders big and cannot fold. The page owns it. */
  questions?: boolean;
}) {
  const [open, setOpen] = useState(true);
  useEffect(() => {
    if (questions) setOpen(true);
  }, [questions]);
  const [index, setIndex] = useState(() =>
    Math.min(initialIndex, round.length - 1),
  );
  useEffect(() => onIndex?.(index), [index, onIndex]);
  const row = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);
  // A move starts at the question's title, not wherever the last one was left.
  const toTop = () => {
    if (body.current) body.current.scrollTop = 0;
  };
  useEffect(toTop, [index]);
  // Set while a smooth scroll we started is still moving, so its in-between frames don't set the question.
  const scrolling = useRef<number | undefined>(undefined);

  // Focus waits a render, so a folded sheet is shown (and the question visible) by the time it lands.
  const [toFocus, setToFocus] = useState<{ index: number }>();
  useEffect(() => {
    if (!focus) return;
    setIndex(focus.index);
    toTop();
    setOpen(true);
    setToFocus(focus);
  }, [focus]);
  useEffect(() => {
    if (!toFocus) return;
    row.current?.children[toFocus.index]
      ?.querySelector<HTMLElement>("button[aria-pressed], textarea")
      ?.focus({ preventScroll: true });
  }, [toFocus]);

  // Keep the scroll position on the current question; scrolling by hand sets the question.
  useEffect(() => {
    const el = row.current;
    if (
      el &&
      typeof el.scrollTo === "function" &&
      el.clientWidth > 0 &&
      Math.round(el.scrollLeft / el.clientWidth) !== index
    ) {
      window.clearTimeout(scrolling.current);
      scrolling.current = window.setTimeout(
        () => (scrolling.current = undefined),
        600,
      );
      el.scrollTo({ left: index * el.clientWidth, behavior: "smooth" });
    }
  }, [index, open]);
  useEffect(() => () => window.clearTimeout(scrolling.current), []);
  const onScroll = () => {
    const el = row.current;
    if (!el || el.clientWidth === 0 || scrolling.current !== undefined) return;
    setIndex(
      Math.min(
        round.length - 1,
        Math.max(0, Math.round(el.scrollLeft / el.clientWidth)),
      ),
    );
  };

  const done = round.filter((c) => isAnswered(answers[c.label])).length;
  const goto = (k: number) => {
    setIndex(k);
    setOpen(true);
  };

  return (
    <Box
      role="group"
      aria-label="Questions"
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      borderRadius="12px"
      bg="surface.raised"
      overflow="hidden"
      display="flex"
      flexDirection="column"
      minH="0"
      flex={questions ? "1" : undefined}
    >
      <Box
        px="3"
        pt="2"
        pb="2"
        display="flex"
        alignItems="center"
        gap="2"
        flex="none"
      >
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
      <Box
        display={open ? "flex" : "none"}
        flexDirection="column"
        flex="1"
        minH="0"
      >
        <Box
          ref={body}
          data-testid="question-body"
          px="3"
          minH="0"
          flex="1"
          overflowY="auto"
        >
          <Box
            maxW={questions ? "820px" : undefined}
            ref={row}
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
              <Box
                key={c.label}
                flex="0 0 100%"
                minW="0"
                css={{ scrollSnapAlign: "start" }}
                role="group"
                aria-label={`Question ${c.label} of ${round.length}`}
                aria-hidden={k !== index}
              >
                <Question
                  big={questions}
                  card={c}
                  answer={answers[c.label] ?? {}}
                  onChange={(a) =>
                    onAnswers((all) => ({ ...all, [c.label]: a }))
                  }
                  onPicked={() => setIndex(Math.min(k + 1, round.length - 1))}
                />
              </Box>
            ))}
          </Box>
        </Box>
        <Box
          display="flex"
          justifyContent="space-between"
          flex="none"
          px="3"
          pt="2"
          pb="3"
        >
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
  );
}
