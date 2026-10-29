import { useEffect } from "react";
import { useActionableCount } from "../agents/useActionableCount";

// The only place that writes `document.title`. Pages pass a `PageTitle` to the page frame, never call the hook.
export type PageTitle = { kind: "Epics" | "Stats" | "Agents" } | { kind: "Epic" | "Design" | "Ticket"; name: string };

const SEPARATOR = " · ";

// `actionable` prefixes `(n) `, hidden at zero like the top bar badge.
export function formatTitle(title: PageTitle, actionable = 0): string {
  const name = "name" in title ? title.name : "";
  const text = [name, title.kind, "gx"].filter((part) => part !== "").join(SEPARATOR);
  return actionable > 0 ? `(${actionable}) ${text}` : text;
}

// Writes on mount and whenever the string changes; the next page sets its own, so nothing is restored.
export function usePageTitle(title: PageTitle) {
  const text = formatTitle(title, useActionableCount());
  useEffect(() => {
    document.title = text;
  }, [text]);
}
