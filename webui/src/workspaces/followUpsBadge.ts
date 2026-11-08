import { FollowUpTriageRunning, FollowUpTriageWaiting, type FollowUpCount } from "../generated/server";
import { triageHref } from "../hrefs";

/** The Epics page card a line focuses, as route state `focus`; the card carries the same value as data-focus. */
export const followUpsFocus = (project: string) => `follow-ups:${project}`;

/** The badge: open follow-ups summed over the projects. */
export const badgeTotal = (counts: FollowUpCount[]) => counts.reduce((sum, c) => sum + c.open, 0);

/** Amber while any Triage waits for Approve. */
export const badgeWaiting = (counts: FollowUpCount[]) => counts.some((c) => c.triage === FollowUpTriageWaiting);

/** A Triage that is running or waiting for Approve; a stopped one is not live. */
export const triageLive = (c: FollowUpCount) => c.triage === FollowUpTriageRunning || c.triage === FollowUpTriageWaiting;

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

export interface BadgeLine {
  project: string;
  /** "8 bugs · 4 ideas", or "Triage · 12 left" while a Triage is live. */
  text: string;
  /** The count pill; absent on a live Triage line, which shows an Open button instead. */
  count?: number;
  live: boolean;
  /** A router path for navigate(). */
  path: string;
  /** Route state for navigate(). */
  state?: { focus: string };
}

export function badgeLine(c: FollowUpCount): BadgeLine {
  if (triageLive(c)) return { project: c.project, text: `Triage · ${c.left} left`, live: true, path: triageHref(c.project).slice(1) };
  const parts = [plural(c.bugs, "bug", "bugs"), plural(c.ideas, "idea", "ideas")];
  if (c.other > 0) parts.push(`${c.other} other`);
  return {
    project: c.project,
    text: parts.join(" · "),
    count: c.open,
    live: false,
    path: "/epics",
    state: { focus: followUpsFocus(c.project) },
  };
}
