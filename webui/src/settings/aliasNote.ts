import type { ModelAlias, SettingsEntry } from "../generated/server"
import { apiFetch } from "../apiFetch"
import { useFetched } from "../useFetched"

/** The Claude model keys: agents.claude.model and the tier variants. Codex has no aliases. */
const CLAUDE_MODEL_KEY = /^agents\.claude\.(?:[a-z]+\.)?model$/

/** The Claude aliases with what they resolve to, or none when the route fails (no note then). */
export async function fetchClaudeAliases(): Promise<ModelAlias[]> {
  try {
    const res = await apiFetch("/v1/sessions/models?harness=claude")
    if (!res.ok) return []
    return ((await res.json()) as { aliases?: ModelAlias[] }).aliases ?? []
  } catch {
    return []
  }
}

/** The Claude aliases, fetched once the caller mounts; undefined until they arrive. */
export const useClaudeAliases = (): ModelAlias[] | undefined =>
  useFetched("settings-claude-aliases", fetchClaudeAliases).data ?? undefined

/** The note under a Claude model setting whose alias resolves to a model that is not the family's latest. */
export function aliasNote(entry: SettingsEntry, aliases: ModelAlias[] | undefined): string | null {
  if (!CLAUDE_MODEL_KEY.test(entry.key) || typeof entry.value !== "string") return null
  const hit = aliases?.find((a) => a.alias === entry.value)
  if (!hit?.latest || hit.latest === hit.id) return null
  return `${hit.alias} resolves to ${hit.id}. ${hit.latest} is newer. Enter the full id to use it.`
}

/** The harness a model key belongs to: agents.<harness>.model and its tier variants. */
const MODEL_KEY = /^agents\.([a-z]+)\.(?:[a-z]+\.)?model$/
export const modelKeyHarness = (key: string): string | undefined => MODEL_KEY.exec(key)?.[1]

/** The names a model key takes: the harness's aliases and model ids. Empty when the route fails. */
export async function fetchModelNames(harness: string): Promise<string[]> {
  try {
    const res = await apiFetch(`/v1/sessions/models?harness=${encodeURIComponent(harness)}`)
    if (!res.ok) return []
    const body = (await res.json()) as {
      aliases?: ModelAlias[]
      models?: { id: string }[]
    }
    return [
      ...new Set([
        ...(body.aliases ?? []).map((a) => a.alias),
        ...(body.models ?? []).map((m) => m.id),
      ]),
    ]
  } catch {
    return []
  }
}

/** The completions for a model key's input; empty for any other key, which starts no fetch. */
export function useModelNames(harness: string | undefined): string[] {
  return (
    useFetched(`settings-model-names-${harness ?? ""}`, () =>
      harness ? fetchModelNames(harness) : Promise.resolve([]),
    ).data ?? []
  )
}
