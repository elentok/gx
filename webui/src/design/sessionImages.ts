import { apiFetch } from "../apiFetch";
import type { SessionImageResult, SessionResult } from "../generated/server";
import { readWrite, postJson } from "./open";

/** Posts the raw bytes; resolves to the saved image id, rejects with the refusal or error message. */
export async function uploadSessionImage(sessionId: string, file: Blob): Promise<string> {
  const res = await apiFetch(`/v1/sessions/images?id=${encodeURIComponent(sessionId)}`, {
    method: "POST",
    headers: { "Content-Type": "application/octet-stream" },
    body: file,
  });
  const result = await readWrite<SessionImageResult & SessionResult>(res);
  if (result.refused || !result.id) throw new Error(result.message || "Upload failed");
  return result.id;
}

export const postDeleteSessionImage = postJson("/v1/sessions/images/delete");

export function deleteSessionImage(sessionId: string, image: string): Promise<SessionResult> {
  return postDeleteSessionImage({ id: sessionId, image });
}

export function sessionImageUrl(sessionId: string, image: string): string {
  return `/v1/sessions/images?session=${encodeURIComponent(sessionId)}&id=${encodeURIComponent(image)}`;
}
