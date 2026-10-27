import type { EpicListRow, SessionInfo } from "../generated/server";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import type { EpicDetail } from "./detail";
import { pageRing } from "./ring";

export function EpicRing({
  detail,
  card,
  sessions,
  size,
}: {
  detail: EpicDetail;
  card?: EpicListRow;
  sessions: SessionInfo[];
  size: "md" | "sm" | "compact";
}) {
  const r = pageRing(detail, card, sessions);
  return <ProgressRing progress={r.progress} tone={r.tone} kind={r.kind} label={size === "compact" ? "" : r.label} size={size} live={r.live} />;
}
