import { CardKindDesign, CardKindFollowUps, CardKindOneOff, SectionNeedsYou, type EpicList, type EpicListRow } from "../generated/server";
import { designHref } from "../hrefs";
import { titleHref } from "../epics/cardLinks";
import { jumpHref } from "../nav/JumpMenu";
import { inWorkspace } from "../workspaces/followUpsBadge";

export interface NextQuestion {
  /** The card to open; undefined when nothing waits. */
  next?: EpicListRow;
  /** The waiting cards, newest first. */
  cards: EpicListRow[];
  count: number;
}

const waitingSince = (r: EpicListRow) => (r.card_kind === CardKindFollowUps ? undefined : r.waiting_since);

/** A router path: the hash link without its `#`. */
const pathOf = (href: string) => href.slice(1);

/** The card's own page, which a person on it counts as "on the card" even when the Answer link is elsewhere. */
const ownPath = (r: EpicListRow) => pathOf(r.card_kind === CardKindDesign ? designHref(r.address) : titleHref(r));

/**
 * The Needs you cards of the workspace, newest first, and the one after the
 * card whose page `pathname` is (wrapping). From any other page it is the
 * newest card. No workspace means every workspace.
 */
export function nextQuestion(list: Pick<EpicList, "epics" | "follow_ups">, workspace: string | undefined, pathname: string): NextQuestion {
  const projects = new Set(inWorkspace(list.follow_ups, workspace).map((c) => c.project));
  const inScope = (r: EpicListRow) => {
    if (r.card_kind === CardKindOneOff) return true;
    if (workspace === undefined) return true;
    return r.card_kind === CardKindFollowUps ? projects.has(r.project) : r.workspace === workspace;
  };
  // Reverse first so ties, and the Follow-ups cards, keep the server's order reversed; the sort is stable.
  const cards = list.epics
    .filter((r) => r.section === SectionNeedsYou && inScope(r))
    .reverse()
    .sort((a, b) => {
      const ta = waitingSince(a);
      const tb = waitingSince(b);
      if (ta === undefined || tb === undefined) return Number(ta === undefined) - Number(tb === undefined);
      return Date.parse(tb) - Date.parse(ta);
    });
  const at = cards.findIndex((r) => pathOf(jumpHref(r)) === pathname || ownPath(r) === pathname);
  return { next: cards[(at + 1) % cards.length], cards, count: cards.length };
}
