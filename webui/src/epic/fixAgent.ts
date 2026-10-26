import { postJson } from "../design/open";
import type { FixAgentRequest, FixAgentResult } from "../generated/server";

export type FixAgent = (body: FixAgentRequest) => Promise<FixAgentResult>;

export const postFixAgent: FixAgent = postJson("/v1/tickets/fix-agent");
