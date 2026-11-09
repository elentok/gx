import { Box, chakra, Text } from "@chakra-ui/react";
import { useId, useState } from "react";
import { NoteBox } from "../../design/NoteBox";
import { Markdown } from "../../markdown/Markdown";
import { Button } from "../button/Button";
import { ExpandableTextField } from "../expandable-text/ExpandableTextField";
import { choicesOf, isAnswered, nameOf, type Answer, type Answers, type QuestionCard } from "./round";

function Choice({
  label,
  letter,
  id,
  consequence,
  on,
  recommended,
  onClick,
}: {
  label: string;
  /** An options row's A, B, C. Yes, No and Huh? have none. */
  letter?: string;
  /** The option's id, shown muted after the label. */
  id?: string;
  consequence?: string;
  on: boolean;
  recommended: boolean;
  onClick: () => void;
}) {
  const dim = on ? "surface" : "text.dim";
  // The button's name is the label alone; the letter and id are decoration and the consequence its description.
  const uid = useId();
  return (
    <chakra.button
      type="button"
      aria-pressed={on}
      aria-labelledby={`${uid}-label`}
      aria-describedby={consequence ? `${uid}-consequence` : undefined}
      display="flex"
      flexDirection="column"
      alignItems="stretch"
      textAlign="start"
      px="4"
      py="2"
      minH="44px"
      justifyContent="center"
      borderRadius="12px"
      borderWidth="1px"
      borderColor={on ? "tone.done.fg" : "transparent"}
      bg={on ? "tone.done.fg" : "tone.muted.bg"}
      color={on ? "surface" : "text"}
      fontWeight="500"
      cursor="pointer"
      onClick={onClick}
    >
      <Box display="flex" alignItems="center" gap="2">
        {letter && <Text as="span">{letter}</Text>}
        <Markdown inline flex="1" fontSize="base" id={`${uid}-label`}>
          {label}
        </Markdown>
        {id && (
          <Text as="span" fontSize="xs" color={dim}>
            {id}
          </Text>
        )}
        {recommended && (
          <Text as="span" fontSize="xs" color={on ? "surface" : "tone.waiting.fg"}>
            ★ recommended
          </Text>
        )}
      </Box>
      {consequence && (
        <Text as="span" id={`${uid}-consequence`} fontSize="sm" fontWeight="normal" color={dim}>
          {consequence}
        </Text>
      )}
    </chakra.button>
  );
}

const letterOf = (k: number) => String.fromCharCode(65 + (k % 26));

// One question of a round. `big` is the Questions-view size.
export function Question({
  card,
  answer,
  onChange,
  onPicked,
  big,
}: {
  card: QuestionCard;
  answer: Answer;
  onChange: (a: Answer) => void;
  onPicked: () => void;
  big?: boolean;
}) {
  const [noteOpen, setNoteOpen] = useState(!!answer.note);
  const choices = choicesOf(card);
  const text = card.kind === "text" || choices.length === 0;
  const lettered = card.kind !== "yes-no";
  return (
    <Box display="flex" flexDirection="column" gap={big ? "3" : "2"}>
      {card.context &&
        (card.contextHref ? (
          // A new tab, so the draft and the place in the questions stay put while the ticket is read.
          <chakra.a href={card.contextHref} target="_blank" rel="noreferrer" fontSize="xs" color="text.dim" textDecoration="underline" aria-label={`Open ticket: ${card.context}`}>
            {card.context} ↗
          </chakra.a>
        ) : (
          <Text fontSize="xs" color="text.dim">
            {card.context}
          </Text>
        ))}
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
          <ExpandableTextField
            size="sm"
            rows={big ? 5 : 2}
            label={`Answer to question ${card.label}`}
            name="question-answer"
            title={`Answer to ${nameOf(card)} - ${card.title}`}
            placeholder="Your answer"
            value={answer.text ?? ""}
            onChange={(t) => onChange({ ...answer, huh: false, text: t })}
          />
        </>
      ) : (
        <Box display="flex" flexDirection="column" gap="2">
          {choices.map((c, k) => {
            const on = answer.choice === c.id;
            return (
              <Choice
                key={c.id}
                label={c.label}
                letter={lettered ? letterOf(k) : undefined}
                id={lettered ? c.id : undefined}
                consequence={c.consequence}
                on={on}
                recommended={card.recommended === c.id}
                onClick={() => {
                  onChange({ ...answer, huh: false, choice: on ? undefined : c.id });
                  if (!on) onPicked();
                }}
              />
            );
          })}
        </Box>
      )}
      <Box>
        <Choice
          label="Huh?"
          on={!!answer.huh}
          recommended={false}
          onClick={() => {
            const on = !answer.huh;
            onChange({ ...answer, huh: on, choice: on ? undefined : answer.choice });
            if (on) onPicked();
          }}
        />
      </Box>
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
          fieldName="question-note"
          title={`Note on ${nameOf(card)} - ${card.title}`}
          placeholder="Note on this question"
          bg="surface"
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
  round: { label: string }[];
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
