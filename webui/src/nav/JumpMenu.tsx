import { Box, chakra, Heading, Input, Text } from "@chakra-ui/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext } from "../connection";
import { CardActionAnswer, CardKindDesign,SectionNeedsYou, SectionReadyToDesign, SectionFinished, type EpicListRow } from "../generated/server";
import { designHref } from "../hrefs";
import { useWide } from "../design/useWide";
import { titleHref } from "../epics/cardLinks";
import { isLive, RING_KIND, ringProgress, ringTone } from "../epics/ring";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { sectionsOf, type Section } from "../epics/view";
import { Button } from "../ui/button/Button";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";

/**
 * A Design opens on its Design page, a card whose Answer link the server set
 * opens there, any other card where its title points.
 */
export const jumpHref = (row: EpicListRow): string =>
  row.card_kind === CardKindDesign
    ? designHref(row.address)
    : (row.actions?.find((a) => a.kind === CardActionAnswer)?.link ?? titleHref(row));

const isCurrent = (row: EpicListRow, current: string) => row.address === current || (row.addresses?.includes(current) ?? false);

/** Ready to design and Finished show only while the filter has text. */
function visibleSections(epics: EpicListRow[], query: string): Section[] {
  const searching = query.trim() !== "";
  return sectionsOf(epics, query).filter(
    (s) => s.rows.length > 0 && (searching || (s.key !== SectionReadyToDesign && s.key !== SectionFinished)),
  );
}

export function JumpRow({
  row,
  current = false,
  active = false,
  onPick,
  id,
}: {
  row: EpicListRow;
  current?: boolean;
  /** The keyboard cursor is on this row. */
  active?: boolean;
  onPick?: () => void;
  id?: string;
}) {
  const kind = RING_KIND[row.card_kind ?? "implementation"];
  const needsYou = row.section === SectionNeedsYou;
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
      <ProgressRing progress={ringProgress(row)} tone={ringTone(row)} kind={kind} size="compact" live={isLive(row)} />
      <Box minW="0" flex="1">
        <Text fontFamily="condensed" fontSize="16px" fontWeight="600" lineClamp="1">
          {row.name}
          {current && (
            <Text as="span" ml="2" fontSize="13px" color="text.dim" fontWeight="400">
              current
            </Text>
          )}
        </Text>
        <Text fontSize="13px" color="text.dim" lineClamp="1">
          {row.project} · {needsYou ? row.state_pill : row.status_words}
        </Text>
      </Box>
    </Box>
  );
}

function Menu({
  epics,
  current,
  phone,
  onClose,
}: {
  epics: EpicListRow[];
  current: string;
  phone: boolean;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const sections = useMemo(() => visibleSections(epics, query), [epics, query]);
  const flat = sections.flatMap((s) => s.rows);
  const pick = (row: EpicListRow) => {
    onClose();
    navigate(jumpHref(row).slice(1));
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
      aria-label="Jump to epic"
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
          aria-label="Jump to epic"
          placeholder="Filter epics"
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
              return (
                <JumpRow
                  key={row.address}
                  id={`jump-row-${i}`}
                  row={row}
                  current={isCurrent(row, current)}
                  active={i === cursor}
                  onPick={() => pick(row)}
                />
              );
            })}
          </Box>
        ))}
      </Box>
    </Box>
  );
}

/**
 * The page title as a button that opens a list of epics to switch to. `current` is the address
 * of the epic on screen; the count on the title is the other Needs-you cards.
 */
export function JumpMenu({ name, current, fetchList }: { name: string; current: string; fetchList?: FetchEpicList }) {
  const conn = useConnectionContext();
  const { list } = useEpicList(fetchList, conn);
  const phone = !useWide();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const epics = list?.epics ?? [];
  const waiting = epics.filter((e) => e.section === SectionNeedsYou && !isCurrent(e, current)).length;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
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
  }, [phone]);

  return (
    <Box ref={root} position="relative" minW="0">
      <Heading as="h1" fontFamily="condensed" fontSize="2xl">
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
          {waiting > 0 && (
            <Text
              as="span"
              data-testid="waiting-count"
              aria-label={`${waiting} more need you`}
              fontSize="14px"
              px="2"
              borderRadius="full"
              color="tone.waiting.fg"
              bg="tone.waiting.bg"
            >
              {waiting}
            </Text>
          )}
          {!phone && (
            <Text as="span" aria-hidden fontSize="13px" color="text.dim" fontFamily="text" fontWeight="400">
              ⌘K
            </Text>
          )}
        </chakra.button>
      </Heading>
      {open && <Menu epics={epics} current={current} phone={phone} onClose={() => setOpen(false)} />}
    </Box>
  );
}
