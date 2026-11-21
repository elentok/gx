import { apiFetch } from "../apiFetch"
import type { InitiativeList, InitiativePage } from "../generated/server"
import { fetchJson, type DetailResult } from "../ticket/detail"
import type { Tone } from "../ui/tone"

export const INITIATIVE_STATE: Record<string, { label: string; tone: Tone }> = {
  active: { label: "Active", tone: "waiting" },
  stopped: { label: "Stopped", tone: "deadlocked" },
  finished: { label: "Finished", tone: "done" },
}

/** Pairs each item with a React key that stays unique when hand-edited bodies repeat an id. */
export function keyed<T>(items: T[], id: (item: T) => string): { key: string; item: T }[] {
  const seen = new Map<string, number>()
  return items.map((item) => {
    const base = id(item)
    const n = (seen.get(base) ?? 0) + 1
    seen.set(base, n)
    return { key: `${base}#${n}`, item }
  })
}

export type FetchInitiatives = () => Promise<InitiativeList>

export const fetchInitiatives: FetchInitiatives = async () => {
  const res = await apiFetch("/v1/initiatives")
  if (!res.ok) throw new Error(`initiatives: HTTP ${res.status}`)
  return res.json()
}

export type FetchInitiative = (address: string) => Promise<DetailResult<InitiativePage>>

export const fetchInitiative: FetchInitiative = (address) =>
  fetchJson(`/v1/initiatives/detail?address=${encodeURIComponent(address)}`, "initiative")
