import { Box, chakra, Heading, Input, Text } from "@chakra-ui/react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useNavigate } from "react-router";
import { CardActionAnswer, CardKindDesign,SectionNeedsYou, SectionReadyToDesign, SectionFinished, type EpicListRow } from "../generated/server";
import { designHref } from "../hrefs";
import { useWide } from "../useWide";
import { titleHref } from "../epics/cardLinks";
import { isCurrent } from "../epics/list";
import { isLive, RING_KIND, ringProgress, ringTone } from "../epics/ring";
import { sectionsOf } from "../epics/view";
import { Button } from "../ui/button/Button";
import { titleSize } from "./titleSize";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";

/**
 * A Design opens on its Design page, a card whose Answer link the server set
 * opens there, any other card where its title points.
 */
export const jumpHref = (row: EpicListRow): string =>
  row.card_kind === CardKindDesign
    ? designHref(row.address)
    : (row.actions?.find((a) => a.kind === CardActionAnswer)?.link ?? titleHref(row));

/** One row of a row list. `href` is a hash link, such as `#/e/demo:blog`. */
export type ListRowData = {
  key: string;
  title: string;
  subtitle: string;
  href: string;
  /** The filter matches this text, case-insensitively. */
  searchText: string;
  lead?: ReactNode;
  trailing?: ReactNode;
  current?: boolean;
};

export type ListSection = {
  key: string;
  title: string;
  rows: ListRowData[];
  /** Hidden while the filter is empty. */
  hiddenUntilSearch?: boolean;
};

function visibleSections(sections: ListSection[], query: string): ListSection[] {
  const q = query.trim().toLowerCase();
  return sections
    .map((s) => (q ? { ...s, rows: s.rows.filter((r) => r.searchText.toLowerCase().includes(q)) } : s))
    .filter((s) => s.rows.length > 0 && (q !== "" || !s.hiddenUntilSearch));
}

export function ListRow({
  row,
  active = false,
  onPick,
  id,
}: {
  row: ListRowData;
  /** The keyboard cursor is on this row. */
  active?: boolean;
  onPick?: () => void;
  id?: string;
}) {
  const current = row.current ?? false;
  return (
    <Box
      id={id}
      role="option"
      aria-selected={active}
      aria-current={current ? "true" : undefined}
      data-current={current || undefined}
      display="flex"
      alignItems="center"
      gap="3"
      px="3"
      py="2"
      cursor="pointer"
      bg={active ? "surface" : undefined}
      _hover={{ bg: "surface" }}
      onClick={onPick}
    >
      {row.lead}
      <Box minW="0" flex="1">
        <Text fontFamily="condensed" fontSize="16px" fontWeight="600" lineClamp="1">
          {row.title}
          {current && (
            <Text as="span" ml="2" fontSize="13px" color="text.dim" fontWeight="400">
              current
            </Text>
          )}
        </Text>
        <Text fontSize="13px" color="text.dim" lineClamp="1">
          {row.subtitle}
        </Text>
      </Box>
      {row.trailing}
    </Box>
  );
}

function epicRow(row: EpicListRow, current: boolean): ListRowData {
  const kind = RING_KIND[row.card_kind ?? "implementation"];
  const needsYou = row.section === SectionNeedsYou;
  return {
    key: row.address,
    title: row.name,
    subtitle: `${row.project} · ${needsYou ? row.state_pill : row.status_words}`,
    href: jumpHref(row),
    searchText: row.name,
    current,
    lead: <ProgressRing progress={ringProgress(row)} tone={ringTone(row)} kind={kind} size="compact" live={isLive(row)} />,
  };
}

/** An epic as a row; gallery cases render it on its own. */
export function JumpRow({
  row,
  current = false,
  active = false,
  onPick,
  id,
}: {
  row: EpicListRow;
  current?: boolean;
  active?: boolean;
  onPick?: () => void;
  id?: string;
}) {
  return <ListRow row={epicRow(row, current)} active={active} onPick={onPick} id={id} />;
}

function Menu({
  sections: all,
  label,
  placeholder,
  phone,
  onClose,
}: {
  sections: ListSection[];
  label: string;
  placeholder: string;
  phone: boolean;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const sections = useMemo(() => visibleSections(all, query), [all, query]);
  const flat = sections.flatMap((s) => s.rows);
  const pick = (row: ListRowData) => {
    onClose();
    navigate(row.href.slice(1));
  };
  const activeId = flat[cursor] ? `jump-row-${cursor}` : undefined;

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (flat.length === 0) return;
      setCursor((c) => (c + (e.key === "ArrowDown" ? 1 : flat.length - 1)) % flat.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (flat[cursor]) pick(flat[cursor]);
    }
  }

  useEffect(() => {
    document.getElementById(activeId ?? "")?.scrollIntoView?.({ block: "nearest" });
  }, [activeId]);

  let index = 0;
  return (
    <Box
      role="dialog"
      aria-label={label}
      bg="surface.raised"
      borderWidth="1px"
      borderColor="border"
      borderRadius={phone ? undefined : "16px"}
      display="flex"
      flexDirection="column"
      zIndex="dropdown"
      {...(phone
        ? { position: "fixed", inset: "0", p: "4", gap: "3" }
        : { position: "absolute", top: "100%", left: "0", mt: "2", w: "440px", maxW: "calc(100vw - 32px)", p: "3", gap: "2" })}
    >
      <Box display="flex" alignItems="center" gap="2">
        <Input
          autoFocus
          type="search"
          aria-label={label}
          placeholder={placeholder}
          h="40px"
          fontSize="16px"
          value={query}
          role="combobox"
          aria-expanded
          aria-controls="jump-list"
          aria-activedescendant={activeId}
          onChange={(e) => {
            setQuery(e.target.value);
            setCursor(0);
          }}
          onKeyDown={onKeyDown}
        />
        {phone && (
          <Button emphasis="ghost" onClick={onClose}>
            Close
          </Button>
        )}
      </Box>
      <Box id="jump-list" role="listbox" overflowY="auto" maxH={phone ? undefined : "60vh"} flex={phone ? "1" : undefined}>
        {sections.length === 0 && (
          <Text px="3" py="2" color="text.muted">
            No match
          </Text>
        )}
        {sections.map((s) => (
          <Box key={s.key} role="group" aria-label={s.title}>
            <Text px="3" pt="2" fontSize="13px" color="text.dim" textTransform="uppercase">
              {s.title}
            </Text>
            {s.rows.map((row) => {
              const i = index++;
              return <ListRow key={row.key} id={`jump-row-${i}`} row={row} active={i === cursor} onPick={() => pick(row)} />;
            })}
          </Box>
        ))}
      </Box>
    </Box>
  );
}

/**
 * The page title as a button that opens a filterable list of rows. The caller gives the sections
 * and their order. `hotkey` turns the built-in Cmd/Ctrl+K toggle off when false.
 */
export function RowList({
  name,
  label,
  placeholder,
  sections,
  hotkey = true,
}: {
  name: string;
  label: string;
  placeholder: string;
  sections: ListSection[];
  hotkey?: boolean;
}) {
  const phone = !useWide();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (hotkey && (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((o) => !o);
      } else if (e.key === "Escape") setOpen(false);
    };
    const onDown = (e: MouseEvent) => {
      if (!phone && root.current && !root.current.contains(e.target as Node)) setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onDown);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onDown);
    };
  }, [phone, hotkey]);

  return (
    <Box ref={root} position="relative" minW="0">
      <Heading as="h1" fontFamily="condensed" {...titleSize(!phone)}>
        <chakra.button
          type="button"
          aria-haspopup="dialog"
          aria-expanded={open}
          display="inline-flex"
          alignItems="center"
          gap="2"
          maxW="100%"
          cursor="pointer"
          onClick={() => setOpen((o) => !o)}
        >
          <Text as="span" lineClamp="1">
            {name}
          </Text>
          <Text as="span" aria-hidden fontSize="16px" color="text.dim">
            ▾
          </Text>
          {!phone && hotkey && (
            <Text as="span" aria-hidden fontSize="13px" color="text.dim" fontFamily="text" fontWeight="400">
              ⌘K
            </Text>
          )}
        </chakra.button>
      </Heading>
      {open && <Menu sections={sections} label={label} placeholder={placeholder} phone={phone} onClose={() => setOpen(false)} />}
    </Box>
  );
}

/**
 * The epic jump menu: the page title opens a list of epics to switch to. `current` is the address
 * of the epic on screen; `epics` is the list the page already loaded. Ready to design and Finished
 * show only while the filter has text.
 */
export function JumpMenu({ name, current, epics }: { name: string; current: string; epics: EpicListRow[] }) {
  const sections = useMemo(
    () =>
      sectionsOf(epics, "").map((s) => ({
        key: s.key,
        title: s.title,
        hiddenUntilSearch: s.key === SectionReadyToDesign || s.key === SectionFinished,
        rows: s.rows.map((r) => epicRow(r, isCurrent(r, current))),
      })),
    [epics, current],
  );
  return <RowList name={name} label="Jump to epic" placeholder="Filter epics" sections={sections} />;
}
