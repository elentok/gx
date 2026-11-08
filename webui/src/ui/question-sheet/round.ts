export type Answer = { choice?: string; text?: string; note?: string; huh?: boolean };
export type Answers = Record<string, Answer>;

export interface QuestionChoice {
  id: string;
  label: string;
  consequence?: string;
}

/** One question of a round, as plain props. A server round card has this shape. */
export interface QuestionCard {
  label: string;
  title: string;
  body?: string;
  kind: string;
  yes?: string;
  no?: string;
  options?: QuestionChoice[];
  recommended?: string;
  note?: string;
}

/** The title a card shows for its question: "Q8", or "Q8b". */
export const nameOf = (c: { label: string }) => `Q${c.label}`;

/** A yes-no card reads Yes and No; its yes and no lines are the consequences. */
export const choicesOf = (c: QuestionCard): QuestionChoice[] =>
  c.kind === "yes-no"
    ? [
        { id: "yes", label: "Yes", consequence: c.yes },
        { id: "no", label: "No", consequence: c.no },
      ]
    : (c.options ?? []);

export const isAnswered = (a?: Answer) => !!a?.huh || !!a?.choice || !!a?.text?.trim();
