import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useWrite } from "../useWrite";
import { Button } from "../ui/button/Button";
import { Chip } from "../ui/chip/Chip";
import { Modal } from "../ui/modal/Modal";
import { configOf, presetRows, previewOf, savedMessage, type Candidate, type Mode, type Row, type SaveChecks, type SaveTarget } from "./setup";
import type { ProjectChecks } from "./status";

const MODES: { mode: Mode; label: string }[] = [
  { mode: "use", label: "Use command" },
  { mode: "default", label: "gx default" },
  { mode: "off", label: "Off" },
];

const kindLabel = (kind: string) => kind[0].toUpperCase() + kind.slice(1);

function KindRow({ row, check, onChange }: { row: Row; check: ProjectChecks["kinds"][number]; onChange: (next: Row) => void }) {
  const found = check.recommended as Candidate | undefined;
  const suggestions = (check.candidates as Candidate[]).filter((c) => c.command !== found?.command);
  const listId = `suggest-${check.kind}`;
  const source =
    row.mode === "use" && found && row.command === found.command
      ? `Found in ${found.source}`
      : row.mode === "off"
        ? "Agents skip this check."
        : `Runs the gx default: ${check.default}`;
  return (
    <Box role="group" aria-label={kindLabel(row.kind)} display="grid" gap="2" py="3" borderTopWidth="1px" borderColor="border">
      <Box display="flex" flexWrap="wrap" alignItems="center" gap="3">
        <Text as="b" w="90px" fontFamily="condensed" fontWeight="600">
          {kindLabel(row.kind)}
        </Text>
        <chakra.input
          aria-label={`${kindLabel(row.kind)} command`}
          list={listId}
          value={row.command}
          disabled={row.mode !== "use"}
          placeholder={found ? undefined : "No command found"}
          onChange={(e) => onChange({ ...row, command: e.target.value })}
          flex="1"
          minW="200px"
          h="36px"
          px="2"
          fontFamily="mono"
          fontSize="sm"
          bg="surface"
          borderWidth="1px"
          borderColor="border"
          borderRadius="sm"
          _disabled={{ opacity: 0.6 }}
        />
        <datalist id={listId}>
          {suggestions.map((c) => (
            <option key={c.command} value={c.command} label={c.source} />
          ))}
        </datalist>
        <Box role="radiogroup" aria-label={`${kindLabel(row.kind)} mode`} display="flex" gap="3">
          {MODES.map((m) => (
            <chakra.label key={m.mode} display="flex" alignItems="center" gap="1" fontSize="sm" cursor="pointer">
              <input type="radio" name={`mode-${row.kind}`} checked={row.mode === m.mode} onChange={() => onChange({ ...row, mode: m.mode })} />
              {m.label}
            </chakra.label>
          ))}
        </Box>
      </Box>
      <Box display="flex" alignItems="center" gap="2" pl={{ base: "0", md: "90px" }}>
        {found ? <Chip tone="done">Recommended</Chip> : <Chip>Nothing found</Chip>}
        <Text fontSize="sm" color="text.muted">
          {source}
        </Text>
      </Box>
    </Box>
  );
}

/** Edits one project's check config and saves it to the repo or to project.json. */
export function SetupDialog({
  project,
  save,
  onSaved,
  onClose,
}: {
  project: ProjectChecks;
  save: SaveChecks;
  /** Runs after a save the server accepted, so the page can read the status again. */
  onSaved: () => void;
  onClose: () => void;
}) {
  const [rows, setRows] = useState(() => presetRows(project));
  const [target, setTarget] = useState<SaveTarget>(project.save_targets.includes("repo") ? "repo" : "project.json");
  const { run, pending } = useWrite();
  const checks = configOf(rows);
  const incomplete = rows.some((r) => r.mode === "use" && r.command.trim() === "");

  const submit = (body: ReturnType<typeof configOf>) =>
    run(() => save({ project: project.project, target, checks: body }), {
      success: () => ({ type: "success", title: savedMessage(target, project.trunk) }),
      onDone: () => {
        onSaved();
        onClose();
      },
    });

  return (
    <Modal label={`Set up checks · ${project.project}`} onClose={onClose} p="5" gap="3" w="860px" overflowY="auto">
      <Box display="flex" justifyContent="space-between" alignItems="center">
        <Text as="h2" m="0" fontFamily="condensed" fontSize="24px" fontWeight="600">
          Set up checks · {project.project}
        </Text>
        <Button emphasis="ghost" tone="muted" aria-label="Close" onClick={onClose}>
          ✕
        </Button>
      </Box>
      <Text color="text.muted">Agents run these through gx check. A kind on "gx default" stays out of the file.</Text>
      <Box>
        {rows.map((row, i) => (
          <KindRow
            key={row.kind}
            row={row}
            check={project.kinds[i]}
            onChange={(next) => setRows((rs) => rs.map((r, j) => (j === i ? next : r)))}
          />
        ))}
      </Box>
      <Box role="radiogroup" aria-label="Save to" display="grid" gap="2">
        <Text as="b">Save to</Text>
        {project.save_targets.includes("repo") && (
          <TargetOption
            value="repo"
            current={target}
            onPick={setTarget}
            label={`Commit .gx/checks.json on ${project.trunk}`}
            note="One commit on trunk. Every clone and worktree sees it."
          />
        )}
        <TargetOption
          value="project.json"
          current={target}
          onPick={setTarget}
          label="Keep it in gx's project.json"
          note="Nothing goes in the repo. Only this machine's gx sees it."
        />
      </Box>
      <details>
        <summary>Show the file</summary>
        <Box as="pre" fontFamily="mono" fontSize="sm" bg="surface.raised" borderRadius="md" p="2" overflowX="auto">
          {previewOf(checks, target, project.trunk)}
        </Box>
      </details>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" disabled={pending} onClick={() => submit({})}>
          Keep gx defaults
        </Button>
        <Button emphasis="main" disabled={pending || incomplete} onClick={() => submit(checks)}>
          Save
        </Button>
      </Box>
    </Modal>
  );
}

function TargetOption({
  value,
  current,
  onPick,
  label,
  note,
}: {
  value: SaveTarget;
  current: SaveTarget;
  onPick: (t: SaveTarget) => void;
  label: string;
  note: string;
}) {
  return (
    <chakra.label display="flex" alignItems="flex-start" gap="2" cursor="pointer">
      <input type="radio" name="save-target" checked={current === value} onChange={() => onPick(value)} />
      <Box>
        <Text>{label}</Text>
        <Text fontSize="sm" color="text.muted">
          {note}
        </Text>
      </Box>
    </chakra.label>
  );
}
