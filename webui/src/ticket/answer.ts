import { apiFetch } from "../apiFetch"
import { isAnswered, type Answer } from "../ui/question-sheet/round"

export interface AnswerBody {
  address: string
  state: string
  /** An option id, or "yes"/"no". Empty for a free-text answer. */
  choice?: string
  text?: string
  note?: string
}

export type AnswerResult =
  | { outcome: string; queue?: { address: string }[] }
  | { refused: true; reason: string; message: string }

export const OutcomeDelivered = "delivered"
export const OutcomeQueued = "queued"
export const OutcomeAdopted = "adopted"
export const ReasonStale = "stale"

/** What the page shows after a successful send; seq says which question it answered. */
export interface Sent {
  seq: number
  outcome: string
  /** 1-based place in the queue, for a queued answer. */
  position?: number
  /** What the person sent, for the card of an adopted send: that outcome records no answer of its own. */
  answer?: string
}

export function sentOf(
  seq: number,
  address: string,
  res: { outcome: string; queue?: { address: string }[] },
  answer?: string,
): Sent {
  const place = (res.queue ?? []).findIndex((q) => q.address === address) + 1
  return {
    seq,
    outcome: res.outcome,
    ...(place > 0 ? { position: place } : {}),
    ...(answer ? { answer } : {}),
  }
}

/** Whether the send reached an agent that had already finished, so no answer was recorded. */
export const wasAdopted = (sent: Sent | null): boolean => sent?.outcome === OutcomeAdopted

export const HowAnswer = "answer"

/** Whether the round was closed by a person answering it on the web (not by a recovery verb). */
export const answeredByPerson = (latest: { answer?: { how?: string } } | null | undefined) =>
  latest?.answer?.how === HowAnswer

/** Whether the answered round shows the "Answer sent" card: a person's answer or an adopted send. */
export const showsSentCard = (
  latest: { answer?: { how?: string } } | null | undefined,
  sent: Sent | null,
) => answeredByPerson(latest) || wasAdopted(sent)

/**
 * The one rule for whether a round shows an answered card. A person's answer shows while it is
 * queued or while this page knows the send; a delivered answer claims the ticket again, so a reload
 * drops the card. A recovery verb's answer always shows, in the old compact layout. An adopted send
 * records no answer but shows too.
 */
export const isAnsweredRound = (
  detail: { answered: boolean; latest?: { seq: number; answer?: { how?: string } } | null },
  sent: Sent | null,
): boolean => {
  const { latest } = detail
  if (wasAdopted(sent)) return true
  if (!latest?.answer) return false
  return !answeredByPerson(latest) || detail.answered || sent?.seq === latest.seq
}

/** Free text needs its text; every other kind is a choice. */
export const isTextKind = (kind: string | undefined): boolean =>
  kind !== "yes-no" && kind !== "options"

/** A pick or text; a choice question may also be answered by a note alone. */
export const isReady = (answer: Answer, isText: boolean): boolean =>
  isAnswered(answer) || (!isText && !!answer.note?.trim())

export function answerBody(
  detail: { address: string; state?: string },
  answer: Answer,
  isText: boolean,
): AnswerBody {
  return {
    address: detail.address,
    state: detail.state ?? "",
    ...(answer.huh
      ? { huh: true }
      : isText
        ? { text: answer.text }
        : answer.choice
          ? { choice: answer.choice }
          : {}),
    ...(!isText && answer.note?.trim() ? { note: answer.note } : {}),
  }
}

export const refusalText = (res: { reason: string; message: string }): string =>
  res.reason === "agent-not-ready"
    ? `${res.message} Approve the agent's prompt first, then send again.`
    : res.message

export type PostAnswer = (body: AnswerBody) => Promise<AnswerResult>

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postAnswer: PostAnswer = async (body) => {
  const res = await apiFetch("/v1/tickets/answer", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  const result = await res.json().catch(() => null)
  if (result && ("outcome" in result || result.refused)) return result
  throw new Error(`answer: HTTP ${res.status}`)
}
