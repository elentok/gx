import { CardKindOneOff, type EpicListRow } from "../generated/server";
import { epicHref, ticketHref } from "../hrefs";

/** The one link of the card; see "Card actions" in docs/specs/united-epics-page.md. */
export function titleHref(row: EpicListRow): string {
  if (row.card_kind === CardKindOneOff) return ticketHref(row.address);
  return epicHref(row.addresses?.[0] ?? row.address);
}
