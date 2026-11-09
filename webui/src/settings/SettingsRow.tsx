import { Box, chakra, Text } from "@chakra-ui/react";
import { useEffect, useState, type ReactNode } from "react";
import type { SettingChange, SettingsEntry, SettingsWriteResult } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Chip } from "../ui/chip/Chip";
import { refusalText, useWrite } from "../useWrite";
import { removeChange, setChange } from "./settingsApi";

export const entryLabel = (e: SettingsEntry) => {
  const text = (e.path[e.path.length - 1] ?? e.key).replace(/-/g, " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
};

export function valueText(e: SettingsEntry): string {
  if (e.type === "secret") return e.set ? "Set" : "Not set";
  return displayValue(e.set ? e.value : e.default);
}

function displayValue(v: unknown): string {
  if (v === undefined || v === null || v === "") return "—";
  if (typeof v === "boolean") return v ? "on" : "off";
  if (Array.isArray(v)) return v.length ? v.join(", ") : "—";
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

export type WriteSetting = (changes: SettingChange[]) => Promise<SettingsWriteResult>;

/** What a control needs: the entry, the draft it shows and how to change or save it. */
interface ControlProps {
  entry: SettingsEntry;
  draft: string;
  dirty: boolean;
  invalid: boolean;
  pending: boolean;
  setDraft: (d: string | undefined) => void;
  save: (value: unknown) => void;
}

const HINTS: Record<string, string> = { duration: "e.g. 30s, 5m, 2h", size: "e.g. 50G" };

const inputProps = {
  h: "36px",
  px: "3",
  minW: "0",
  maxW: "220px",
  borderWidth: "1px",
  borderRadius: "8px",
  bg: "surface",
  color: "text",
} as const;

// The browser sends text for a number; a draft that is not one goes as typed so the server refuses it with its reason.
const draftValue = (type: string, draft: string): unknown => {
  const text = draft.trim();
  return type === "number" && text !== "" && !Number.isNaN(Number(text)) ? Number(text) : text;
};

function TextControl({ entry, draft, dirty, invalid, pending, setDraft, save }: ControlProps) {
  return (
    <>
      <chakra.input
        type="text"
        aria-label={entryLabel(entry)}
        aria-invalid={invalid || undefined}
        value={draft}
        inputMode={entry.type === "number" ? "decimal" : undefined}
        borderColor={invalid ? "tone.deadlocked.fg" : "border"}
        onChange={(e) => setDraft(e.currentTarget.value)}
        {...inputProps}
      />
      {HINTS[entry.type] && (
        <Text fontSize="xs" color="text.dim">
          {HINTS[entry.type]}
        </Text>
      )}
      <Button size="sm" disabled={!dirty || pending} onClick={() => save(draftValue(entry.type, draft))}>
        Save
      </Button>
      {dirty && (
        <Button size="sm" emphasis="ghost" tone="muted" onClick={() => setDraft(undefined)}>
          Cancel
        </Button>
      )}
    </>
  );
}

function BoolControl({ entry, draft, pending, save }: ControlProps) {
  const on = draft === "true";
  return (
    <chakra.button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={entryLabel(entry)}
      disabled={pending}
      onClick={() => save(!on)}
      w="40px"
      h="22px"
      borderRadius="full"
      position="relative"
      bg={on ? "tone.running.fg" : "border"}
    >
      <Box position="absolute" top="2px" left={on ? "20px" : "2px"} w="18px" h="18px" borderRadius="full" bg="surface" />
    </chakra.button>
  );
}

function EnumControl({ entry, draft, pending, save }: ControlProps) {
  return (
    <Box display="inline-flex" borderWidth="1px" borderColor="border" borderRadius="8px" overflow="hidden" role="group" aria-label={entryLabel(entry)}>
      {(entry.enum ?? []).map((option) => (
        <chakra.button
          key={option}
          type="button"
          aria-pressed={option === draft}
          disabled={pending}
          onClick={() => option !== draft && save(option)}
          px="3"
          py="1.5"
          fontWeight="600"
          color="text.muted"
          _notFirst={{ borderLeftWidth: "1px", borderLeftColor: "border" }}
          _pressed={{ bg: "tone.running.bg", color: "tone.running.fg" }}
        >
          {option}
        </chakra.button>
      ))}
    </Box>
  );
}

/** The control for each entry type; a type with no entry here shows its value read-only. */
const CONTROLS: Record<string, (p: ControlProps) => ReactNode> = {
  bool: BoolControl,
  number: TextControl,
  duration: TextControl,
  size: TextControl,
  string: TextControl,
  enum: EnumControl,
};

const serverText = (e: SettingsEntry) => {
  const v = e.set ? e.value : e.default;
  return v === undefined || v === null ? "" : String(v);
};

const SAVED_MS = 2000;

export function SettingsRow({ entry, pendingRestart, onWrite }: { entry: SettingsEntry; pendingRestart: boolean; onWrite: WriteSetting }) {
  const write = useWrite({ toastRefusal: false });
  const [draft, setDraft] = useState<string>();
  const [saved, setSaved] = useState(false);
  const [restartNeeded, setRestartNeeded] = useState(false);
  useEffect(() => {
    if (!saved) return;
    const timer = setTimeout(() => setSaved(false), SAVED_MS);
    return () => clearTimeout(timer);
  }, [saved]);

  const current = serverText(entry);
  const shown = draft ?? current;
  const dirty = shown !== current;
  // useWrite keeps the refusal as a Refusable; every write here returns a SettingsWriteResult.
  const refusal = write.refusal as SettingsWriteResult | undefined;
  const refused = refusal !== undefined && (!refusal.keys?.length || refusal.keys.includes(entry.key));
  const errorText = refused ? `Not saved: ${refusalText(refusal)}` : write.error && `Not saved: ${write.error}`;

  const send = (change: SettingChange) =>
    void write.run(() => onWrite([change]), {
      onDone: (res) => {
        setDraft(undefined);
        setSaved(true);
        if (res.restart_needed?.includes(entry.key)) setRestartNeeded(true);
      },
    });

  const Control = CONTROLS[entry.type];
  const defaultText = displayValue(entry.default);
  return (
    <Box px="3.5" py="3" display="grid" gap="1.5" _notFirst={{ borderTopWidth: "1px", borderTopColor: "hairline" }}>
      <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
        <Text fontWeight="600">{entryLabel(entry)}</Text>
        {entry.set && <Box title="Changed from default" w="7px" h="7px" borderRadius="full" bg="tone.running.fg" />}
        {entry.restart && (
          <Box data-mock-part="restart-badge">
            <Chip tone="waiting">{restartNeeded || pendingRestart ? "Restart needed to apply" : "Needs restart"}</Chip>
          </Box>
        )}
        <Box flex="1" />
        {entry.set && (
          <chakra.button
            type="button"
            data-mock-part="reset-link"
            disabled={write.pending}
            onClick={() => send(removeChange(entry.key))}
            fontSize="12.5px"
            fontWeight="600"
            color="tone.running.fg"
          >
            Reset to default ({defaultText})
          </chakra.button>
        )}
      </Box>
      <Text fontSize="13px" color="text.muted" maxW="62ch">
        {entry.description}
      </Text>
      {Control ? (
        <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
          <Control
            entry={entry}
            draft={shown}
            dirty={dirty}
            invalid={refused}
            pending={write.pending}
            setDraft={setDraft}
            save={(value) => send(setChange(entry.key, value))}
          />
        </Box>
      ) : (
        <Text fontSize="sm" color="text.muted" data-testid="setting-value">
          {valueText(entry)}
        </Text>
      )}
      {errorText && (
        <Text data-mock-part="error-message" role="alert" fontSize="xs" color="tone.deadlocked.fg">
          {errorText}
        </Text>
      )}
      {saved && !errorText && (
        <Text data-mock-part="saved-message" role="status" fontSize="xs" color="tone.done.fg">
          <span aria-hidden>✓ </span>Saved
        </Text>
      )}
      <Text fontFamily="mono" fontSize="11px" color="text.dim">
        {entry.key}
      </Text>
    </Box>
  );
}
