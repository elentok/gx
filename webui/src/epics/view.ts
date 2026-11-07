import {
  SectionFinished,
  SectionDesigning,
  SectionImplementing,
  SectionNeedsYou,
  SectionOnHold,
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
  { key: SectionImplementing, title: "Implementing", tone: "running", collapsedByDefault: false },
  { key: SectionReadyToQueue, title: "Ready to queue", tone: "running", collapsedByDefault: false },
  { key: SectionDesigning, title: "Designing", tone: "running", collapsedByDefault: false },
  { key: SectionReadyToDesign, title: "Ready to design", tone: "muted", collapsedByDefault: true },
  { key: SectionFinished, title: "Finished", tone: "done", collapsedByDefault: true },
  { key: SectionOnHold, title: "On hold", tone: "muted", collapsedByDefault: true },
];

/**
 * Groups the cards by the section the server put them in, keeping the server's order. Without a
 * text filter, Finished shows only its first `finishedCap` cards; text searches all of them. A
 * project and workspace filters apply first, so the cap counts only their cards. A named
 * workspace hides the cards that have none (One-off, Follow-ups).
 */
export function sectionsOf(
  epics: EpicListRow[],
  query: string,
  finishedCap?: number,
  projects?: string[],
  workspace?: string,
): Section[] {
  const q = query.trim().toLowerCase();
  return SECTIONS.map((s) => {
    let rows = epics.filter(
      (e) =>
        e.section === s.key &&
        (projects === undefined || projects.includes(e.project)) &&
        (workspace === undefined || e.workspace === workspace),
    );
    if (q) rows = rows.filter((e) => e.name.toLowerCase().includes(q));
    else if (s.key === SectionFinished && finishedCap !== undefined) rows = rows.slice(0, finishedCap);
    return { ...s, rows };
  });
}
