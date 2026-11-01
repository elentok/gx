import { useEffect, useState } from "react";
import type { SessionPermissionResponse } from "../generated/server";
import { fetchJson } from "../ticket/detail";

export type FetchPermission = (sessionId: string) => Promise<SessionPermissionResponse | undefined>;

/** The dialog a Blocked session waits on. Undefined when it cannot be read: the caller falls back to a plain notice. */
export const fetchPermission: FetchPermission = async (sessionId) => {
  const res = await fetchJson<SessionPermissionResponse>(`/v1/sessions/permission?session=${encodeURIComponent(sessionId)}`, "session permission");
  return res.kind === "ok" ? res.detail : undefined;
};

type Read = { sessionId: string; value?: SessionPermissionResponse };

/** Reads the prompt once per mount: it is the text the person answers, so it must not change under them. */
export function usePermission(sessionId: string, enabled: boolean, read: FetchPermission) {
  const [done, setDone] = useState<Read>();
  useEffect(() => {
    if (!enabled) return;
    let live = true;
    read(sessionId).then(
      (value) => live && setDone({ sessionId, value }),
      () => live && setDone({ sessionId }),
    );
    return () => {
      live = false;
    };
    // `read` is a prop that tests stub; a new identity must not re-read.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, enabled]);
  const mine = done?.sessionId === sessionId ? done : undefined;
  return { loaded: !!mine, permission: mine?.value };
}
