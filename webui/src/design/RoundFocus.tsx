import { Box, chakra, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import type { SessionRoundCard } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Question, QuestionDots } from "./QuestionSheet";
import { answerText, isAnswered, nameOf, type Answers } from "./round";

// A Grilling round full screen: a rail of every question with its answer, the current question large beside it,
// and the composer below it. The composer portals into `footer`, so its draft survives the switch.
// Escape or the collapse button goes back to the docked sheet.
export function RoundFocus({
  round,
  answers,
  onAnswers,
  focus,
  onClose,
  footer,
}: {
  round: SessionRoundCard[];
  answers: Answers;
  onAnswers: (update: (all: Answers) => Answers) => void;
  /** A new object moves to that question; the one already set when the overlay opens is ignored. */
  focus?: { index: number };
  onClose: () => void;
  /** Receives the element the composer renders into. */
  footer: (el: HTMLDivElement | null) => void;
}) {
  const [index, setIndex] = useState(() => Math.max(0, round.findIndex((c) => !isAnswered(answers[c.label]))));
  const openedWith = useRef(focus);
  useEffect(() => {
    if (focus && focus !== openedWith.current) setIndex(focus.index);
  }, [focus]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
    };
  }, [onClose]);

  const done = round.filter((c) => isAnswered(answers[c.label])).length;
  const card = round[index];
  return (
    <Box
      role="dialog"
      aria-modal="true"
      aria-label="Questions, full screen"
      position="fixed"
      inset="0"
      zIndex="modal"
      bg="surface"
      display="grid"
      gridTemplateColumns={{ base: "1fr", md: "300px minmax(0, 1fr)" }}
    >
      <Box display={{ base: "none", md: "flex" }} flexDirection="column" borderRightWidth="1px" bg="surface.raised" overflowY="auto">
        <Box px="4" py="3" display="flex" alignItems="center" borderBottomWidth="1px">
          <Text flex="1" fontWeight="600">
            Questions
          </Text>
          <Text fontSize="xs" color="text.dim">
            {done}/{round.length}
          </Text>
        </Box>
        {round.map((c, k) => {
          const a = answers[c.label];
          const answered = isAnswered(a);
          return (
            <chakra.button
              key={c.label}
              type="button"
              aria-current={k === index ? "step" : undefined}
              textAlign="start"
              px="4"
              py="3"
              borderLeftWidth="3px"
              borderLeftColor={k === index ? "tone.waiting.fg" : "transparent"}
              bg={k === index ? "surface" : undefined}
              cursor="pointer"
              onClick={() => setIndex(k)}
            >
              <Text fontSize="sm" fontWeight="600">
                <chakra.span color={answered ? "tone.done.fg" : "text.dim"}>{answered ? "✓" : "○"}</chakra.span> {nameOf(c)} {c.title}
              </Text>
              {a && answered && (
                <Text fontSize="xs" color="text.muted" lineClamp={1} pl="4">
                  {answerText(c, a)}
                </Text>
              )}
            </chakra.button>
          );
        })}
      </Box>
      <Box display="flex" flexDirection="column" minH="0" minW="0">
        <Box px="4" py="3" borderBottomWidth="1px" display="flex" alignItems="center" gap="3">
          <Box flex="1" display={{ base: "block", md: "none" }}>
            <QuestionDots round={round} answers={answers} current={index} onPick={setIndex} />
          </Box>
          <Text flex="1" display={{ base: "none", md: "block" }} color="text.dim" fontSize="sm">
            {index + 1} of {round.length}
          </Text>
          <Button variant="ghost" size="sm" fontSize="lg" aria-label="Exit full screen" title="Exit full screen (Esc)" onClick={onClose}>
            ⤡
          </Button>
        </Box>
        <Box flex="1" overflowY="auto" px={{ base: "4", md: "10" }} py="8">
          <Box maxW="680px">
            <Question
              big
              key={card.label}
              card={card}
              answer={answers[card.label] ?? {}}
              onChange={(a) => onAnswers((all) => ({ ...all, [card.label]: a }))}
              onPicked={() => setIndex(Math.min(index + 1, round.length - 1))}
            />
            <Box display="flex" justifyContent="space-between" mt="6">
              <Button variant="ghost" disabled={index === 0} onClick={() => setIndex(index - 1)}>
                ← Prev
              </Button>
              <Button variant="ghost" disabled={index === round.length - 1} onClick={() => setIndex(index + 1)}>
                Next →
              </Button>
            </Box>
          </Box>
        </Box>
        <Box ref={footer} />
      </Box>
    </Box>
  );
}
