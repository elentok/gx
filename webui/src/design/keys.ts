import { apiFetch } from "../apiFetch"
import type {
  AttachKey,
  SessionAttachRequest,
  SessionKeysRequest,
  SessionResult,
} from "../generated/server"
import { readWrite } from "./open"

export type PostKeys = (body: SessionKeysRequest) => Promise<SessionResult>
export type PostInput = (body: SessionAttachRequest) => Promise<SessionResult>

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postKeys: PostKeys = async (body) => {
  const res = await apiFetch("/v1/sessions/keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  return readWrite(res)
}

// The key codes the holder attach takes: a rune for a plain key, the terminal library's
// extended codes for the arrows. Mod 4 is Ctrl.
export const ATTACH_KEYS: Record<string, AttachKey> = {
  esc: { code: 27 },
  enter: { code: 13 },
  tab: { code: 9 },
  up: { code: 1114113 },
  down: { code: 1114114 },
  right: { code: 1114115 },
  left: { code: 1114116 },
  "ctrl+c": { code: 99, mod: 4 },
}

// The attach input route returns no screen, so the caller waits this long for the child to react.
export const INPUT_SETTLE_MS = 400

export const postInput: PostInput = async (body) => {
  const res = await apiFetch("/v1/sessions/attach/input", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  const out = await readWrite(res)
  await new Promise((r) => setTimeout(r, INPUT_SETTLE_MS))
  return out
}
