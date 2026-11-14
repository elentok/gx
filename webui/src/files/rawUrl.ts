import { encodePath } from "./viewerUrl"

/** The raw route of a hosted file. `download` asks for an attachment. */
export function rawUrl(path: string, { download = false } = {}): string {
  return `/v1/files${encodePath(path)}${download ? "?download=1" : ""}`
}
