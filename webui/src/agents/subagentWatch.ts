import { useEffect } from "react"
import { apiFetch } from "../apiFetch"
import type { Connection } from "../connection"

// Mirrors server/wire/subagent.WatchRequest (tygo does not emit it yet).
export interface SubagentWatch {
  client: string
  session: string
  call_id: string
  closed?: boolean
}

export type PostSubagentWatch = (req: SubagentWatch) => Promise<unknown>

export const postSubagentWatch: PostSubagentWatch = async (req) => {
  await apiFetch("/v1/sessions/subagent-watch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  })
}

// Asks the server to watch one subagent's file while the page is open. The server forgets a client
// when its stream closes, so each new stream asks again; a refusal (no stream yet, unknown call) is
// ignored because the read already shows what is wrong.
export function useSubagentWatch(
  conn: Connection,
  session: string,
  callId: string,
  streams: number,
  post: PostSubagentWatch = postSubagentWatch,
) {
  useEffect(() => {
    if (streams === 0) return
    void post({ client: conn.clientId, session, call_id: callId }).catch(() => {})
  }, [conn, post, session, callId, streams])
  useEffect(() => {
    const end = () =>
      void post({ client: conn.clientId, session, call_id: callId, closed: true }).catch(() => {})
    window.addEventListener("pagehide", end)
    return () => {
      window.removeEventListener("pagehide", end)
      end()
    }
  }, [conn, post, session, callId])
}
