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
