import { useEffect, useState } from "react"
import { Text } from "@chakra-ui/react"
import type { SessionInfo } from "../generated/server"
import { modelName } from "../modelName"

/** A switch (or the switch back after a failure) is running: the server holds the model it relaunches with. */
export const switchRunning = (s: SessionInfo) => !!s.switching_model

/** Why a switch cannot run right now, or null when it can. The picker and the footer popover share it. */
export function blockedNote(s: SessionInfo): string | null {
  if (s.status.state === "exited" || s.stopped) return "Resume the session to switch."
  // A rollback can leave the state idle while the switch still runs.
  if (s.status.state !== "idle" || switchRunning(s))
    return "The agent is working. Wait until it is idle to switch."
  return null
}

const label = (s: SessionInfo, id: string | undefined) => modelName(id ?? "", s.harness)

type Seen = { sessionId: string; model: string; back: boolean }

/** The one line for the current switch phase, or nothing. "Switched" is remembered here: the server only clears its fields. */
export function useSwitchLine(s: SessionInfo): { text: string; red: boolean } | undefined {
  const [seen, setSeen] = useState<Seen>()
  const running = switchRunning(s)
  const model = s.switching_model ?? ""
  const back = !!s.switching_back
  useEffect(() => {
    if (running) setSeen({ sessionId: s.id, model, back })
  }, [running, s.id, model, back])
  if (running) {
    return {
      // The back target is the launch value, which can be an alias; the footer shows s.model.
      text: back ? `Switching back to ${label(s, s.model)}…` : `Switching to ${label(s, model)}…`,
      red: false,
    }
  }
  if (s.switch_failed_model) {
    return {
      text: `Switch to ${label(s, s.switch_failed_model)} failed. Still on ${label(s, s.model)}.`,
      red: true,
    }
  }
  if (seen && seen.sessionId === s.id && !seen.back) {
    return { text: `Switched to ${label(s, seen.model)}.`, red: false }
  }
}

export const TOKEN_COST_WARNING =
  "Changing the model costs tokens. The whole chat is sent again at the new model's price."

/** The token cost warning for a chat with history. The picker and the footer popover share it; it never blocks a switch. */
export function TokenCostWarning({ session }: { session: SessionInfo }) {
  if (!session.has_history) return null
  return (
    <Text color="text.dim" fontSize="xs" data-testid="token-cost-warning">
      {TOKEN_COST_WARNING}
    </Text>
  )
}

/** The centered system line at the end of a chat log. It is not part of the transcript. */
export function SwitchLine({ session }: { session: SessionInfo }) {
  const line = useSwitchLine(session)
  if (!line) return null
  return (
    <Text
      role="status"
      data-testid="switch-line"
      alignSelf="center"
      fontSize="xs"
      textAlign="center"
      color={line.red ? "tone.deadlocked.fg" : "text.dim"}
      borderWidth="1px"
      borderColor={line.red ? "tone.deadlocked.fg/40" : "border"}
      borderRadius="full"
      px="3"
      py="0.5"
    >
      {line.text}
    </Text>
  )
}
