import { liveSessions } from "../design/view";
import {
  EpicStateFinished,
  EpicStateMerging,
  EpicStateRunning,
  MapStepCurrent,
  MapStepDone,
  ProgressCreateTickets,
  ProgressDone,
  SectionNeedsYou,
  type EpicDetail,
  type EpicListRow,
  type SessionInfo,
} from "../generated/server";
import { RING_KIND, isLive, ringTone } from "../epics/ring";
import type { Kind, Tone } from "../ui/tone";

export interface PageRing {
  kind: Kind;
  tone: Tone;
  live: boolean;
  progress: number;
  label: string;
}

const stepState = (d: EpicDetail, step: string) => d.steps?.find((s) => s.step === step)?.state;

// A Design page's own tone, first match wins. It cannot lean on the card, which shows the -impl epic once one exists.
function designTone(d: EpicDetail, sessions: SessionInfo[]): Pick<PageRing, "tone" | "live"> {
  if (stepState(d, ProgressDone) === MapStepDone) return { tone: "done", live: false };
  const live = liveSessions(d.address, sessions);
  const actionable = live.some((s) => !!s.actionable?.reason);
  if (d.design_section === SectionNeedsYou || actionable) return { tone: "waiting", live: false };
  const creating = stepState(d, ProgressCreateTickets) === MapStepCurrent && !!d.impl_address;
  if (creating || live.some((s) => s.status.state === "starting" || s.status.state === "working")) {
    return { tone: "running", live: true };
  }
  return live.length > 0 ? { tone: "running", live: false } : { tone: "muted", live: false };
}

function loopTone(d: EpicDetail): Pick<PageRing, "tone" | "live"> {
  if (d.state === EpicStateRunning || d.state === EpicStateMerging) return { tone: "running", live: true };
  return { tone: d.state === EpicStateFinished ? "done" : "muted", live: false };
}

/** The ring of an epic page. `card` is the joined card whose active address is this page, if any. */
export function pageRing(d: EpicDetail, card: EpicListRow | undefined, sessions: SessionInfo[]): PageRing {
  const isMap = d.kind === "map";
  const kind = card?.card_kind ? RING_KIND[card.card_kind] : isMap ? "design" : "implementation";
  if (isMap) {
    const pct = d.ring_progress ?? 0;
    return { kind, ...designTone(d, sessions), progress: pct / 100, label: `${pct}%` };
  }
  const tone = card && d.kind === "loop" ? { tone: ringTone(card), live: isLive(card) } : loopTone(d);
  return { kind, ...tone, progress: d.total ? d.done / d.total : 0, label: `${d.done}/${d.total}` };
}
