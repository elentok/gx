import { Box, Text, chakra } from "@chakra-ui/react"
import { useEffect, useRef, useState, type ReactNode } from "react"
import { postSaveTriageRow, type SaveTriageRow } from "./api"
import type { TriageRow, TriageRowRequest, TriageView } from "../generated/server"
import { Button } from "../ui/button/Button"
import { GxMenu, type MenuEntry } from "../ui/menu/Menu"
import { Modal } from "../ui/modal/Modal"
import { showToast } from "../ui/toaster"
import { refusalText } from "../useWrite"
import { GROUPS, destinationChoices, takesEpic, type ReviewAction } from "./destinations"

// The wire's refusal for an out-of-date revision (server/wire ReasonStale); the generated types carry no constant for it.
const REASON_STALE = "stale"

interface Edit {
  action: string
  destination?: string
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
  }
}

/** Saves row edits one after another on the revision the last save returned. Shown edits stay until the view changes. */
function useRowEdits(project: string, view: TriageView, refetch: () => void, save: SaveTriageRow) {
  const [drafts, setDrafts] = useState<Record<string, Edit>>({})
  const revision = useRef(view.revision ?? 0)
  const queue = useRef<Promise<void>>(Promise.resolve())
  useEffect(() => {
    revision.current = view.revision ?? 0
    setDrafts({})
  }, [view.revision])

  const enqueue = (bodies: ((revision: number) => TriageRowRequest)[]) => {
    queue.current = queue.current.then(async () => {
      for (const body of bodies) {
        const res = await save(body(revision.current))
        if (res.refused) {
          showToast(
            res.reason === REASON_STALE
              ? { type: "warning", title: "The plan changed. Review it again." }
              : { type: "error", title: refusalText(res) },
          )
          setDrafts({})
          refetch()
          return
        }
        if (res.revision !== undefined) revision.current = res.revision
      }
      refetch()
    })
  }

  const edit = (changes: [TriageRow, Edit][]) => {
    setDrafts((d) => ({ ...d, ...Object.fromEntries(changes.map(([r, e]) => [r.address, e])) }))
    enqueue(
      changes.map(
        ([row, e]) =>
          (rev) =>
            saveBody(project, rev, row, e),
      ),
    )
  }
  /** Undoes the group the row is in: the same record operation as `triage row --ungroup`. */
  const ungroup = (row: TriageRow) =>
    enqueue([(rev) => ({ project, revision: rev, address: row.address, ungroup: true })])
  return { drafts, edit, ungroup }
}

/** What a row becomes when its action changes: an epic destination survives a ready/map switch, any other starts empty. */
const switched = (epicActions: string[], row: TriageRow, action: string): Edit => ({
  action,
  destination:
    takesEpic(epicActions, row.action) && takesEpic(epicActions, action)
      ? row.destination
      : undefined,
})

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

function noteOf(action: ReviewAction, rows: TriageRow[], readyEpic?: string): string {
  switch (action) {
    case "ready":
      return readyEpic ?? rows.find((r) => r.destination)?.destination ?? ""
    case "map": {
      const areas = [...new Set(rows.map((r) => r.destination).filter((d): d is string => !!d))]
      return areas.length ? `${plural(areas.length, "area", "areas")}: ${areas.join(", ")}` : ""
    }
    case "merge":
      return "duplicates of older follow-ups"
    case "close": {
      const parts = (["fixed", "obsolete"] as const)
        .map((v) => [v, rows.filter((r) => r.verdict === v).length] as const)
        .filter(([, n]) => n > 0)
      return parts.map(([v, n]) => `${v} ${n}`).join(", ")
    }
    case "keep":
      return "stay in follow-ups"
  }
}

function Dropdown({ label, items, name }: { label: string; items: MenuEntry[]; name: string }) {
  return (
    <GxMenu
      placement="bottom-start"
      items={items}
      trigger={
        <Button
          emphasis="secondary"
          size="sm"
          h="28px"
          px="2"
          fontSize="sm"
          aria-label={name}
          flexShrink={0}
        >
          <chakra.span maxW="110px" truncate>
            {label}
          </chakra.span>
          ▾
        </Button>
      }
    />
  )
}

/** The verdict's details, read-only. A full-width sheet on a phone, a centred panel from the md breakpoint up. */
function EvidenceSheet({ row, onClose }: { row: TriageRow; onClose: () => void }) {
  const section = (title: string, body: ReactNode) => (
    <Box>
      <Text fontSize="sm" fontWeight="700" color="text.muted">
        {title}
      </Text>
      {body}
    </Box>
  )
  const destination = row.destination ?? "none"
  return (
    <Modal
      label={`Evidence for ${row.title}`}
      onClose={onClose}
      backdrop={{ alignItems: { base: "flex-end", md: "center" }, p: { base: "0", md: "4" } }}
      w="full"
      maxW={{ base: "full", md: "560px" }}
      maxH="80vh"
      overflowY="auto"
      display="flex"
      flexDirection="column"
      gap="3"
      p="4"
    >
      <Text fontWeight="700">{row.title}</Text>
      {section("Verdict", <Text>{row.verdict ?? "none"}</Text>)}
      {section("Evidence", <Text whiteSpace="pre-wrap">{row.evidence ?? "none"}</Text>)}
      {section(
        "Files a fix would touch",
        row.files?.length ? (
          row.files.map((f) => (
            <Text key={f} fontFamily="mono" fontSize="sm">
              {f}
            </Text>
          ))
        ) : (
          <Text>none</Text>
        ),
      )}
      {section(
        "Questions",
        row.questions?.length ? (
          row.questions.map((q) => <Text key={q.id}>{q.title}</Text>)
        ) : (
          <Text>none</Text>
        ),
      )}
      {row.travels?.length
        ? section(
            "Travels with ticket",
            row.travels.map((q) => <Text key={q.id}>{q.title}</Text>),
          )
        : null}
      {section("Suggested destination", <Text>{destination}</Text>)}
      <Button emphasis="secondary" size="sm" alignSelf="flex-end" onClick={onClose}>
        Close
      </Button>
    </Modal>
  )
}

function RowItem({
  row,
  view,
  onEdit,
}: {
  row: TriageRow
  view: TriageView
  onEdit: (e: Edit) => void
}) {
  const [showEvidence, setShowEvidence] = useState(false)
  const [naming, setNaming] = useState(false)
  const [typed, setTyped] = useState("")
  const action = GROUPS.find((g) => g.action === row.action)
  const choices = destinationChoices(
    view.epic_actions,
    row.action,
    row.address,
    view.destinations,
    view.rows,
  )
  const current =
    row.destination && (choices.find((c) => c.value === row.destination)?.label ?? row.destination)
  const actionItems: MenuEntry[] = GROUPS.map((g) => ({
    value: g.action,
    label: g.short,
    current: g.action === row.action,
    onSelect: () => onEdit(switched(view.epic_actions, row, g.action)),
  }))
  const destItems: MenuEntry[] = choices.map((c) => ({
    value: c.value ?? "new",
    label: c.label,
    current: c.value !== undefined && c.value === row.destination,
    onSelect: () =>
      c.kind === "new"
        ? setNaming(true)
        : onEdit({ action: row.action ?? "", destination: c.value }),
  }))
  return (
    <Box
      display="flex"
      alignItems="center"
      flexWrap="wrap"
      gap="2"
      px="3"
      py="2"
      borderTopWidth="1px"
      borderColor="border"
      boxShadow={
        row.merge_orphan
          ? "inset 0 0 0 2px var(--chakra-colors-tone-deadlocked-fg)"
          : row.person_changed
            ? "inset 0 0 0 2px var(--chakra-colors-tone-running-fg)"
            : undefined
      }
      data-changed={row.person_changed || undefined}
      data-orphan={row.merge_orphan || undefined}
    >
      {/* On a phone the title gets its own line, in full; the controls wrap beneath it. From md it shares the line and truncates. */}
      <Text
        data-part="row-title"
        flex={{ base: "1 1 100%", md: "1" }}
        minW="0"
        fontSize="sm"
        overflow="hidden"
        textOverflow={{ md: "ellipsis" }}
        whiteSpace={{ base: "normal", md: "nowrap" }}
        overflowWrap="anywhere"
        title={row.title}
      >
        {row.title}
      </Text>
      {!row.group && !!row.overlap_with?.length && (
        <Text data-part="chain" fontSize="sm" color="text.muted" flex="1 1 100%">
          after{" "}
          {row.overlap_with
            .map((a) => view.rows.find((r) => r.address === a)?.title ?? a)
            .join(", ")}
        </Text>
      )}
      <Button
        emphasis="secondary"
        size="sm"
        h="28px"
        px="2"
        fontSize="sm"
        flexShrink={0}
        onClick={() => setShowEvidence(true)}
      >
        Evidence
      </Button>
      {showEvidence && <EvidenceSheet row={row} onClose={() => setShowEvidence(false)} />}
      <Dropdown
        name={`Action for ${row.title}`}
        label={action?.short ?? "Action"}
        items={actionItems}
      />
      {destItems.length > 0 && !naming && (
        <Dropdown
          name={`Destination for ${row.title}`}
          label={current || "Destination"}
          items={destItems}
        />
      )}
      {naming && (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            const name = typed.trim()
            if (!name) return
            setNaming(false)
            onEdit({ action: row.action ?? "", destination: name })
          }}
        >
          <chakra.input
            aria-label="New map epic name"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            placeholder="New map epic name"
            borderWidth="1px"
            borderColor="border"
            borderRadius="md"
            px="2"
            h="32px"
            autoFocus
          />
        </form>
      )}
    </Box>
  )
}

/** Rows Plan proposed to promote as one ticket: one row listing its members, with an ungroup action. */
function GroupRow({ members, onUngroup }: { members: TriageRow[]; onUngroup: () => void }) {
  return (
    <Box
      data-part="group-row"
      display="flex"
      alignItems="center"
      flexWrap="wrap"
      gap="2"
      px="3"
      py="2"
      borderTopWidth="1px"
      borderColor="border"
    >
      <Box flex="1" minW="0">
        <Text fontSize="sm" fontWeight="700">
          Group of {members.length}: one ticket
        </Text>
        <Box as="ul" listStyleType="none" fontSize="sm">
          {members.map((m) => (
            <li key={m.address}>{m.title}</li>
          ))}
        </Box>
      </Box>
      <Button
        emphasis="secondary"
        size="sm"
        h="28px"
        px="2"
        fontSize="sm"
        flexShrink={0}
        aria-label={`Ungroup ${members.map((m) => m.title).join(", ")}`}
        onClick={onUngroup}
      >
        Ungroup
      </Button>
    </Box>
  )
}

function Group({
  action,
  name,
  rows,
  view,
  open,
  onToggle,
  onEdit,
  onUngroup,
}: {
  action: ReviewAction
  name: string
  rows: TriageRow[]
  view: TriageView
  open: boolean
  onToggle: () => void
  onEdit: (changes: [TriageRow, Edit][]) => void
  onUngroup: (row: TriageRow) => void
}) {
  const setAll: MenuEntry[] = GROUPS.filter((g) => g.action !== action).map((g) => ({
    value: g.action,
    label: g.short,
    onSelect: () => onEdit(rows.map((r) => [r, switched(view.epic_actions, r, g.action)])),
  }))
  return (
    <Box
      data-part="action-group"
      display="flex"
      flexDirection="column"
      borderWidth="1px"
      borderColor="border"
      borderRadius="md"
      overflow="hidden"
    >
      <Box display="flex" alignItems="center" gap="2" flexWrap="wrap" px="3" py="2">
        <chakra.button
          type="button"
          aria-expanded={open}
          onClick={onToggle}
          display="flex"
          alignItems="center"
          gap="2"
          flex="1"
          minW="0"
          cursor="pointer"
          textAlign="left"
        >
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
      {open &&
        rows.map((r) => {
          if (!r.group)
            return <RowItem key={r.address} row={r} view={view} onEdit={(e) => onEdit([[r, e]])} />
          const members = rows.filter((m) => m.group === r.group)
          if (members[0] !== r) return null
          return <GroupRow key={r.group} members={members} onUngroup={() => onUngroup(r)} />
        })}
    </Box>
  )
}

/** The plan in five collapsible action groups; each row edits its action and destination and saves with the record revision. */
export function ReviewView({
  project,
  view,
  refetch,
  save = postSaveTriageRow,
}: {
  project: string
  view: TriageView
  refetch: () => void
  save?: SaveTriageRow
}) {
  const { drafts, edit, ungroup } = useRowEdits(project, view, refetch, save)
  const [open, setOpen] = useState<Record<string, boolean>>({ ready: true })
  const rows = view.rows
    .filter((r) => !r.gone && r.action)
    .map((r) => {
      const d = drafts[r.address]
      return d ? { ...r, ...d, person_changed: true } : r
    })
  const shown = { ...view, rows }
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
          onUngroup={ungroup}
        />
      ))}
    </Box>
  )
}
