import { Box, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import type { SessionRoundCard } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Question, QuestionDots } from "../ui/question-sheet/Question";
import { isAnswered, type Answers } from "../ui/question-sheet/round";

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
      // In the Questions view the card never shrinks below a readable height, even with the keyboard up.
      minH={questions ? "220px" : "0"}
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
