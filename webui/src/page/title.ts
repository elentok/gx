// Pages pass a `PageTitle` to the page frame, which writes `document.title`.
// `name` is the epic slug or ticket title; the list pages have none, and an empty one renders as none.
export type PageTitle = { kind: "Epics" | "Stats" | "Agents" | "Epic" | "Design" | "Ticket" | "Settings"; name?: string };

const SEPARATOR = " · ";

// `actionable` prefixes `(n) `, hidden at zero like the top bar badge.
export function formatTitle({ kind, name }: PageTitle, actionable = 0): string {
  const text = [name, kind, "gx"].filter(Boolean).join(SEPARATOR);
  return actionable > 0 ? `(${actionable}) ${text}` : text;
}
