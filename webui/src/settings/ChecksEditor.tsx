import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import type { SettingsEntry, SettingsWriteResult } from "../generated/server"
import { Button } from "../ui/button/Button"
import { TextBox } from "../ui/text-box/TextBox"
import { withKeys } from "../uniqueKeys"
import { refusalText, useWrite } from "../useWrite"
import {
  CHECK_KINDS,
  CHECKS_KEY,
  SLOW_TOOLS,
  checksErrors,
  checksToModel,
  setupCommands,
  modelToChange,
  type CheckForm,
  type CheckKind,
  type CheckMode,
  type ChecksModel,
} from "./checks"
import { ChangedDot, SaveStatus, Segmented, TagChip } from "./parts"
import type { WriteSetting } from "./SettingsRow"

const MODES: { value: CheckMode; label: string }[] = [
  { value: "default", label: "default" },
  { value: "off", label: "off" },
  { value: "one", label: "one command" },
  { value: "pair", label: "changed + full" },
]

const CommandBox = ({
  name,
  label,
  value,
  onChange,
}: {
  name: string
  label: string
  value: string
  onChange: (v: string) => void
}) => (
  <TextBox
    kind="line"
    verbatim
    name={name}
    label={label}
    hideLabel
    value={value}
    onChange={(e) => onChange(e.currentTarget.value)}
  />
)

function AlwaysList({
  kind,
  items,
  onChange,
}: {
  kind: CheckKind
  items: string[]
  onChange: (items: string[]) => void
}) {
  const [text, setText] = useState("")
  const add = () => {
    const value = text.trim()
    if (!value) return
    setText("")
    onChange([...items, value])
  }
  return (
    <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
      <Text fontSize="13px" color="text.muted">
        Also run on every non-full run:
      </Text>
      {withKeys(items, (item) => item).map(({ item, index, key }) => (
        <TagChip
          key={key}
          label={item}
          onRemove={() => onChange(items.filter((_, j) => j !== index))}
        />
      ))}
      <TextBox
        kind="line"
        verbatim
        size="sm"
        name="check-always-command"
        label={`Add always command for ${kind}`}
        hideLabel
        placeholder="+ add command"
        value={text}
        onChange={(e) => setText(e.currentTarget.value)}
        onKeyDown={(e) => e.key === "Enter" && add()}
        onBlur={add}
        w="150px"
      />
    </Box>
  )
}

function Limits({
  form,
  onChange,
}: {
  form: CheckForm
  onChange: (patch: Partial<CheckForm>) => void
}) {
  return (
    <Box
      display="grid"
      gridTemplateColumns={`repeat(${form.slowIsMap ? 2 + SLOW_TOOLS.length : 3}, minmax(0, 1fr))`}
      gap="1.5"
      // A label word wider than its narrow column breaks instead of covering the next one.
      overflowWrap="anywhere"
    >
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
  )
}

const LimitField = ({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (v: string) => void
}) => (
  <TextBox
    kind="line"
    verbatim
    name="check-limit"
    label={`${label} limit`}
    value={value}
    onChange={(e) => onChange(e.currentTarget.value)}
    minW="0"
    // Keeps the boxes level when only some labels wrap.
    alignSelf="end"
  />
)

function KindCard({
  kind,
  form,
  error,
  onChange,
}: {
  kind: CheckKind
  form: CheckForm
  error?: string
  onChange: (patch: Partial<CheckForm>) => void
}) {
  return (
    <Box
      display="grid"
      gap="2"
      p="2.5"
      borderWidth="1px"
      borderColor="border"
      borderRadius="10px"
      bg="surface"
      data-testid={`check-${kind}`}
    >
      <Box display="flex" gap="2" alignItems="center">
        <Text as="h4" fontWeight="600">
          {kind}
        </Text>
        {form.mode !== "default" && (
          <Box
            title="Set"
            data-testid={`check-dot-${kind}`}
            w="7px"
            h="7px"
            borderRadius="full"
            bg="tone.running.fg"
          />
        )}
      </Box>
      <Segmented
        label={`${kind} mode`}
        options={MODES}
        value={form.mode}
        onChange={(mode) => onChange({ mode })}
      />
      {form.mode === "default" && (
        <Text fontSize="13px" color="text.dim">
          Uses the gx default for {kind}.
        </Text>
      )}
      {form.mode === "one" && (
        <CommandBox
          name="check-command"
          label={`${kind} command`}
          value={form.command}
          onChange={(command) => onChange({ command })}
        />
      )}
      {form.mode === "pair" && (
        <>
          <CommandBox
            name="check-changed-command"
            label={`${kind} changed command`}
            value={form.changed}
            onChange={(changed) => onChange({ changed })}
          />
          <CommandBox
            name="check-full-command"
            label={`${kind} full command`}
            value={form.full}
            onChange={(full) => onChange({ full })}
          />
          <AlwaysList kind={kind} items={form.always} onChange={(always) => onChange({ always })} />
        </>
      )}
      {kind === "test" && (form.mode === "one" || form.mode === "pair") && (
        <Limits form={form} onChange={onChange} />
      )}
      {error && (
        <Text role="alert" fontSize="xs" color="tone.deadlocked.fg">
          {error}
        </Text>
      )}
    </Box>
  )
}

/** The project's Checks section: one card per kind, saved together as the `checks` key. */
export function ChecksEditor({
  entry,
  repoChecksFile,
  onWrite,
}: {
  entry?: SettingsEntry
  repoChecksFile: boolean
  onWrite: WriteSetting
}) {
  const initial = checksToModel(entry?.set ? entry.value : undefined)
  const [draft, setDraft] = useState<ChecksModel>()
  const write = useWrite({ toastRefusal: false })
  const [saved, setSaved] = useState(false)
  const model = draft ?? initial
  const errors = checksErrors(model)
  const setup = setupCommands(model)
  const dirty =
    draft !== undefined &&
    JSON.stringify(modelToChange(draft)) !== JSON.stringify(modelToChange(initial))
  const refusal = write.refusal as SettingsWriteResult | undefined
  const errorText = refusal
    ? `Not saved: ${refusalText(refusal)}`
    : write.error && `Not saved: ${write.error}`
  const edit = (kind: CheckKind, patch: Partial<CheckForm>) => {
    setSaved(false)
    setDraft({ ...model, [kind]: { ...model[kind], ...patch } })
  }
  return (
    <Box
      mt="6"
      borderWidth="1px"
      borderColor="border"
      borderRadius="lg"
      bg="surface.raised"
      px="3.5"
      py="3"
      display="grid"
      gap="1.5"
    >
      <Box display="flex" gap="2" alignItems="center">
        <Text fontWeight="600">Checks</Text>
        {entry?.set && <ChangedDot />}
      </Box>
      <Text fontSize="13px" color="text.muted" maxW="62ch">
        {entry?.description ?? "Commands `gx check` runs."} A .gx/checks.json in the repo wins over
        this.
      </Text>
      {repoChecksFile && (
        <Text
          role="note"
          data-testid="repo-checks-notice"
          fontSize="sm"
          bg="tone.waiting.bg"
          color="tone.waiting.fg"
          px="3"
          py="2"
          borderRadius="md"
        >
          Showing the active checks from .gx/checks.json. Edit that file in the repo to change these
          commands.
        </Text>
      )}
      <chakra.fieldset
        disabled={repoChecksFile}
        data-mock-part="checks-editor"
        display="grid"
        gap="2"
        minW="0"
      >
        {CHECK_KINDS.map((kind) => (
          <KindCard
            key={kind}
            kind={kind}
            form={model[kind]}
            error={errors[kind]}
            onChange={(patch) => edit(kind, patch)}
          />
        ))}
        {setup.length > 0 && (
          <Box data-testid="checks-setup" display="grid" gap="1">
            <Text fontWeight="600" fontSize="sm">
              setup
            </Text>
            <Text
              fontFamily="mono"
              fontSize="12px"
              color="text.muted"
              whiteSpace="pre-wrap"
              overflowWrap="anywhere"
            >
              {setup.join("\n")}
            </Text>
          </Box>
        )}
      </chakra.fieldset>
      <Box display="flex" gap="2" alignItems="center">
        <Button
          size="sm"
          disabled={repoChecksFile || !dirty || Object.keys(errors).length > 0 || write.pending}
          onClick={() =>
            void write.run(() => onWrite([modelToChange(model)]), {
              onDone: () => {
                setDraft(undefined)
                setSaved(true)
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
      <SaveStatus errorText={errorText || undefined} saved={saved} />
      <Text fontFamily="mono" fontSize="11px" color="text.dim">
        {CHECKS_KEY}
      </Text>
    </Box>
  )
}
