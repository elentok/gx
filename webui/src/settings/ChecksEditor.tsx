import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import type { SettingsEntry, SettingsWriteResult } from "../generated/server";
import { Button } from "../ui/button/Button";
import { refusalText, useWrite } from "../useWrite";
import {
  CHECK_KINDS,
  CHECKS_KEY,
  SLOW_TOOLS,
  checksErrors,
  checksToModel,
  modelToChange,
  type CheckForm,
  type CheckKind,
  type CheckMode,
  type ChecksModel,
} from "./checks";
import type { WriteSetting } from "./SettingsRow";

const MODES: { mode: CheckMode; label: string }[] = [
  { mode: "default", label: "default" },
  { mode: "off", label: "off" },
  { mode: "one", label: "one command" },
  { mode: "pair", label: "changed + full" },
];

const inputProps = {
  h: "36px",
  px: "3",
  minW: "0",
  borderWidth: "1px",
  borderColor: "border",
  borderRadius: "8px",
  bg: "surface",
  color: "text",
} as const;

const TextInput = ({ label, value, onChange, wide = false }: { label: string; value: string; onChange: (v: string) => void; wide?: boolean }) => (
  <chakra.input type="text" aria-label={label} value={value} w={wide ? "100%" : "90px"} onChange={(e) => onChange(e.currentTarget.value)} {...inputProps} />
);

function AlwaysList({ kind, items, onChange }: { kind: CheckKind; items: string[]; onChange: (items: string[]) => void }) {
  const [text, setText] = useState("");
  const add = () => {
    const value = text.trim();
    if (!value) return;
    setText("");
    onChange([...items, value]);
  };
  return (
    <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
      <Text fontSize="13px" color="text.muted">
        Also run on every non-full run:
      </Text>
      {items.map((item, i) => (
        <Box key={i} display="inline-flex" alignItems="center" gap="1.5" px="2.5" py="0.5" borderWidth="1px" borderColor="border" borderRadius="full" fontFamily="mono" fontSize="12.5px">
          {item}
          <chakra.button type="button" aria-label={`Remove ${item}`} onClick={() => onChange(items.filter((_, j) => j !== i))} color="text.dim">
            ×
          </chakra.button>
        </Box>
      ))}
      <chakra.input
        type="text"
        aria-label={`Add always command for ${kind}`}
        placeholder="+ add command"
        value={text}
        onChange={(e) => setText(e.currentTarget.value)}
        onKeyDown={(e) => e.key === "Enter" && add()}
        onBlur={add}
        w="150px"
        h="26px"
        px="2.5"
        borderWidth="1px"
        borderColor="border"
        borderStyle="dashed"
        borderRadius="full"
        bg="transparent"
        color="text"
        fontSize="12.5px"
      />
    </Box>
  );
}

function Limits({ form, onChange }: { form: CheckForm; onChange: (patch: Partial<CheckForm>) => void }) {
  return (
    <Box display="grid" gridTemplateColumns={`repeat(${form.slowIsMap ? 2 + SLOW_TOOLS.length : 3}, minmax(0, 1fr))`} gap="1.5">
      <LimitField label="suite" value={form.suite} onChange={(suite) => onChange({ suite })} />
      <LimitField label="hang" value={form.hang} onChange={(hang) => onChange({ hang })} />
      {form.slowIsMap ? (
        SLOW_TOOLS.map((tool) => (
          <LimitField
            key={tool}
            label={`slow ${tool}`}
            value={form.slowMap[tool]}
            onChange={(v) => onChange({ slowMap: { ...form.slowMap, [tool]: v } })}
          />
        ))
      ) : (
        <LimitField label="slow" value={form.slow} onChange={(slow) => onChange({ slow })} />
      )}
    </Box>
  );
}

const LimitField = ({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) => (
  <chakra.label display="grid" gap="1" fontSize="11px" color="text.muted">
    {label}
    <TextInput wide label={`${label} limit`} value={value} onChange={onChange} />
  </chakra.label>
);

function KindCard({ kind, form, error, onChange }: { kind: CheckKind; form: CheckForm; error?: string; onChange: (patch: Partial<CheckForm>) => void }) {
  return (
    <Box display="grid" gap="2" p="2.5" borderWidth="1px" borderColor="border" borderRadius="10px" bg="surface" data-testid={`check-${kind}`}>
      <Box display="flex" gap="2" alignItems="center">
        <Text as="h4" fontWeight="600">
          {kind}
        </Text>
        {form.mode !== "default" && <Box title="Set" data-testid={`check-dot-${kind}`} w="7px" h="7px" borderRadius="full" bg="tone.running.fg" />}
      </Box>
      <Box display="flex" borderWidth="1px" borderColor="border" borderRadius="8px" overflow="hidden" role="group" aria-label={`${kind} mode`}>
        {MODES.map(({ mode, label }) => (
          <chakra.button
            key={mode}
            type="button"
            aria-pressed={form.mode === mode}
            onClick={() => onChange({ mode })}
            px="3"
            py="1.5"
            fontWeight="600"
            color="text.muted"
            _notFirst={{ borderLeftWidth: "1px", borderLeftColor: "border" }}
            _pressed={{ bg: "tone.running.bg", color: "tone.running.fg" }}
          >
            {label}
          </chakra.button>
        ))}
      </Box>
      {form.mode === "default" && (
        <Text fontSize="13px" color="text.dim">
          Uses the gx default for {kind}.
        </Text>
      )}
      {form.mode === "one" && <TextInput wide label={`${kind} command`} value={form.command} onChange={(command) => onChange({ command })} />}
      {form.mode === "pair" && (
        <>
          <TextInput wide label={`${kind} changed command`} value={form.changed} onChange={(changed) => onChange({ changed })} />
          <TextInput wide label={`${kind} full command`} value={form.full} onChange={(full) => onChange({ full })} />
          <AlwaysList kind={kind} items={form.always} onChange={(always) => onChange({ always })} />
        </>
      )}
      {kind === "test" && (form.mode === "one" || form.mode === "pair") && <Limits form={form} onChange={onChange} />}
      {error && (
        <Text role="alert" fontSize="xs" color="tone.deadlocked.fg">
          {error}
        </Text>
      )}
    </Box>
  );
}

/** The project's Checks section: one card per kind, saved together as the `checks` key. */
export function ChecksEditor({ entry, repoChecksFile, onWrite }: { entry?: SettingsEntry; repoChecksFile: boolean; onWrite: WriteSetting }) {
  const initial = checksToModel(entry?.set ? entry.value : undefined);
  const [draft, setDraft] = useState<ChecksModel>();
  const write = useWrite({ toastRefusal: false });
  const [saved, setSaved] = useState(false);
  const model = draft ?? initial;
  const errors = checksErrors(model);
  const dirty = draft !== undefined && JSON.stringify(modelToChange(draft)) !== JSON.stringify(modelToChange(initial));
  const refusal = write.refusal as SettingsWriteResult | undefined;
  const errorText = refusal ? `Not saved: ${refusalText(refusal)}` : write.error && `Not saved: ${write.error}`;
  const edit = (kind: CheckKind, patch: Partial<CheckForm>) => {
    setSaved(false);
    setDraft({ ...model, [kind]: { ...model[kind], ...patch } });
  };
  return (
    <Box mt="6" borderWidth="1px" borderColor="border" borderRadius="lg" bg="surface.raised" px="3.5" py="3" display="grid" gap="1.5">
      <Box display="flex" gap="2" alignItems="center">
        <Text fontWeight="600">Checks</Text>
        {entry?.set && <Box title="Changed from default" w="7px" h="7px" borderRadius="full" bg="tone.running.fg" />}
      </Box>
      <Text fontSize="13px" color="text.muted" maxW="62ch">
        {entry?.description ?? "Commands `gx check` runs."} A .gx/checks.json in the repo wins over this.
      </Text>
      {repoChecksFile && (
        <Text role="note" data-testid="repo-checks-notice" fontSize="sm" bg="tone.waiting.bg" color="tone.waiting.fg" px="3" py="2" borderRadius="md">
          This repo has a .gx/checks.json, which wins over these settings. Edits below have no effect.
        </Text>
      )}
      <Box data-mock-part="checks-editor" display="grid" gap="2">
        {CHECK_KINDS.map((kind) => (
          <KindCard key={kind} kind={kind} form={model[kind]} error={errors[kind]} onChange={(patch) => edit(kind, patch)} />
        ))}
      </Box>
      <Box display="flex" gap="2" alignItems="center">
        <Button
          size="sm"
          disabled={!dirty || Object.keys(errors).length > 0 || write.pending}
          onClick={() =>
            void write.run(() => onWrite([modelToChange(model)]), {
              onDone: () => {
                setDraft(undefined);
                setSaved(true);
              },
            })
          }
        >
          Save
        </Button>
        {dirty && (
          <Button size="sm" emphasis="ghost" tone="muted" onClick={() => setDraft(undefined)}>
            Cancel
          </Button>
        )}
      </Box>
      {errorText && (
        <Text role="alert" fontSize="xs" color="tone.deadlocked.fg">
          {errorText}
        </Text>
      )}
      {saved && !errorText && (
        <Text role="status" fontSize="xs" color="tone.done.fg">
          <span aria-hidden>✓ </span>Saved
        </Text>
      )}
      <Text fontFamily="mono" fontSize="11px" color="text.dim">
        {CHECKS_KEY}
      </Text>
    </Box>
  );
}
