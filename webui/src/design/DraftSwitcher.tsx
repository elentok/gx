import { Box, chakra, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import type { DesignDraftItem } from "../generated/server";
import { Button } from "../ui/button/Button";
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
  onRename: () => void;
  /** Deletes a draft; the menu then drops its row. */
  onDelete: (id: string) => Promise<void>;
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
export function DraftSwitcher({ label, currentId, load, onPick, onNew, onRename, onDelete, defaultOpen = false }: Props) {
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
  const [asked, setAsked] = useState<string>();
  const remove = async (id: string) => {
    setAsked(undefined);
    await onDelete(id);
    setItems((all) => all.filter((d) => d.id !== id));
  };
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
            const asking = asked === d.id;
            return (
              <Box key={d.id} display="flex" alignItems="center" borderBottomWidth="1px" borderColor="border" bg={here ? "tone.running.bg" : undefined}>
                {asking ? (
                  <Box display="flex" alignItems="center" gap="2" w="100%" px="12px" minH="52px">
                    <Text flex="1" fontSize="sm" color="tone.paused.fg" fontWeight="600">
                      Discard this draft and its images?
                    </Text>
                    <Button type="button" emphasis="ghost" tone="muted" onClick={() => setAsked(undefined)}>
                      Keep
                    </Button>
                    <Button type="button" tone="paused" onClick={() => remove(d.id)}>
                      Discard
                    </Button>
                  </Box>
                ) : (
                  <>
                    <chakra.button type="button" role="menuitem" onClick={() => choose(() => (here ? undefined : onPick(d.id)))} {...rowStyle} borderBottomWidth="0" flex="1" minW="0">
                      <Text fontWeight="600" fontSize="sm" truncate maxW="100%">
                        {draftLabel(d)}
                      </Text>
                      <Text fontSize="xs" color="text.dim">
                        {d.project} · {d.image_count > 0 && `${d.image_count} ${d.image_count === 1 ? "image" : "images"} · `}{here ? "editing now" : `edited ${timeAgo(d.edited_at)}`}
                      </Text>
                    </chakra.button>
                    <chakra.button type="button" aria-label="Delete draft" title="Delete draft" onClick={() => (d.slug.trim() || d.idea.trim() || d.name || d.image_count > 0 ? setAsked(d.id) : remove(d.id))} w="36px" h="36px" mr="6px" flexShrink="0" borderRadius="8px" color="text.dim" _hover={{ bg: "surface", color: "text" }}>
                      🗑
                    </chakra.button>
                  </>
                )}
              </Box>
            );
          })}
          <chakra.button type="button" role="menuitem" onClick={() => choose(onNew)} {...rowStyle} color="tone.running.fg" fontWeight="600" fontSize="sm">
            <span>
              <chakra.span aria-hidden>＋ </chakra.span>New draft
            </span>
          </chakra.button>
          <chakra.button type="button" role="menuitem" onClick={() => choose(onRename)} {...rowStyle} fontSize="sm">
            <span>
              <chakra.span aria-hidden>✎ </chakra.span>Name this draft
            </span>
          </chakra.button>
        </Box>
      )}
    </Box>
  );
}
