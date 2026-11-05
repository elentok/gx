import { TIERS } from "../agents/NewSessionForm";
import type { SessionInfo } from "../generated/server";
import { modelName, type ModelLabel } from "../modelName";

export interface AgentDetails {
  model: ModelLabel;
  effort: string;
  tier: string;
  switched: boolean;
}

/** What the Design header and its menu's Agent block show for a session; the header shows the model, the menu the rest. */
export function agentDetails(s: SessionInfo): AgentDetails {
  const known = s.tier || s.model || s.effort;
  const tierLabel = s.tier ? (TIERS.find((t) => t.id === s.tier)?.label ?? "") : "";
  return {
    model: modelName(s.model ?? "", s.harness),
    effort: s.effort ?? "",
    // A session with a tier-less, model-less config still ran as the Worker tier.
    tier: known && !s.model && !tierLabel ? (TIERS.find((t) => t.id === "worker")?.label ?? "") : tierLabel,
    switched: s.model_switched ?? false,
  };
}
