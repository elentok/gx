import { useEffect, useState } from "react";
import type { Connection } from "../connection";
import type { FocusReport } from "../generated/server";

export type PostFocus = (report: FocusReport) => Promise<unknown>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postFocus: PostFocus = async (report) => {
  await fetch("/v1/sessions/focus", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(report),
  });
};

const visible = () => document.visibilityState === "visible";

// Reports the tab's focus (its visibility) and the session on screen, again on every change.
// A new event stream also reports again: the server forgets a client when its stream closes.
export function useFocusReport(conn: Connection, session: string | undefined, streams: number, post: PostFocus = postFocus) {
  const [focused, setFocused] = useState(visible);
  useEffect(() => {
    const on = () => setFocused(visible());
    document.addEventListener("visibilitychange", on);
    return () => document.removeEventListener("visibilitychange", on);
  }, []);
  useEffect(() => {
    if (streams === 0) return;
    void post({ client: conn.clientId, focused, session: session ?? "" }).catch(() => {});
  }, [conn, post, focused, session, streams]);
  // Keyed on conn and post only: a stream or session change must not post a transient "left" that could land after the next report.
  useEffect(
    () => () => {
      void post({ client: conn.clientId, focused: false, session: "" }).catch(() => {});
    },
    [conn, post],
  );
}
