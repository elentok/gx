import type { SessionImageResult } from "../generated/server"
import { fetchJson, type DetailResult } from "../ticket/detail"

export type FetchMapImages = (address: string) => Promise<DetailResult<SessionImageResult[]>>

/** The ids of the map's images/ folder, in order; a Design with no folder lists none. */
export const fetchMapImages: FetchMapImages = (address) =>
  fetchJson(`/v1/designs/images?epic=${encodeURIComponent(address)}`, "map images")

export function mapImageUrl(address: string, image: string): string {
  return `/v1/designs/images/read?epic=${encodeURIComponent(address)}&id=${encodeURIComponent(image)}`
}
