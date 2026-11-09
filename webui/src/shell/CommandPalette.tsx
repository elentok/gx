import { Box } from "@chakra-ui/react";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { useNavigate } from "react-router";
import { toggleAfk } from "../afk";
import { useConnectionContext, useConnectionState } from "../connection";
import { AGENTS_HREF, EPICS_HREF, SETTINGS_HREF, STATS_HREF } from "../hrefs";
import { Menu, type ListSection } from "../nav/JumpMenu";
import { isMac } from "../platform";
import { useWide } from "../useWide";
import { setCommandOpen, useCommandOpen } from "./commandOpen";
import { openNewAgent, openNewDesign } from "./newItemOpen";
import { useShell } from "./ShellContext";
import { useDismissLayer } from "./useDismissLayer";

export interface Command {
  id: string;
  title: string;
  /** Extra words the filter matches besides the title. */
  words?: string;
  subtitle?: string;
  run: () => void;
}

/** The one command list, in display order. Insert a row at any position. */
export function useCommands(): Command[] {
  const navigate = useNavigate();
  const shell = useShell();
  const afk = useConnectionState(useConnectionContext()).snapshot?.afk ?? false;
  return useMemo(() => {
    const goto = (id: string, title: string, href: string): Command => ({
      id,
      title,
      run: () => {
        navigate(href.replace(/^#/, ""));
        shell?.closeSliding();
      },
    });
    return [
      { id: "new-design", title: "New design", run: () => openNewDesign() },
      { id: "new-chat", title: "New chat", run: () => openNewAgent() },
      goto("go-epics", "Go to Epics", EPICS_HREF),
      goto("go-agents", "Go to Agents", AGENTS_HREF),
      goto("go-stats", "Go to Stats", STATS_HREF),
      goto("go-settings", "Go to Settings", SETTINGS_HREF),
      { id: "toggle-afk", title: "Toggle AFK", subtitle: afk ? "AFK is on" : "AFK is off", run: () => toggleAfk(afk) },
    ];
  }, [navigate, shell, afk]);
}

/** Cmd+E on a Mac, Ctrl+E elsewhere, on every page, in text fields too. Same layout as the jump palette. */
export function CommandPalette() {
  const phone = !useWide();
  const open = useCommandOpen();
  const root = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setCommandOpen(false), []);
  const { isTop, hasLayer } = useDismissLayer({ open, close, root, phone });
  const commands = useCommands();

  useEffect(() => () => setCommandOpen(false), []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = isMac() ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
      if (mod && e.key.toLowerCase() === "e") {
        e.preventDefault();
        if (open ? isTop() : !hasLayer()) setCommandOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, isTop, hasLayer]);

  const sections = useMemo<ListSection[]>(
    () => [
      {
        key: "commands",
        title: "Commands",
        rows: commands.map((c) => ({
          key: c.id,
          title: c.title,
          subtitle: c.subtitle ?? "",
          href: "",
          searchText: [c.title, c.words].filter(Boolean).join(" "),
          onSelect: c.run,
        })),
      },
    ],
    [commands],
  );

  if (!open) return null;
  return (
    <Box ref={root} display="contents">
      <Menu sections={sections} label="Commands" placeholder="Type a command…" phone={phone} floating onClose={close} />
    </Box>
  );
}
