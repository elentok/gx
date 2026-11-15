import type {
  TriageAbandonRequest,
  TriageEditResult,
  TriageRowActionRequest,
} from "../generated/server"
import { postJson } from "../design/open"
import type { OpenFlow } from "../design/open"

const postTriageStart = postJson("/v1/triage/open")
/** `epic` is the flow's key; for a Triage it is the project. */
export const postOpenTriage: OpenFlow = ({ epic, ...rest }) =>
  postTriageStart({ project: epic, ...rest })
export type AbandonTriage = (body: TriageAbandonRequest) => Promise<TriageEditResult>
export const postAbandonTriage: AbandonTriage = postJson<TriageEditResult>("/v1/triage/abandon")
export type TriageRowAction = (body: TriageRowActionRequest) => Promise<TriageEditResult>
export const postRetryTriageRow: TriageRowAction = postJson<TriageEditResult>("/v1/triage/retry")
export const postKeepTriageRow: TriageRowAction = postJson<TriageEditResult>("/v1/triage/keep")
