import type { SessionScreenResponse } from "../generated/server";
import { fetchJson, type DetailResult } from "../ticket/detail";

export type FetchScreen = (target: string) => Promise<DetailResult<SessionScreenResponse>>;

/** A design session's screen, by session id. */
export const fetchScreen: FetchScreen = (sessionId) =>
  fetchJson(`/v1/sessions/screen?session=${encodeURIComponent(sessionId)}`, "session screen");

/** A loop ticket agent's screen, by ticket address. */
export const fetchTicketScreen: FetchScreen = (address) =>
  fetchJson(`/v1/sessions/screen?ticket=${encodeURIComponent(address)}`, "ticket screen");
