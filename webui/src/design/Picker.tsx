import { Box, Input, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { Button } from "../ui/button/Button";
import { Sidebar } from "../ui/sidebar/Sidebar";
import { SidebarItem } from "../ui/sidebar/SidebarItem";
import { SidebarSection } from "../ui/sidebar/SidebarSection";
import { filterGroups, needsYouCount, rowHref, type PickerGroup, type PickerRow } from "./view";

const NOT_STARTED = "not-started";
// Sections that start collapsed, unless a filter matches in them or they hold the open Design.
const COLLAPSED = [NOT_STARTED, "recently-finished"];

function Marks({ section, row }: { section: string; row: PickerRow }) {
  if (section === "ready-to-queue") return <Mark color="tone.done.fg" label="Ready to queue" glyph="▶" />;
  if (section === "recently-finished") return <Mark color="tone.done.fg" label="Finished" glyph="✓" />;
  return (
    <Box display="flex" gap="1.5" alignItems="center">
      {row.chart && (
        <Text as="span" fontSize="xs" color="text.dim">
          Chart
        </Text>
      )}
      {row.failed && <Mark color="tone.deadlocked.fg" label="Needs attention" glyph="!" />}
      {row.question && <Mark color="tone.waiting.fg" label="Waiting for your answer" glyph="?" />}
      {row.live && <LiveMark />}
    </Box>
  );
}

const Mark = ({ color, label, glyph }: { color: string; label: string; glyph: string }) => (
  <Text as="span" role="img" aria-label={label} title={label} color={color} fontWeight="700" fontSize="sm">
    {glyph}
  </Text>
);

function LiveMark() {
  return (
    <Box as="span" role="img" aria-label="Live" title="Live session" position="relative" display="inline-flex" w="2.5" h="2.5">
      <Box position="absolute" inset="0" borderRadius="full" bg="tone.running.fg" opacity={0.5} animation="ping 1.6s infinite" />
      <Box position="relative" w="2.5" h="2.5" borderRadius="full" bg="tone.running.fg" />
    </Box>
  );
}

// Chakra's `lg` breakpoint; below it the picker is a dropdown. Without matchMedia (tests) it is wide.
const WIDE = "(min-width: 62em)";
export const useWide = () => {
  const mq = () => (typeof window.matchMedia === "function" ? window.matchMedia(WIDE) : undefined);
  const [wide, setWide] = useState(() => mq()?.matches ?? true);
  useEffect(() => {
    const m = mq();
    if (!m) return;
    const on = () => setWide(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return wide;
};

interface PickerProps {
  groups: PickerGroup[];
  projects: number;
  current?: string;
  onNew: () => void;
  // Called when a design is clicked; the mobile picker uses it to close itself.
  onSelect?: () => void;
}

export function Picker({ groups, projects, current, onNew, onSelect }: PickerProps) {
  const [query, setQuery] = useState("");
  const filtering = query.trim() !== "";
  return (
    <Sidebar
      title="Designs"
      action={
        <Button emphasis="ghost" onClick={onNew}>
          + New
        </Button>
      }
    >
      <Input
        type="search"
        aria-label="Filter Designs"
        placeholder="Filter"
        size="sm"
        mb="3"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {filterGroups(groups, query).map((g) => (
        // The key remounts the section so a filter match opens Not started and clearing it collapses it again.
        <SidebarSection
          key={`${g.key}-${filtering}`}
          title={g.title}
          count={g.rows.length}
          defaultCollapsed={COLLAPSED.includes(g.key) && !filtering && !g.rows.some((r) => r.row.address === current)}
        >
          {g.rows.map((r) => (
            <SidebarItem
              key={r.row.address}
              label={r.row.name}
              href={rowHref(g.key, r.row)}
              current={r.row.address === current}
              onClick={onSelect}
              marks={
                <>
                  {projects > 1 && (
                    <Text as="span" fontSize="xs" color="text.dim">
                      {r.row.project}
                    </Text>
                  )}
                  <Marks section={g.key} row={r} />
                </>
              }
            />
          ))}
        </SidebarSection>
      ))}
    </Sidebar>
  );
}

export function MobilePicker({ groups, projects, current, onNew }: PickerProps) {
  const [open, setOpen] = useState(false);
  const name = groups.flatMap((g) => g.rows).find((r) => r.row.address === current)?.row.name ?? "Designs";
  const more = needsYouCount(groups, current);
  return (
    <Box>
      <Box display="flex" alignItems="center" gap="3">
        <Button size="sm" emphasis="secondary" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {name} {open ? "▴" : "▾"}
        </Button>
        {more > 0 && (
          <Text as="span" fontSize="sm" color="tone.waiting.fg">
            ? {more} more
          </Text>
        )}
      </Box>
      {open && <Picker groups={groups} projects={projects} current={current} onNew={onNew} onSelect={() => setOpen(false)} />}
    </Box>
  );
}
