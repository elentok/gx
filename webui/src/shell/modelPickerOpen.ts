import { useSyncExternalStore } from "react"
import { apiFetch } from "../apiFetch"
import { postJson } from "../design/open"
import type {
  SessionInfo,
  SessionModelsResponse,
  SessionResult,
  SessionSwitchRequest,
} from "../generated/server"

export type FetchModels = (sessionId: string) => Promise<SessionModelsResponse | null>
export type SwitchModel = (body: SessionSwitchRequest) => Promise<SessionResult>

export type ModelPickerRequest = {
  session: SessionInfo
  /** Override the real calls; tests inject fakes. */
  fetchModels?: FetchModels
  switchModel?: SwitchModel
}

/** The model list of the session's harness, or null when the route fails (the picker then shows the current model only). */
export const fetchSessionModels: FetchModels = async (id) => {
  try {
    const res = await apiFetch(`/v1/sessions/models?session=${encodeURIComponent(id)}`)
    return res.ok ? ((await res.json()) as SessionModelsResponse) : null
  } catch {
    return null
  }
}

export const postSwitchSession: SwitchModel = postJson("/v1/sessions/switch")

// App-level state like renameOpen: the chord opens one picker and only its host re-renders.
let request: ModelPickerRequest | null = null
const listeners = new Set<() => void>()

function set(next: ModelPickerRequest | null) {
  request = next
  listeners.forEach((l) => l())
}

export const openModelPicker = (open: ModelPickerRequest) => set(open)
export const closeModelPicker = () => set(null)

export function useModelPickerRequest(): ModelPickerRequest | null {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => request,
  )
}
