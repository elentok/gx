import { Box, Text } from "@chakra-ui/react"
import { apiFetch } from "../apiFetch"
import {
  BlockingDraft,
  type SessionInfo,
  type SessionRequest,
  type SessionResult,
} from "../generated/server"
import { Button } from "../ui/button/Button"
import { refusalText, useWrite } from "../useWrite"
import {
  OWNER_TEXT,
  postTakeOver,
  subscribeOwner,
  useInputOwner,
  type SubscribeOwner,
  type TakeOver,
} from "./inputOwner"
import { readWrite } from "./open"

export type PostRetry = (body: SessionRequest) => Promise<SessionResult>

/** Retry runs the unsent steps as the TUI's `a` does; the server clears the draft first. */
export const postRetry: PostRetry = async (body) => {
  const res = await apiFetch("/v1/sessions/run-action", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  return readWrite(res)
}

/** Whether the session carries a gx card that Retry can run again. */
export const isBlockingCard = (s: SessionInfo) =>
  !!s.actionable?.gx_raised && !!s.actionable.blocking && !!s.actionable.reason

/**
 * The gx card of a prompt that did not land ("input would not clear", "agent still starting"): its reason and Retry.
 * The draft card also names who owns the input and offers Take over, so a person can clear the draft.
 */
export function BlockingCard({
  session,
  retry = postRetry,
  subscribe = subscribeOwner,
  takeOver = postTakeOver,
  onDone,
}: {
  session: SessionInfo
  retry?: PostRetry
  subscribe?: SubscribeOwner
  takeOver?: TakeOver
  onDone?: () => void
}) {
  const draft = session.actionable.blocking === BlockingDraft
  const input = useInputOwner({ target: session.id, enabled: draft, subscribe, takeOver })
  const { run, pending, refusal, error } = useWrite({ toastRefusal: false })
  const problem = refusal ? refusalText(refusal) : error
  return (
    <Box
      role="region"
      aria-label="gx stopped"
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      borderRadius="md"
      p="3"
      display="flex"
      flexDirection="column"
      gap="2"
    >
      <Text fontWeight="700" color="tone.waiting.fg">
        gx stopped
      </Text>
      <Text fontSize="sm">{session.actionable.reason}</Text>
      {draft && input.owner && (
        <Text fontSize="sm" color="text.muted" role="status">
          {OWNER_TEXT[input.owner]}
        </Text>
      )}
      {problem && <Text role="alert">{problem}</Text>}
      <Box display="flex" gap="2" flexWrap="wrap">
        <Button
          emphasis="secondary"
          disabled={pending}
          onClick={() => run(() => retry({ id: session.id }), { onDone })}
        >
          Retry
        </Button>
        {draft && input.owner && input.owner !== "self" && (
          <Button emphasis="secondary" onClick={() => void input.takeOver()}>
            Take over
          </Button>
        )}
      </Box>
    </Box>
  )
}
