import {
  TargetRefused,
  type AutoAdvanceRequest,
  type AutoQueueRequest,
  type DesignDeleteRequest,
  type DesignOpenRequest,
  type SessionInfo,
  type SessionRenameRequest,
  type SessionRequest,
  type SessionResult,
  type TargetResult,
  type TargetsResult,
} from "../generated/server"
import { apiFetch } from "../apiFetch"
import { WriteError } from "../useWrite"

export type OpenDesign = (body: DesignOpenRequest) => Promise<SessionResult>
/** Opens a stepped flow's session. `epic` is the flow's key: a Design's map address, a Triage's project. */
export type OpenFlow = (
  body: Pick<DesignOpenRequest, "epic" | "new" | "state">,
) => Promise<SessionResult>
export type DeleteDesign = (body: DesignDeleteRequest) => Promise<SessionResult>
export type StopSession = (body: SessionRequest) => Promise<SessionResult>
export type ResumeSession = (body: SessionRequest) => Promise<SessionResult>
export type InterruptSession = (body: SessionRequest) => Promise<SessionResult>

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postJson =
  <R extends object = SessionResult>(path: string) =>
  async (body: unknown): Promise<R> => {
    const res = await apiFetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })
    return readWrite<R>(res)
  }

/** A JSON answer, or a refusal; any other answer throws the server's plain-text body for the toast. */
export async function readWrite<R extends object>(res: Response): Promise<R> {
  const text = await res.text().catch(() => "")
  let result: R | null = null
  try {
    result = JSON.parse(text)
  } catch {
    // Not JSON: an error body, handled below.
  }
  // `refused` is the only field read; a many-target envelope has none, so R is not constrained to it.
  if (result && (res.ok || (result as { refused?: boolean }).refused)) return result
  throw new WriteError(text.trim() || `HTTP ${res.status}`)
}

export const postOpenDesign: OpenDesign = postJson("/v1/designs/open")
export const postDeleteDesign: DeleteDesign = postJson("/v1/designs/delete")
/** POSTs to a many-target route: one result per target in the server's answer. */
export async function postTargetsTo<Req>(path: string, body: Req): Promise<TargetResult[]> {
  const res = await postJson<TargetsResult>(path)(body)
  // A refused answer (HTTP 4xx with refused: true) carries no results.
  return res.results ?? []
}

/** The refusable shape `useWrite` reads, from one target's result. */
export const refusalOf = (r: TargetResult): SessionResult =>
  r.status === TargetRefused ? { refused: true, reason: r.reason, message: r.message } : {}

/** A stop or remove of one session on the many-target route, answered as the refusable shape the pages use. `body.state` is a rowToken. */
export const postTarget =
  (path: string, yes = false) =>
  async ({ id, state }: SessionRequest): Promise<SessionResult> => {
    const [r] = await postTargetsTo(path, { targets: [{ id, state }], yes })
    return r ? refusalOf(r) : {}
  }

export const postStopSession: StopSession = postTarget("/v1/sessions/stop")
export type RenameSession = (body: SessionRenameRequest) => Promise<SessionResult>
export const postRenameSession: RenameSession = postJson("/v1/sessions/rename")
export const postResumeSession: ResumeSession = postJson("/v1/sessions/resume")
export const postInterruptSession: InterruptSession = postJson("/v1/sessions/interrupt")
export type SetAutoAdvance = (body: AutoAdvanceRequest) => Promise<SessionResult>
export type CancelAutoAdvance = (body: SessionRequest) => Promise<SessionResult>
export const postAutoAdvance: SetAutoAdvance = postJson("/v1/sessions/auto-advance")
export const postCancelAutoAdvance: CancelAutoAdvance = postJson("/v1/sessions/auto-advance/cancel")
export type SetAutoQueue = (body: AutoQueueRequest) => Promise<SessionResult>
export const postAutoQueue: SetAutoQueue = postJson("/v1/sessions/auto-queue")
export const postCancelAutoQueue: CancelAutoAdvance = postJson("/v1/sessions/auto-queue/cancel")

/** The server's SessionStateToken: id, state and the status time in Unix nanoseconds. */
export function stateToken(s: SessionInfo): string {
  const at = s.status_changed_at
  const nanos = (/\.(\d+)/.exec(at)?.[1] ?? "").padEnd(9, "0").slice(0, 9)
  const seconds = Math.floor(Date.parse(at.replace(/\.\d+/, "")) / 1000)
  return `${s.id}:${s.status.state}:${BigInt(seconds) * 1_000_000_000n + BigInt(nanos)}`
}

/** The server's SessionRowToken, sent with a stop or remove: id, run_started_at as the page holds it, and live or exited. A Working/Idle flip leaves it alone. */
export function rowToken(s: SessionInfo): string {
  return `${s.id}:${s.run_started_at}:${s.status.state === "exited" ? "exited" : "live"}`
}
