import { tierLabel } from "../tiers"
import type { SessionInfo } from "../generated/server"
import { modelName, type ModelLabel } from "../modelName"

export interface AgentDetails {
  model: ModelLabel
  effort: string
  tier: string
  switched: boolean
}

/** What the Design header and its menu's Agent block show for a session; the header shows the model, the menu the rest. */
export function agentDetails(s: SessionInfo): AgentDetails {
  const known = s.tier || s.model || s.effort
  const label = tierLabel(s.tier)
  // A session with a config but no tier or model still ran as the Worker tier.
  const defaultsToWorker = known && !s.model && !label
  return {
    model: modelName(s.model ?? "", s.harness),
    effort: s.effort ?? "",
    tier: defaultsToWorker ? tierLabel("worker") : label,
    switched: s.model_switched ?? false,
  }
}
