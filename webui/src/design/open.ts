import type {
  AutoAdvanceRequest,
  DesignDeleteRequest,
  DesignOpenRequest,
  SessionInfo,
  SessionRequest,
  SessionResult,
} from "../generated/server";

export type OpenDesign = (body: DesignOpenRequest) => Promise<SessionResult>;
export type DeleteDesign = (
  body: DesignDeleteRequest,
) => Promise<SessionResult>;
export type StopSession = (body: SessionRequest) => Promise<SessionResult>;
export type ResumeSession = (body: SessionRequest) => Promise<SessionResult>;
export type InterruptSession = (body: SessionRequest) => Promise<SessionResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postJson =
  <R extends SessionResult = SessionResult>(path: string) =>
  async (body: unknown): Promise<R> => {
    const res = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const result = await res.json().catch(() => null);
    if (result && (res.ok || result.refused)) return result;
    throw new Error(`${path}: HTTP ${res.status}`);
  };

export const postOpenDesign: OpenDesign = postJson("/v1/designs/open");
export const postDeleteDesign: DeleteDesign = postJson("/v1/designs/delete");
export const postStopSession: StopSession = postJson("/v1/sessions/stop");
export const postResumeSession: ResumeSession = postJson("/v1/sessions/resume");
export const postInterruptSession: InterruptSession = postJson(
  "/v1/sessions/interrupt",
);
export type SetAutoAdvance = (
  body: AutoAdvanceRequest,
) => Promise<SessionResult>;
export type CancelAutoAdvance = (
  body: SessionRequest,
) => Promise<SessionResult>;
export const postAutoAdvance: SetAutoAdvance = postJson(
  "/v1/sessions/auto-advance",
);
export const postCancelAutoAdvance: CancelAutoAdvance = postJson(
  "/v1/sessions/auto-advance/cancel",
);

/** The server's SessionStateToken: id, state and the status time in Unix nanoseconds. */
export function stateToken(s: SessionInfo): string {
  const at = s.status_changed_at;
  const nanos = (/\.(\d+)/.exec(at)?.[1] ?? "").padEnd(9, "0").slice(0, 9);
  const seconds = Math.floor(Date.parse(at.replace(/\.\d+/, "")) / 1000);
  return `${s.id}:${s.status.state}:${BigInt(seconds) * 1_000_000_000n + BigInt(nanos)}`;
}
