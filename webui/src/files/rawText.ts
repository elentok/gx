import { apiFetch } from "../apiFetch"
import { rawUrl } from "./rawUrl"

/** The text of a hosted file. Resolves undefined on a 404; rejects on any other failure. */
export type FetchFileText = (path: string) => Promise<string | undefined>

export const fetchRawText: FetchFileText = async (path) => {
  // HTML answers with a redirect to its token route; fetch follows it.
  const res = await apiFetch(rawUrl(path), { credentials: "same-origin" })
  if (res.status === 404) return undefined
  if (!res.ok) throw new Error(`raw ${res.status}`)
  return res.text()
}
