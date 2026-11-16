import { useMemo } from "react"
import { useNavigate } from "react-router"
import { RestKindBedtime, RestKindBreak } from "./generated/server"
import type { RestState } from "./rest"
import { RestScreen } from "./RestScreen"

const RULE = { work_minutes: 50, break_minutes: 10, bedtime_start: "23:00", bedtime_end: "07:00" }

function demoState(kind: string): RestState {
  const now = Date.now()
  const minutes = kind === RestKindBreak ? 10 : 480
  return {
    counting: false,
    now: new Date(now).toISOString(),
    rule: RULE,
    period: {
      kind,
      start: new Date(now).toISOString(),
      end: new Date(now + minutes * 60_000).toISOString(),
      message: kind === RestKindBreak ? "Stand up and stretch." : "It is late. Go to sleep.",
      sub_message: kind === RestKindBreak ? "Look at something far away." : undefined,
    },
  }
}

// Demo routes /rest/break and /rest/bedtime: the real Rest screen on made-up server state.
export function RestDemoPage({ kind }: { kind: typeof RestKindBreak | typeof RestKindBedtime }) {
  const navigate = useNavigate()
  const state = useMemo(() => demoState(kind), [kind])
  return <RestScreen demo={state} onExit={() => navigate("/")} />
}
