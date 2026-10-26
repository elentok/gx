import {
  SectionFinished,
  SectionInProgress,
  SectionNeedsYou,
  SectionReadyToDesign,
  SectionReadyToQueue,
  type EpicListRow,
} from "../generated/server";
import type { Tone } from "../ui/tone";

export interface Section {
  key: string;
  title: string;
  tone: Tone;
  collapsedByDefault: boolean;
  rows: EpicListRow[];
}

const SECTIONS: Omit<Section, "rows">[] = [
  { key: SectionNeedsYou, title: "Needs you", tone: "waiting", collapsedByDefault: false },
  { key: SectionReadyToQueue, title: "Ready to queue", tone: "running", collapsedByDefault: false },
  { key: SectionInProgress, title: "In progress", tone: "running", collapsedByDefault: false },
  { key: SectionReadyToDesign, title: "Ready to design", tone: "muted", collapsedByDefault: true },
  { key: SectionFinished, title: "Finished", tone: "done", collapsedByDefault: true },
];

/**
 * Groups the cards by the section the server put them in, keeping the server's order. Without a
 * text filter, Finished shows only its first `finishedCap` cards; text searches all of them. A
 * project filter applies first, so the cap counts only that project's cards.
 */
export function sectionsOf(epics: EpicListRow[], query: string, finishedCap?: number, project?: string): Section[] {
  const q = query.trim().toLowerCase();
  return SECTIONS.map((s) => {
    let rows = epics.filter((e) => e.section === s.key && (project === undefined || e.project === project));
    if (q) rows = rows.filter((e) => e.name.toLowerCase().includes(q));
    else if (s.key === SectionFinished && finishedCap !== undefined) rows = rows.slice(0, finishedCap);
    return { ...s, rows };
  });
}
