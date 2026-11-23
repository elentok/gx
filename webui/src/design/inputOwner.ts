import { useCallback, useEffect, useRef, useState } from "react"
import { apiFetch } from "../apiFetch"
import {
  AttachEventBlocked,
  AttachEventOwner,
  type SessionAttachEvent,
  type SessionAttachRequest,
} from "../generated/server"
import { useLatest } from "../hooks"
import { readWrite } from "./open"

export type InputOwner = "self" | "other" | "gx"

/** Opens the attach event stream for one client; returns the function that closes it. */
export type SubscribeOwner = (
  target: string,
  client: string,
  onEvent: (ev: SessionAttachEvent) => void,
) => () => void
export type TakeOver = (target: string, client: string) => Promise<unknown>

export const subscribeOwner: SubscribeOwner = (target, client, onEvent) => {
  if (typeof EventSource === "undefined") return () => {}
  const q = new URLSearchParams({ id: target, client })
  const es = new EventSource(`/v1/sessions/attach/events/?${q}`)
  const listen = (type: string) =>
    es.addEventListener(type, (e) => {
      try {
        onEvent({ ...JSON.parse((e as MessageEvent<string>).data), type })
      } catch {
        // A malformed event is dropped; the next owner report corrects the state.
      }
    })
  listen(AttachEventOwner)
  listen(AttachEventBlocked)
  return () => es.close()
}

export const postTakeOver: TakeOver = async (target, client) => {
  const body: SessionAttachRequest = { id: target, client }
  const res = await apiFetch("/v1/sessions/attach/takeover", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  return readWrite(res)
}

export const BLOCKED_HINT_MS = 4000

/** Who owns the session's input, and whether this client's last key was blocked. */
export function useInputOwner(opts: {
  target: string
  enabled: boolean
  subscribe: SubscribeOwner
  takeOver: TakeOver
  hintMs?: number
}) {
  const [owner, setOwner] = useState<InputOwner>()
  const [blocked, setBlocked] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const client = useRef(crypto.randomUUID())
  const latest = useLatest(opts)
  const { target, enabled, subscribe } = opts

  const clearHint = useCallback(() => {
    clearTimeout(timer.current)
    setBlocked(false)
  }, [])

  useEffect(() => {
    if (!enabled) return
    const close = subscribe(target, client.current, (ev) => {
      if (ev.type === AttachEventOwner) {
        setOwner(ev.self ? "self" : ev.gx ? "gx" : "other")
        if (ev.self) clearHint()
      } else if (ev.type === AttachEventBlocked) {
        setBlocked(true)
        clearTimeout(timer.current)
        timer.current = setTimeout(
          () => setBlocked(false),
          latest.current.hintMs ?? BLOCKED_HINT_MS,
        )
      }
    })
    return () => {
      close()
      clearTimeout(timer.current)
    }
  }, [target, enabled, subscribe, clearHint])

  const takeOver = useCallback(async () => {
    clearHint()
    await latest.current.takeOver(latest.current.target, client.current)
  }, [clearHint])

  return { owner, blocked, takeOver }
}
