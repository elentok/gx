import {
  ActionChooseBase,
  ActionKeepWaiting,
  ActionMerge,
  ActionReopenAndRetry,
  ActionResume,
  ActionResumeOnWorker,
  ActionRetry,
  ActionStartOver,
  ActionStopTask,
  type Action,
  type TicketDetail,
} from "../generated/server"
import type { Entity } from "../actions/entity"
import { apiFetch } from "../apiFetch"
import { createExternalStore } from "../shell/externalStore"
import { WriteError, type Refusable } from "../useWrite"

export const ReasonAgentWorking = "agent-working"

/** Not a detail action: the page offers Cancel for a One-off from its address and status. */
export const ActionCancel = "cancel"

export const DISCARD_NOTICE =
  "Start over discards the branch's commits and runs the ticket again from the base. A backup branch keeps them."

export const CANCEL_NOTICE =
  "Cancel ends this run. If it belongs to a Schedule, later runs of that Schedule may start."

// A paused ticket reaches the page as its legacy word, never "paused". oldword:ok
const CANCELLABLE_STATUSES = ["open", "claimed", "needs-answer", "needs-repair"] // oldword:ok

// How many recovery writes are in flight, across the page's footer and the shared confirm. One
// source, so the footer buttons and the Start over / Cancel run actions disable together.
const inFlight = createExternalStore(0)
export const useRecoverPending = () => inFlight.use() > 0

/** Runs one recovery write and counts it while it is pending. */
export async function trackRecover<R>(call: () => Promise<R>): Promise<R> {
  inFlight.set(inFlight.get() + 1)
  try {
    return await call()
  } finally {
    inFlight.set(inFlight.get() - 1)
  }
}

/** The ticket page's registry entity: its address, the server's action list and its status word. */
export const ticketEntity = (detail: TicketDetail): Entity => ({
  kind: "ticket",
  key: detail.address,
  name: detail.title,
  surface: "page",
  serverActions: detail.actions as Action[],
  status: detail.status,
})

/** A One-off that has not ended. The server refuses any other ticket. */
export const canCancel = (detail: { address: string; status: string }) =>
  /^[^:]+:one-offs\//.test(detail.address) && CANCELLABLE_STATUSES.includes(detail.status)

export interface RecoverBody {
  address: string
  /** The page's state token; the server refuses the write if the pause moved on. */
  state: string
  /** The chosen blocker of an ambiguous-base pause. */
  base?: string
  /** Retry only: stop the working agent first. */
  stop?: boolean
  /** Start over only: why the work is thrown away. */
  reason?: string
  /** Resume only: a one-time tier for the ticket's next launch. */
  tier?: string
}

export type PostRecover = (action: string, body: RecoverBody) => Promise<Refusable>

// The web route each button posts to. The resume route also serves the verbs that lift a pause.
// stash-merge has no route here yet, so the map is partial over the Action union.
const ROUTES: Partial<Record<Action | typeof ActionCancel, string>> = {
  [ActionCancel]: "/v1/tickets/cancel",
  [ActionResume]: "/v1/tickets/resume",
  [ActionResumeOnWorker]: "/v1/tickets/resume",
  [ActionChooseBase]: "/v1/tickets/resume",
  [ActionReopenAndRetry]: "/v1/tickets/resume",
  [ActionRetry]: "/v1/tickets/retry",
  [ActionKeepWaiting]: "/v1/tickets/keep-waiting",
  [ActionStopTask]: "/v1/tickets/stop-task",
  [ActionMerge]: "/v1/tickets/merge",
  [ActionStartOver]: "/v1/tickets/start-over",
}

/** Whether the card has a button for this action; the rest keep their own forms. */
export const isRecoverAction = (action: string) => action !== ActionCancel && action in ROUTES

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postRecover: PostRecover = async (action, body) => {
  const route = ROUTES[action as Action | typeof ActionCancel]
  if (!route) throw new WriteError(`no route for ${action}`)
  const res = await apiFetch(route, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
  const result = await res.json().catch(() => null)
  if (result) return result
  throw new WriteError(`HTTP ${res.status}`)
}
