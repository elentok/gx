import type { SessionScreenResponse } from "../generated/server";
import { fetchJson, type DetailResult } from "../ticket/detail";

export type FetchScreen = (sessionId: string) => Promise<DetailResult<SessionScreenResponse>>;

export const fetchScreen: FetchScreen = (sessionId) =>
  fetchJson(`/v1/sessions/screen?session=${encodeURIComponent(sessionId)}`, "session screen");
