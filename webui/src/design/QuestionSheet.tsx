import { Box, chakra, Text, Textarea } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import type { SessionRoundCard } from "../generated/server";
import { Markdown } from "../markdown/Markdown";
import { Button } from "../ui/button/Button";
import { choicesOf, isAnswered, type Answer, type Answers } from "./round";

function Choice({ label, on, recommended, onClick }: { label: string; on: boolean; recommended: boolean; onClick: () => void }) {
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
      borderRadius="md"
      borderWidth="1px"
      borderColor={on ? "tone.done.fg" : "transparent"}
      bg={on ? "tone.done.fg" : "tone.muted.bg"}
      color={on ? "surface" : "text"}
      fontWeight="500"
      cursor="pointer"
      onClick={onClick}
    >
      <Markdown inline flex="1" fontSize="sm">
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

function Question({
  card,
  answer,
  onChange,
  onPicked,
}: {
  card: SessionRoundCard;
  answer: Answer;
  onChange: (a: Answer) => void;
  onPicked: () => void;
}) {
  const [noteOpen, setNoteOpen] = useState(false);
  const text = card.kind === "text" || choicesOf(card).length === 0;
  return (
    <Box display="flex" flexDirection="column" gap="2">
      <Markdown inline fontSize="md" fontWeight="600">{`Q${card.number} - ${card.title}`}</Markdown>
      {card.body && <Markdown fontSize="md">{card.body}</Markdown>}
      {text ? (
        <>
          {card.note && (
            <Text fontSize="xs" color="text.dim">
              {card.note}
            </Text>
          )}
          <Textarea
            size="sm"
            rows={2}
            aria-label={`Answer to question ${card.number}`}
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
          variant="ghost"
          size="sm"
          aria-label={`Note on question ${card.number}`}
          aria-pressed={noteOpen}
          onClick={() => setNoteOpen((o) => !o)}
        >
          📝
        </Button>
      </Box>
      {noteOpen && (
        <Textarea
          size="sm"
          rows={2}
          aria-label={`Note text for question ${card.number}`}
          placeholder="Note on this question"
          value={answer.note ?? ""}
          onChange={(e) => onChange({ ...answer, note: e.target.value })}
        />
      )}
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
}: {
  round: SessionRoundCard[];
  answers: Answers;
  onAnswers: (update: (all: Answers) => Answers) => void;
  /** A new object opens the sheet at that question. */
  focus?: { index: number };
}) {
  const [open, setOpen] = useState(true);
  const [index, setIndex] = useState(0);
  const row = useRef<HTMLDivElement>(null);
  // Set while a smooth scroll we started is still moving, so its in-between frames don't set the question.
  const scrolling = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (!focus) return;
    setIndex(focus.index);
    setOpen(true);
  }, [focus]);

  // Keep the scroll position on the current question; scrolling by hand sets the question.
  useEffect(() => {
    const el = row.current;
    if (el && typeof el.scrollTo === "function" && el.clientWidth > 0 && Math.round(el.scrollLeft / el.clientWidth) !== index) {
      window.clearTimeout(scrolling.current);
      scrolling.current = window.setTimeout(() => (scrolling.current = undefined), 600);
      el.scrollTo({ left: index * el.clientWidth, behavior: "smooth" });
    }
  }, [index, open]);
  useEffect(() => () => window.clearTimeout(scrolling.current), []);
  const onScroll = () => {
    const el = row.current;
    if (!el || el.clientWidth === 0 || scrolling.current !== undefined) return;
    setIndex(Math.min(round.length - 1, Math.max(0, Math.round(el.scrollLeft / el.clientWidth))));
  };

  const done = round.filter((c) => isAnswered(answers[c.number])).length;
  const goto = (k: number) => {
    setIndex(k);
    setOpen(true);
  };

  return (
    <Box role="group" aria-label="Questions" borderWidth="1px" borderColor="tone.waiting.fg" borderRadius="md" bg="surface.raised" overflow="hidden">
      <Box px="3" pt="2" pb="2" display="flex" alignItems="center" gap="2">
        <Box display="flex" gap="1.5" flex="1" flexWrap="wrap">
          {round.map((c, k) => {
            const answered = isAnswered(answers[c.number]);
            const current = open && k === index;
            return (
              <chakra.button
                key={c.number}
                type="button"
                aria-label={`Question ${c.number}${answered ? ", answered" : ""}`}
                aria-current={current ? "step" : undefined}
                w="7"
                h="7"
                borderRadius="full"
                borderWidth="2px"
                fontSize="xs"
                fontWeight="600"
                cursor="pointer"
                borderColor={answered ? "tone.done.fg" : current ? "tone.waiting.fg" : "border"}
                bg={answered ? "tone.done.fg" : current ? "tone.waiting.fg" : "transparent"}
                color={answered || current ? "surface" : "text.muted"}
                onClick={() => goto(k)}
              >
                {answered ? "✓" : c.number}
              </chakra.button>
            );
          })}
        </Box>
        <Text fontSize="xs" color="text.dim">
          {done} of {round.length} answered
        </Text>
        <Button variant="ghost" size="sm" aria-expanded={open} aria-label={open ? "Fold questions" : "Open questions"} onClick={() => setOpen((o) => !o)}>
          {open ? "▾" : "▴"}
        </Button>
      </Box>
      <Box hidden={!open} px="3" pb="3" maxH="50dvh" overflowY="auto">
        <Box
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
              key={c.number}
              flex="0 0 100%"
              minW="0"
              css={{ scrollSnapAlign: "start" }}
              role="group"
              aria-label={`Question ${c.number} of ${round.length}`}
              aria-hidden={k !== index}
            >
              <Question
                card={c}
                answer={answers[c.number] ?? {}}
                onChange={(a) => onAnswers((all) => ({ ...all, [c.number]: a }))}
                onPicked={() => setIndex(Math.min(k + 1, round.length - 1))}
              />
            </Box>
          ))}
        </Box>
        <Box display="flex" justifyContent="space-between" mt="2">
          <Button variant="ghost" disabled={index === 0} onClick={() => setIndex(index - 1)}>
            ← Prev
          </Button>
          <Button variant="ghost" disabled={index === round.length - 1} onClick={() => setIndex(index + 1)}>
            Next →
          </Button>
        </Box>
      </Box>
    </Box>
  );
}
