import { Box, Text, chakra } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import { postSaveTriageRow, type SaveTriageRow } from "../design/open";
import type { TriageRow, TriageView } from "../generated/server";
import { Button } from "../ui/button/Button";
import { GxMenu, type MenuEntry } from "../ui/menu/Menu";
import { showToast } from "../ui/toaster";
import { refusalText } from "../useWrite";
import { GROUPS, destinationChoices, takesEpic, type ReviewAction } from "./destinations";

// The wire's refusal for an out-of-date revision (server/wire ReasonStale); the generated types carry no constant for it.
const REASON_STALE = "stale";

interface Edit {
  action: string;
  destination?: string;
}

/** The row's person fields as the save replaces them wholesale: this edit over what the row already shows. */
function saveBody(project: string, revision: number, row: TriageRow, edit: Edit) {
  return {
    project,
    revision,
    address: row.address,
    action: edit.action === "keep" ? undefined : edit.action,
    destination: edit.destination,
    keep_for_later: edit.action === "keep" || undefined,
    answers: row.answers,
  };
}

/** Saves row edits one after another on the revision the last save returned. Shown edits stay until the view changes. */
function useRowEdits(project: string, view: TriageView, refetch: () => void, save: SaveTriageRow) {
  const [drafts, setDrafts] = useState<Record<string, Edit>>({});
  const revision = useRef(view.revision ?? 0);
  const queue = useRef<Promise<void>>(Promise.resolve());
  useEffect(() => {
    revision.current = view.revision ?? 0;
    setDrafts({});
  }, [view.revision]);

  const edit = (changes: [TriageRow, Edit][]) => {
    setDrafts((d) => ({ ...d, ...Object.fromEntries(changes.map(([r, e]) => [r.address, e])) }));
    queue.current = queue.current.then(async () => {
      for (const [row, e] of changes) {
        const res = await save(saveBody(project, revision.current, row, e));
        if (res.refused) {
          showToast(res.reason === REASON_STALE ? { type: "warning", title: "The plan changed. Review it again." } : { type: "error", title: refusalText(res) });
          setDrafts({});
          refetch();
          return;
        }
        if (res.revision !== undefined) revision.current = res.revision;
      }
      refetch();
    });
  };
  return { drafts, edit };
}

/** What a row becomes when its action changes: an epic destination survives a ready/map switch, any other starts empty. */
const switched = (row: TriageRow, action: string): Edit => ({ action, destination: takesEpic(row.action) && takesEpic(action) ? row.destination : undefined });

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

function noteOf(action: ReviewAction, rows: TriageRow[], readyEpic?: string): string {
  switch (action) {
    case "ready":
      return readyEpic ?? rows.find((r) => r.destination)?.destination ?? "";
    case "map": {
      const areas = [...new Set(rows.map((r) => r.destination).filter((d): d is string => !!d))];
      return areas.length ? `${plural(areas.length, "area", "areas")}: ${areas.join(", ")}` : "";
    }
    case "merge":
      return "duplicates of older follow-ups";
    case "close": {
      const parts = (["fixed", "obsolete"] as const).map((v) => [v, rows.filter((r) => r.verdict === v).length] as const).filter(([, n]) => n > 0);
      return parts.map(([v, n]) => `${v} ${n}`).join(", ");
    }
    case "keep":
      return "stay in follow-ups";
  }
}

function Dropdown({ label, items, name }: { label: string; items: MenuEntry[]; name: string }) {
  return (
    <GxMenu
      placement="bottom-start"
      items={items}
      trigger={
        <Button emphasis="secondary" size="sm" h="28px" px="2" fontSize="sm" aria-label={name} flexShrink={0}>
          <chakra.span maxW="110px" truncate>
            {label}
          </chakra.span>
          ▾
        </Button>
      }
    />
  );
}

function RowItem({ row, view, onEdit }: { row: TriageRow; view: TriageView; onEdit: (e: Edit) => void }) {
  const [naming, setNaming] = useState(false);
  const [typed, setTyped] = useState("");
  const action = GROUPS.find((g) => g.action === row.action);
  const choices = destinationChoices(row.action, row.address, view.destinations, view.rows);
  const current = row.destination && (choices.find((c) => c.value === row.destination)?.label ?? row.destination);
  const actionItems: MenuEntry[] = GROUPS.map((g) => ({
    value: g.action,
    label: g.short,
    current: g.action === row.action,
    onSelect: () => onEdit(switched(row, g.action)),
  }));
  const destItems: MenuEntry[] = choices.map((c) => ({
    value: c.value ?? "new",
    label: c.label,
    current: c.value !== undefined && c.value === row.destination,
    onSelect: () => (c.kind === "new" ? setNaming(true) : onEdit({ action: row.action ?? "", destination: c.value })),
  }));
  return (
    <Box
      display="flex"
      alignItems="center"
      gap="2"
      px="3"
      py="2"
      borderTopWidth="1px"
      borderColor="border"
      boxShadow={row.person_changed ? "inset 0 0 0 2px var(--chakra-colors-tone-running-fg)" : undefined}
      data-changed={row.person_changed || undefined}
    >
      <Text flex="1" minW="0" fontSize="sm" truncate title={row.title}>
        {row.title}
      </Text>
      <Button emphasis="secondary" size="sm" h="28px" px="2" fontSize="sm" flexShrink={0}>
        Evidence
      </Button>
      <Dropdown name={`Action for ${row.title}`} label={action?.short ?? "Action"} items={actionItems} />
      {destItems.length > 0 && !naming && <Dropdown name={`Destination for ${row.title}`} label={current || "Destination"} items={destItems} />}
      {naming && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            const name = typed.trim();
            if (!name) return;
            setNaming(false);
            onEdit({ action: row.action ?? "", destination: name });
          }}
        >
          <chakra.input aria-label="New map epic name" value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="New map epic name" borderWidth="1px" borderColor="border" borderRadius="md" px="2" h="32px" autoFocus />
        </form>
      )}
    </Box>
  );
}

function Group({
  action,
  name,
  rows,
  view,
  open,
  onToggle,
  onEdit,
}: {
  action: ReviewAction;
  name: string;
  rows: TriageRow[];
  view: TriageView;
  open: boolean;
  onToggle: () => void;
  onEdit: (changes: [TriageRow, Edit][]) => void;
}) {
  const setAll: MenuEntry[] = GROUPS.filter((g) => g.action !== action).map((g) => ({
    value: g.action,
    label: g.short,
    onSelect: () => onEdit(rows.map((r) => [r, switched(r, g.action)])),
  }));
  return (
    <Box data-part="action-group" display="flex" flexDirection="column" borderWidth="1px" borderColor="border" borderRadius="md" overflow="hidden">
      <Box display="flex" alignItems="center" gap="2" flexWrap="wrap" px="3" py="2">
        <chakra.button type="button" aria-expanded={open} onClick={onToggle} display="flex" alignItems="center" gap="2" flex="1" minW="0" cursor="pointer" textAlign="left">
          <span aria-hidden>{open ? "▾" : "▸"}</span>
          <Text fontWeight="700">{name}</Text>
          <Text px="2" borderRadius="full" bg="surface.raised" fontSize="sm" fontWeight="700">
            {rows.length}
          </Text>
          <Text color="text.muted" fontSize="sm" truncate>
            {noteOf(action, rows, view.ready_epic)}
          </Text>
        </chakra.button>
        {rows.length > 0 && <Dropdown name={`Set all in ${name}`} label="Set all" items={setAll} />}
      </Box>
      {open && rows.map((r) => <RowItem key={r.address} row={r} view={view} onEdit={(e) => onEdit([[r, e]])} />)}
    </Box>
  );
}

/** The plan in five collapsible action groups; each row edits its action and destination and saves with the record revision. */
export function ReviewView({ project, view, refetch, save = postSaveTriageRow }: { project: string; view: TriageView; refetch: () => void; save?: SaveTriageRow }) {
  const { drafts, edit } = useRowEdits(project, view, refetch, save);
  const [open, setOpen] = useState<Record<string, boolean>>({ ready: true });
  const rows = view.rows
    .filter((r) => !r.gone && r.action)
    .map((r) => {
      const d = drafts[r.address];
      return d ? { ...r, ...d, person_changed: true } : r;
    });
  const shown = { ...view, rows };
  return (
    <Box display="flex" flexDirection="column" gap="3">
      {GROUPS.map((g) => (
        <Group
          key={g.action}
          action={g.action}
          name={g.name}
          rows={rows.filter((r) => r.action === g.action)}
          view={shown}
          open={!!open[g.action]}
          onToggle={() => setOpen((o) => ({ ...o, [g.action]: !o[g.action] }))}
          onEdit={edit}
        />
      ))}
    </Box>
  );
}
