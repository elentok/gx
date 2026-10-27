import { Box, chakra, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { refusalText, useWrite } from "../useWrite";
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

const Code = ({ children }: { children: string }) => (
  <chakra.code fontFamily="mono" fontSize="xs" color="text.muted">
    {children}
  </chakra.code>
);

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
    <Box
      role="group"
      aria-label={kindLabel(row.kind)}
      display="grid"
      gridTemplateColumns={{ base: "1fr", md: "110px 1fr auto" }}
      columnGap="3"
      rowGap="2"
      alignItems="center"
      py="3"
      borderTopWidth="1px"
      borderColor="hairline"
      css={{ "& > *": { minWidth: 0 } }}
    >
      <Text as="b" textTransform="uppercase" letterSpacing="0.05em" fontSize="md" fontFamily="condensed" fontWeight="600">
        {kindLabel(row.kind)}
      </Text>
      <chakra.input
        aria-label={`${kindLabel(row.kind)} command`}
        list={listId}
        value={row.command}
        disabled={row.mode !== "use"}
        placeholder={found ? undefined : "No command found"}
        onChange={(e) => onChange({ ...row, command: e.target.value })}
        w="100%"
        px="2"
        py="6px"
        fontFamily="mono"
        fontSize="sm"
        bg="surface"
        borderWidth="1px"
        borderColor="border"
        borderRadius="sm"
      />
      <datalist id={listId}>
        {suggestions.map((c) => (
          <option key={c.command} value={c.command} label={c.source} />
        ))}
      </datalist>
      <Box
        role="radiogroup"
        aria-label={`${kindLabel(row.kind)} mode`}
        display="inline-flex"
        justifySelf="start"
        borderWidth="1px"
        borderColor="border"
        borderRadius="sm"
        overflow="hidden"
      >
        {MODES.map((m) => {
          const on = row.mode === m.mode;
          return (
            <chakra.label
              key={m.mode}
              position="relative"
              px="10px"
              py="4px"
              fontSize="xs"
              cursor="pointer"
              color={on ? "tone.running.fg" : "text.muted"}
              bg={on ? "tone.running.bg" : "transparent"}
              borderLeftWidth={m.mode === MODES[0].mode ? "0" : "1px"}
              borderColor="border"
              _focusWithin={{
                outline: "2px solid",
                outlineColor: "tone.running.fg",
                outlineOffset: "-2px",
              }}
            >
              <chakra.input
                type="radio"
                name={`mode-${row.kind}`}
                checked={on}
                onChange={() => onChange({ ...row, mode: m.mode })}
                position="absolute"
                opacity="0"
                inset="0"
                w="100%"
                h="100%"
                m="0"
                cursor="pointer"
              />
              {m.label}
            </chakra.label>
          );
        })}
      </Box>
      <Box display="flex" alignItems="center" gap="3" gridColumn={{ base: "auto", md: "2 / 4" }}>
        <Box flexShrink="0">
          {found ? <Chip tone="done">Recommended</Chip> : <Chip>Nothing found</Chip>}
        </Box>
        <Text fontSize="xs" color="text.dim">
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
  const { run, pending, refusal, error } = useWrite();
  const problem = refusal ? refusalText(refusal) : error;
  const checks = configOf(rows);
  const incomplete = rows.some((r) => r.mode === "use" && r.command.trim() === "");

  const submit = (body: ReturnType<typeof configOf>) =>
    run(() => save({ project: project.project, target, checks: body }), {
      success: () => ({
        type: "success",
        title: savedMessage(target, project.trunk),
      }),
      onDone: () => {
        onSaved();
        onClose();
      },
    });

  return (
    <Modal label={`Set up checks · ${project.project}`} onClose={onClose} p="5" gap="4" w="100%" maxW="760px" overflowY="auto">
      <Box display="flex" justifyContent="space-between" alignItems="center">
        <Text as="h2" m="0" fontFamily="condensed" fontSize="xl" fontWeight="700" textTransform="uppercase">
          Set up checks · {project.project}
        </Text>
        <Button emphasis="ghost" tone="muted" aria-label="Close" onClick={onClose}>
          ✕
        </Button>
      </Box>
      <Text fontSize="xs" color="text.dim">
        Agents run these through <Code>gx check</Code>. A kind on "gx default" stays out of the file.
      </Text>
      <Box>
        {rows.map((row, i) => (
          <KindRow key={row.kind} row={row} check={project.kinds[i]} onChange={(next) => setRows((rs) => rs.map((r, j) => (j === i ? next : r)))} />
        ))}
      </Box>
      <Box role="radiogroup" aria-label="Save to" display="grid" gap="2">
        <Text as="b" textTransform="uppercase" letterSpacing="0.05em" fontSize="md" fontFamily="condensed" fontWeight="600">
          Save to
        </Text>
        {project.save_targets.includes("repo") && (
          <TargetOption
            value="repo"
            current={target}
            onPick={setTarget}
            label={
              <>
                Commit <Code>.gx/checks.json</Code> on {project.trunk}
              </>
            }
            note="One commit on trunk. Every clone and worktree sees it."
          />
        )}
        <TargetOption
          value="project.json"
          current={target}
          onPick={setTarget}
          label={
            <>
              Keep it in gx's <Code>project.json</Code>
            </>
          }
          note="Nothing goes in the repo. Only this machine's gx sees it."
        />
      </Box>
      <details>
        <Text as="summary" fontSize="xs" color="text.dim" cursor="pointer">
          Show the file
        </Text>
        <Box
          as="pre"
          m="0"
          mt="2"
          fontFamily="mono"
          fontSize="xs"
          color="text.muted"
          bg="surface"
          borderWidth="1px"
          borderColor="hairline"
          borderRadius="md"
          p="3"
          overflowX="auto"
        >
          {previewOf(checks, target, project.trunk)}
        </Box>
      </details>
      {problem && (
        <Text role="alert" fontSize="sm" color="tone.deadlocked.fg" overflowWrap="anywhere">
          {problem}
        </Text>
      )}
      <Box display="flex" gap="2" justifyContent="flex-end" flexWrap="wrap">
        <Button emphasis="ghost" tone="muted" disabled={pending} onClick={() => submit({})}>
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
  label: ReactNode;
  note: string;
}) {
  const on = current === value;
  return (
    <chakra.label
      display="flex"
      alignItems="flex-start"
      gap="10px"
      px="12px"
      py="10px"
      cursor="pointer"
      borderWidth="1px"
      borderRadius="md"
      borderColor={on ? "tone.running.fg" : "hairline"}
      bg={on ? "tone.running.bg" : "transparent"}
    >
      <chakra.input type="radio" name="save-target" checked={on} onChange={() => onPick(value)} mt="4px" />
      <Box>
        <Text fontSize="md">{label}</Text>
        <Text fontSize="xs" color="text.dim">
          {note}
        </Text>
      </Box>
    </chakra.label>
  );
}
