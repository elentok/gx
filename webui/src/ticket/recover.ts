import {
  ActionChooseBase,
  ActionMerge,
  ActionReopenAndRetry,
  ActionResume,
  ActionRetry,
  ActionStartOver,
} from "../generated/server";
import { WriteError, type Refusable } from "../useWrite";

export const ReasonAgentWorking = "agent-working";

export interface RecoverBody {
  address: string;
  /** The page's state token; the server refuses the write if the pause moved on. */
  state: string;
  /** The chosen blocker of an ambiguous-base pause. */
  base?: string;
  /** Retry only: stop the working agent first. */
  stop?: boolean;
  /** Start over only: why the work is thrown away. */
  reason?: string;
}

export type PostRecover = (action: string, body: RecoverBody) => Promise<Refusable>;

// The web route each button posts to. The resume route also serves the verbs that lift a pause.
const ROUTES: Record<string, string> = {
  [ActionResume]: "/v1/tickets/resume",
  [ActionChooseBase]: "/v1/tickets/resume",
  [ActionReopenAndRetry]: "/v1/tickets/resume",
  [ActionRetry]: "/v1/tickets/retry",
  [ActionMerge]: "/v1/tickets/merge",
  [ActionStartOver]: "/v1/tickets/start-over",
};

/** Whether the card has a button for this action; the rest keep their own forms. */
export const isRecoverAction = (action: string) => action in ROUTES;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postRecover: PostRecover = async (action, body) => {
  const res = await fetch(ROUTES[action]!, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
  const result = await res.json().catch(() => null);
  if (result) return result;
  throw new WriteError(`HTTP ${res.status}`);
};
