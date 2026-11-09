import type {
  TriageAbandonRequest,
  TriageApproveRequest,
  TriageEditResult,
  TriageRowActionRequest,
  TriageRowRequest,
  TriageSendBackRequest,
} from "../generated/server";
import { postJson } from "../design/open";
import type { OpenFlow } from "../design/open";

const postTriageStart = postJson("/v1/triage/open");
/** `epic` is the flow's key; for a Triage it is the project. */
export const postOpenTriage: OpenFlow = ({ epic, ...rest }) => postTriageStart({ project: epic, ...rest });
export type AbandonTriage = (body: TriageAbandonRequest) => Promise<TriageEditResult>;
export const postAbandonTriage: AbandonTriage = postJson<TriageEditResult>("/v1/triage/abandon");
export type SaveTriageRow = (body: TriageRowRequest) => Promise<TriageEditResult>;
export const postSaveTriageRow: SaveTriageRow = postJson<TriageEditResult>("/v1/triage/row");
export type TriageRowAction = (body: TriageRowActionRequest) => Promise<TriageEditResult>;
export const postRetryTriageRow: TriageRowAction = postJson<TriageEditResult>("/v1/triage/retry");
export const postKeepTriageRow: TriageRowAction = postJson<TriageEditResult>("/v1/triage/keep");
export type ApproveTriage = (body: TriageApproveRequest) => Promise<TriageEditResult>;
export const postApproveTriage: ApproveTriage = postJson<TriageEditResult>("/v1/triage/approve");
export type SendBackTriage = (body: TriageSendBackRequest) => Promise<TriageEditResult>;
export const postSendBackTriage: SendBackTriage = postJson<TriageEditResult>("/v1/triage/send-back");
