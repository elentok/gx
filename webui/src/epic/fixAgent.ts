import { postJson } from "../design/open";
import type { FixAgentRequest, FixAgentResult } from "../generated/server";

export type FixAgent = (body: FixAgentRequest) => Promise<FixAgentResult>;

export const postFixAgent = postJson("/v1/tickets/fix-agent") as FixAgent;

/** The prompt the fix agent starts from; a pause reason follows the colon, none leaves the sentence at "needs-repair". */
export const fixPrompt = (address: string, reason?: string) =>
  `/gx-investigate ${address} is needs-repair${reason ? `: ${reason}` : ""}`;
