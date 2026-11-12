/**
 * What a card offers besides its options: "round" (a printed round: Huh? and a note), "notes" (a note only:
 * hook and Codex cards) or "plain" (neither: a screen-read dialog, a permission or plan card).
 */
export type QuestionMode = "round" | "notes" | "plain"

/** `choices` is a multi-select answer: the ids of the chosen options, as the server reads them. */
export type Answer = {
  choice?: string
  choices?: string[]
  text?: string
  note?: string
  huh?: boolean
}
export type Answers = Record<string, Answer>

export interface QuestionChoice {
  id: string
  label: string
  consequence?: string
  /** A row the person types into: "Other", "None of the above" or "Tell Claude what to change". */
  text?: boolean
}

/** One question of a round, as plain props. A server round card has this shape. */
export interface QuestionCard {
  label: string
  title: string
  body?: string
  kind: string
  yes?: string
  no?: string
  options?: QuestionChoice[]
  recommended?: string
  /** Resources the question depends on, such as a mock's URL. */
  links?: { label: string; url: string }[]
  note?: string
  /** What the question is about, shown muted above its title. */
  context?: string
  /** Where the context leads: the ticket the question is about. */
  contextHref?: string
}

/** The title a card shows for its question: "Q8", or "Q8b". */
export const nameOf = (c: { label: string }) => `Q${c.label}`

/** A yes-no card reads Yes and No; its yes and no lines are the consequences. */
export const choicesOf = (c: QuestionCard): QuestionChoice[] =>
  c.kind === "yes-no"
    ? [
        { id: "yes", label: "Yes", consequence: c.yes },
        { id: "no", label: "No", consequence: c.no },
      ]
    : (c.options ?? [])

/** Whether the picked option or options include a text row, which needs text before the question counts. */
const needsText = (a: Answer, card?: QuestionCard) => {
  const picked = new Set([a.choice, ...(a.choices ?? [])])
  return !!card?.options?.some((o) => o.text && picked.has(o.id)) && !a.text?.trim()
}

/** With `card`, a picked text row also needs its text. Without it, any choice, text or Huh? counts. */
export const isAnswered = (a?: Answer, card?: QuestionCard) => {
  if (!a) return false
  if (a.huh) return true
  if (needsText(a, card)) return false
  return !!a.choice || !!a.choices?.length || !!a.text?.trim()
}

/** The Submit rule: a card is answered only when each of its questions is. */
export const allAnswered = (cards: QuestionCard[], answers: Answers) =>
  cards.every((c) => isAnswered(answers[c.label], c))
