import { apiFetch } from "../apiFetch"
import type { SessionModelsResponse } from "../generated/server"

/** What a chat launches with when the draft leaves the model and effort empty; empty strings when the server cannot say. */
export interface LaunchDefaults {
  model: string
  effort: string
}

export type FetchDefaults = (harness: string) => Promise<LaunchDefaults>

export const fetchLaunchDefaults: FetchDefaults = async (harness) => {
  try {
    const res = await apiFetch(`/v1/sessions/models?harness=${encodeURIComponent(harness)}`)
    if (!res.ok) return { model: "", effort: "" }
    const body = (await res.json()) as Partial<SessionModelsResponse>
    return { model: body.default_model ?? "", effort: body.default_effort ?? "" }
  } catch {
    return { model: "", effort: "" }
  }
}
