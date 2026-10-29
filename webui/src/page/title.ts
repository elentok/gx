import { useEffect } from "react";

// The only place that writes `document.title`. Pages pass a `PageTitle` to the page frame, never call the hook.
export type PageTitle = { kind: "Epics" | "Stats" } | { kind: "Epic" | "Design" | "Ticket"; name: string };

const SEPARATOR = " · ";

export function formatTitle(title: PageTitle): string {
  const name = "name" in title ? title.name : "";
  return [name, title.kind, "gx"].filter((part) => part !== "").join(SEPARATOR);
}

// Writes on mount and whenever the string changes; the next page sets its own, so nothing is restored.
export function usePageTitle(title: PageTitle) {
  const text = formatTitle(title);
  useEffect(() => {
    document.title = text;
  }, [text]);
}
