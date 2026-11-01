import { useState, type ReactNode } from "react";
import { CommandMenu, type CommandMenuRow, type CommandMenuStatus } from "../ui/command-menu/CommandMenu";
import { useWide } from "../useWide";
import { fillCommand, matchCommands, openToken } from "./match";
import { useCommandList, type CommandTarget, type FetchCommands } from "./useCommandList";

/** What a field needs to offer completion: whose commands, and for which harness. */
export interface CompletionConfig {
  target: CommandTarget;
  harness: string | undefined;
  fetcher?: FetchCommands;
  /** No menu while the field cannot send. Kept a flag, not a missing config, so the textarea is not remounted. */
  off?: boolean;
}

/** The part of a keydown the menu reads: React's and the DOM's both fit. */
export type MenuKey = Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey" | "preventDefault">;

export interface Completion {
  menu: ReactNode;
  /** True when the key was used by the menu; the caller then skips its own handling. */
  onKeyDown: (e: MenuKey) => boolean;
}

/** The completion menu for a text field: open state, selection and keys. The field passes its text and cursor and applies `fill`. */
export function useCompletion(
  config: CompletionConfig,
  value: string,
  cursor: number,
  disabled: boolean | undefined,
  fill: (text: string, cursor: number) => void,
): Completion {
  const wide = useWide();
  const token = openToken(value, cursor, { harness: config.harness, disabled: disabled || config.off });
  const list = useCommandList(config.target, token?.kind ?? "/", config.fetcher);
  // Escape hides the menu until the text changes; the selection restarts when the query does.
  const [dismissed, setDismissed] = useState<string | null>(null);
  const [pick, setPick] = useState({ query: "", index: 0 });

  const open = token !== null && dismissed !== value;
  const matches = token ? matchCommands(list.entries, token.query) : [];
  // Rows show the sigil the fill will write: the menu takes names and segments as displayed.
  const sigil = token?.kind ?? "/";
  const rows: CommandMenuRow[] = matches.map((m) => ({
    name: `${sigil}${m.entry.name}`,
    hint: m.entry.arg_hint || undefined,
    description: m.entry.description,
    source: m.entry.source,
    segments: m.segments.map((s, i) => (i === 0 ? { ...s, text: sigil + s.text } : s)),
  }));
  const selected = token && pick.query === token.query ? Math.min(pick.index, Math.max(rows.length - 1, 0)) : 0;
  // A degraded list (built-ins only) warns like a failed one, and still lists its rows.
  const status: CommandMenuStatus =
    list.status === "loading" ? "loading" : list.degraded ? "failed" : rows.length === 0 && list.status === "ready" ? "no-match" : list.status;

  const choose = (i: number) => {
    if (!token || !matches[i]) return;
    const out = fillCommand(value, token, matches[i].entry.name);
    fill(out.text, out.cursor);
  };

  const onKeyDown = (e: MenuKey) => {
    if (!open || !token) return false;
    if (e.key === "Escape") {
      e.preventDefault();
      setDismissed(value);
      return true;
    }
    if (rows.length === 0 || e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return false;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const step = e.key === "ArrowDown" ? 1 : rows.length - 1;
      setPick({ query: token.query, index: (selected + step) % rows.length });
      return true;
    }
    if (e.key === "Enter" || e.key === "Tab") {
      e.preventDefault();
      choose(selected);
      return true;
    }
    return false;
  };

  const menu = open && token ? <CommandMenu rows={rows} selected={selected} status={status} typed={value.slice(token.start, token.end)} compact={!wide} onPick={choose} /> : null;
  return { menu, onKeyDown };
}
