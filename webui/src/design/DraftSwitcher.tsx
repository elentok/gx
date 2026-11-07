import { Box, chakra, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import type { DesignDraftItem } from "../generated/server";
import { draftLabel, timeAgo } from "./drafts";

interface Props {
  /** The open draft's title. */
  label: string;
  /** The open draft's server id, once it has one. */
  currentId?: string;
  /** Fetches the drafts, most recent first; runs each time the menu opens. */
  load: () => Promise<DesignDraftItem[]>;
  onPick: (id: string) => void;
  onNew: () => void;
  /** Starts with the menu open; the gallery shows it that way. */
  defaultOpen?: boolean;
}

const rowStyle = {
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: "1px",
  w: "100%",
  px: "12px",
  py: "10px",
  minH: "52px",
  justifyContent: "center",
  borderBottomWidth: "1px",
  borderColor: "border",
  _last: { borderBottomWidth: "0" },
  textAlign: "left",
  cursor: "pointer",
  _hover: { bg: "surface" },
} as const;

/** The title row's draft menu: one row per draft, then New draft. */
export function DraftSwitcher({ label, currentId, load, onPick, onNew, defaultOpen = false }: Props) {
  const [open, setOpen] = useState(defaultOpen);
  const [items, setItems] = useState<DesignDraftItem[]>([]);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open]);

  // Fetches on every open, so the list is never older than the menu.
  useEffect(() => {
    if (open) load().then(setItems, () => {});
    // `load` is a fresh closure each render; only opening should refetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  const toggle = () => setOpen(!open);
  const choose = (act: () => void) => {
    setOpen(false);
    act();
  };

  return (
    <Box ref={root} minW="0">
      <chakra.button type="button" aria-haspopup="menu" aria-expanded={open} onClick={toggle} display="flex" alignItems="center" gap="1" minW="0" maxW="100%" fontWeight="600" cursor="pointer">
        <Text as="span" truncate>
          {label}
        </Text>
        <chakra.span aria-hidden color="text.dim" fontSize="xs">
          ▾
        </chakra.span>
      </chakra.button>
      {open && (
        <Box
          role="menu"
          aria-label="Drafts"
          position="absolute"
          top="calc(100% - 4px)"
          left="12px"
          right={{ base: "12px", md: "auto" }}
          w={{ md: "340px" }}
          maxH="320px"
          overflowY="auto"
          zIndex="10"
          bg="surface.raised"
          borderWidth="1px"
          borderColor="border"
          borderRadius="10px"
          boxShadow="0 10px 30px rgba(0,0,0,.5)"
        >
          {items.map((d) => {
            const here = d.id === currentId;
            return (
              <chakra.button key={d.id} type="button" role="menuitem" onClick={() => choose(() => (here ? undefined : onPick(d.id)))} {...rowStyle} bg={here ? "tone.running.bg" : undefined}>
                <Text fontWeight="600" fontSize="sm" truncate maxW="100%">
                  {draftLabel(d)}
                </Text>
                <Text fontSize="xs" color="text.dim">
                  {d.project} · {d.image_count > 0 && `${d.image_count} ${d.image_count === 1 ? "image" : "images"} · `}{here ? "editing now" : `edited ${timeAgo(d.edited_at)}`}
                </Text>
              </chakra.button>
            );
          })}
          <chakra.button type="button" role="menuitem" onClick={() => choose(onNew)} {...rowStyle} color="tone.running.fg" fontWeight="600" fontSize="sm">
            <span>
              <chakra.span aria-hidden>＋ </chakra.span>New draft
            </span>
          </chakra.button>
        </Box>
      )}
    </Box>
  );
}
