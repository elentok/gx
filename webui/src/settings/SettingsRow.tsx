import { Box, chakra, Text } from "@chakra-ui/react";
import { useEffect, useState, type ReactNode } from "react";
import type { SettingChange, SettingsEntry, SettingsWriteResult } from "../generated/server";
import { DEV_FRONTEND_KEY, useDev } from "../dev";
import { Button } from "../ui/button/Button";
import { Chip } from "../ui/chip/Chip";
import { Modal } from "../ui/modal/Modal";
import { refusalText, useWrite } from "../useWrite";
import { entryLabel, numberListHint, removeChange, setChange } from "./settingsApi";

function valueText(e: SettingsEntry): string {
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
  /** The control is switched off for a reason shown beside it. */
  locked: boolean;
  setDraft: (d: string | undefined) => void;
  save: (value: unknown) => void;
  remove: () => void;
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

function BoolControl({ entry, draft, pending, locked, save }: ControlProps) {
  const on = draft === "true";
  return (
    <chakra.button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={entryLabel(entry)}
      disabled={pending || locked}
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

const listItems = (e: SettingsEntry): unknown[] => {
  const v = e.set ? e.value : e.default;
  return Array.isArray(v) ? v : [];
};

// Each add or remove saves the whole list; an empty list is a real value, not a reset.
function ListControl({ entry, pending, save }: ControlProps) {
  const numeric = entry.type === "number-list";
  const items = listItems(entry);
  const [text, setText] = useState("");
  const [problem, setProblem] = useState<string>();
  const add = () => {
    const value = text.trim();
    if (value === "") return;
    const hint = numeric ? numberListHint(value) : undefined;
    if (hint || (numeric && value.includes(","))) {
      setProblem(hint ?? "Add one number at a time");
      return;
    }
    setProblem(undefined);
    setText("");
    save([...items, numeric ? Number(value) : value]);
  };
  return (
    <Box display="grid" gap="2" w="100%">
      <Box display="flex" gap="1.5" flexWrap="wrap" data-testid="list-tags">
        {items.length === 0 && (
          <Text fontSize="sm" color="text.dim">
            Empty list
          </Text>
        )}
        {items.map((item, i) => (
          <Box key={i} display="inline-flex" alignItems="center" gap="1.5" px="2.5" py="0.5" borderWidth="1px" borderColor="border" borderRadius="full" fontFamily="mono" fontSize="12.5px">
            {String(item)}
            <chakra.button type="button" aria-label={`Remove ${String(item)}`} disabled={pending} onClick={() => save(items.filter((_, j) => j !== i))} color="text.dim">
              ×
            </chakra.button>
          </Box>
        ))}
      </Box>
      <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
        <chakra.input
          type="text"
          aria-label={`Add to ${entryLabel(entry)}`}
          aria-invalid={problem ? true : undefined}
          value={text}
          borderColor={problem ? "tone.deadlocked.fg" : "border"}
          onChange={(e) => setText(e.currentTarget.value)}
          onKeyDown={(e) => e.key === "Enter" && add()}
          {...inputProps}
        />
        <Button size="sm" disabled={pending || text.trim() === ""} onClick={add}>
          Add
        </Button>
      </Box>
      {problem && (
        <Text role="alert" fontSize="xs" color="tone.deadlocked.fg">
          {problem}
        </Text>
      )}
    </Box>
  );
}

// The server never sends a secret's value; the field is write-only and emptied the moment it is sent.
function SecretControl({ entry, pending, save, remove }: ControlProps) {
  const [replacing, setReplacing] = useState(false);
  const [text, setText] = useState("");
  const done = () => {
    setText("");
    setReplacing(false);
  };
  return (
    <>
      <Chip tone={entry.set ? "done" : "muted"}>{entry.set ? "Set" : "Not set"}</Chip>
      {replacing ? (
        <>
          <chakra.input
            type="password"
            autoComplete="off"
            aria-label={`New ${entryLabel(entry)}`}
            value={text}
            onChange={(e) => setText(e.currentTarget.value)}
            borderColor="border"
            {...inputProps}
          />
          <Button
            size="sm"
            disabled={pending || text === ""}
            onClick={() => {
              save(text);
              done();
            }}
          >
            Save
          </Button>
          <Button size="sm" emphasis="ghost" tone="muted" onClick={done}>
            Cancel
          </Button>
        </>
      ) : (
        <>
          <Button size="sm" emphasis="secondary" tone="muted" disabled={pending} onClick={() => setReplacing(true)}>
            {entry.set ? "Replace" : "Set"}
          </Button>
          {entry.set && (
            <Button size="sm" emphasis="secondary" tone="deadlocked" disabled={pending} onClick={remove}>
              Clear
            </Button>
          )}
        </>
      )}
    </>
  );
}

/** The control for each entry type; a type with no entry here shows its value read-only. */
const CONTROLS: Record<string, (p: ControlProps) => ReactNode> = {
  "string-list": ListControl,
  "number-list": ListControl,
  secret: SecretControl,
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

// Turning the one-time code on makes the page safer, so only turning it off (or resetting away a set value) asks.
const needsConfirm = (e: SettingsEntry, change: SettingChange) =>
  !!e.risky && !(e.risky_off_only && "value" in change && change.value === true);

/** In a project, a key with a global counterpart is inherited until the project file holds it. */
function projectTexts(e: SettingsEntry, project: string) {
  const global = displayValue(e.global_value);
  if (e.override === "cap") {
    return {
      inherit: (
        <>
          No per-project cap (the global cap of <b>{global}</b> still applies)
        </>
      ),
      start: `Set a cap for ${project}`,
      back: "Remove cap",
    };
  }
  return {
    inherit:
      e.override === "follows-global" ? (
        <>
          Follows global mode (<b>{global}</b>)
        </>
      ) : (
        <>
          Uses the global value <b>{global}</b>
        </>
      ),
    start: `Override for ${project}`,
    back: `Use global value (${global})`,
  };
}

export function SettingsRow({
  entry,
  pendingRestart,
  onWrite,
  project,
}: {
  entry: SettingsEntry;
  pendingRestart: boolean;
  onWrite: WriteSetting;
  /** The project on the page; its overridable keys show inherit and override states. */
  project?: string;
}) {
  const write = useWrite({ toastRefusal: false });
  // Presence comes from the snapshot's Dev block, which repaints on dev-changed; the entries are read once.
  const noDevCheckout = entry.key === DEV_FRONTEND_KEY && !useDev().has_checkout;
  const [draft, setDraft] = useState<string>();
  const [saved, setSaved] = useState(false);
  const [restartNeeded, setRestartNeeded] = useState(false);
  const [notices, setNotices] = useState<string[]>([]);
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

  const [confirming, setConfirming] = useState<SettingChange>();
  const run = (change: SettingChange) => {
    setNotices([]);
    void write.run(() => onWrite([change]), {
      onDone: (res) => {
        setDraft(undefined);
        setSaved(true);
        setNotices(res.notices ?? []);
        if (res.restart_needed?.includes(entry.key)) setRestartNeeded(true);
      },
    });
  };
  const send = (change: SettingChange) => (needsConfirm(entry, change) ? setConfirming(change) : run(change));

  const Control = CONTROLS[entry.type];
  const defaultText = displayValue(entry.default);
  const overridable = project !== undefined && !!entry.global_key && !entry.read_only;
  const texts = overridable ? projectTexts(entry, project) : undefined;
  const inherited = overridable && !entry.set;
  // Turning a project's evolve switch back on removes the key: unset follows the global mode.
  const followsGlobal = entry.override === "follows-global";
  const saveValue = (value: unknown) => send(followsGlobal && value === true ? removeChange(entry.key) : setChange(entry.key, value));
  return (
    <Box px="3.5" py="3" display="grid" gap="1.5" _notFirst={{ borderTopWidth: "1px", borderTopColor: "hairline" }}>
      {/* No wrap: a long label wraps itself, so the reset link stays top-right on this row. */}
      <Box data-mock-part="title-row" display="flex" gap="2" alignItems="center">
        <Text fontWeight="600" minW="0">
          {entryLabel(entry)}
        </Text>
        {entry.set && entry.type !== "secret" && <Box title="Changed from default" w="7px" h="7px" flexShrink="0" borderRadius="full" bg="tone.running.fg" />}
        {entry.risky && <Chip tone="waiting">Careful</Chip>}
        {entry.restart && (
          <Box data-mock-part="restart-badge">
            <Chip tone="waiting">{restartNeeded || pendingRestart ? "Restart needed to apply" : "Needs restart"}</Chip>
          </Box>
        )}
        {overridable && entry.set && (
          <Box data-mock-part="overridden-chip">
            <Chip tone="running">Overridden</Chip>
          </Box>
        )}
        <Box flex="1" />
        {entry.set && entry.type !== "secret" && (
          <chakra.button
            type="button"
            data-mock-part="reset-link"
            disabled={write.pending}
            onClick={() => send(removeChange(entry.key))}
            fontSize="12.5px"
            fontWeight="600"
            color="tone.running.fg"
            flexShrink="0"
            whiteSpace="nowrap"
          >
            {texts ? texts.back : `Reset to default (${defaultText})`}
          </chakra.button>
        )}
      </Box>
      <Text fontSize="13px" color="text.muted" maxW="62ch">
        {entry.description}
      </Text>
      {inherited && texts ? (
        <Box display="flex" gap="3" alignItems="center" flexWrap="wrap">
          <Text data-mock-part="inherits-global" fontSize="sm" color="text.muted" css={{ "& b": { color: "var(--chakra-colors-text)" } }}>
            {texts.inherit}
          </Text>
          <Button
            size="sm"
            emphasis="secondary"
            tone="muted"
            disabled={write.pending}
            onClick={() => send(setChange(entry.key, followsGlobal ? false : entry.global_value))}
          >
            {texts.start}
          </Button>
        </Box>
      ) : Control ? (
        <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
          <Control
            entry={entry}
            draft={shown}
            dirty={dirty}
            invalid={refused}
            pending={write.pending}
            locked={noDevCheckout}
            setDraft={setDraft}
            save={saveValue}
            remove={() => send(removeChange(entry.key))}
          />
        </Box>
      ) : (
        <Text fontSize="sm" color="text.muted" data-testid="setting-value">
          {valueText(entry)}
        </Text>
      )}
      {noDevCheckout && (
        <Text fontSize="sm" color="text.muted">
          Off: gx has no Dev checkout. Set the Dev checkout path below.
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
      {notices.map((n) => (
        <Text key={n} data-mock-part="switch-notice" role="note" fontSize="sm" bg="tone.waiting.bg" color="tone.waiting.fg" px="3" py="2" borderRadius="md">
          {n}
        </Text>
      ))}
      <Text fontFamily="mono" fontSize="11px" color="text.dim">
        {entry.key}
      </Text>
      {confirming && (
        <Modal label={`Change ${entryLabel(entry)}?`} onClose={() => setConfirming(undefined)} maxW="440px" p="5" gap="3" data-mock-part="confirm-dialog">
          <Text as="h3" fontWeight="700" fontSize="16px">
            Change {entryLabel(entry)}?
          </Text>
          <Text fontSize="sm">{entry.confirm_text}</Text>
          <Text fontSize="13px" color="text.muted">
            The value is checked before it is written. Nothing is saved until you confirm.
          </Text>
          <Box display="flex" gap="2" justifyContent="flex-end">
            <Button emphasis="secondary" tone="muted" onClick={() => setConfirming(undefined)}>
              Cancel
            </Button>
            <Button
              tone="deadlocked"
              onClick={() => {
                run(confirming);
                setConfirming(undefined);
              }}
            >
              Change it
            </Button>
          </Box>
        </Modal>
      )}
    </Box>
  );
}
