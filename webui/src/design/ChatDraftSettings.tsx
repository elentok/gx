import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { Field, field } from "../agents/formField"
import { fetchWorktrees, type FetchWorktrees } from "../agents/newSession"
import { EFFORTS, HARNESSES, tierOptions, type Harness } from "../agents/tierOptions"
import type { ProjectWorktrees } from "../generated/server"
import type { ChatSettings } from "./useChatDraftAutosave"

const FOLDERS = [
  { id: "source", label: "Source folder" },
  { id: "trunk", label: "Trunk checkout" },
  { id: "worktree", label: "New worktree" },
]

function Select({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: { value: string; label: string }[]
  onChange: (value: string) => void
}) {
  // A saved value the list no longer offers stays selectable, so it is not silently replaced.
  const listed = options.some((o) => o.value === value)
  return (
    <Field label={label} caret>
      <chakra.select
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        {...field}
        h="40px"
        appearance="none"
        pr="8"
      >
        {!listed && <option value={value}>{value}</option>}
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </chakra.select>
    </Field>
  )
}

/** The project, folder, harness, model, effort and name of a chat draft, editable until Start made a worktree. */
export function ChatDraftSettings({
  settings,
  worktreePath,
  onChange,
  fetchTrees = fetchWorktrees,
}: {
  settings: ChatSettings
  /** Set once a Start made the worktree: the folder is then that worktree and fixed. */
  worktreePath: string
  onChange: (patch: Partial<ChatSettings>) => void
  fetchTrees?: FetchWorktrees
}) {
  const [trees, setTrees] = useState<ProjectWorktrees[]>([])
  useEffect(() => {
    let live = true
    fetchTrees().then(
      (list) => live && setTrees(list),
      () => {},
    )
    return () => {
      live = false
    }
    // The fetcher is fixed for the page's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const harness: Harness = settings.harness === "codex" ? "codex" : "claude"
  const current = trees.find((p) => p.project === settings.project)
  const projects = trees.map((p) => ({ value: p.project, label: p.project }))
  // A tier the server gave no model is not offered: its id is not a model name.
  const models = [
    { value: "", label: "Default" },
    ...tierOptions(current, harness).flatMap((t) =>
      t.model ? [{ value: t.model, label: t.label }] : [],
    ),
  ]
  const efforts = [
    { value: "", label: "Default" },
    ...EFFORTS[harness].map((e) => ({ value: e, label: e })),
  ]

  return (
    <Box
      display="grid"
      gridTemplateColumns="repeat(auto-fit, minmax(180px, 1fr))"
      gap="3"
      p="3"
      borderBottomWidth="1px"
      borderColor="hairline"
      flexShrink="0"
    >
      <Select
        label="Project"
        value={settings.project}
        options={projects}
        onChange={(project) => onChange({ project })}
      />
      {worktreePath ? (
        <Box minW="0">
          <Text fontSize="13px" color="text.muted">
            Folder
          </Text>
          <Text fontSize="sm" truncate title={worktreePath}>
            Worktree {worktreePath}
          </Text>
        </Box>
      ) : (
        <>
          <Select
            label="Folder"
            value={settings.folder}
            options={FOLDERS.map((f) => ({ value: f.id, label: f.label }))}
            onChange={(folder) => onChange({ folder })}
          />
          {settings.folder === "worktree" && (
            <Field label="Worktree name">
              <chakra.input
                aria-label="Worktree name"
                autoComplete="off"
                spellCheck={false}
                {...field}
                h="40px"
                value={settings.worktree}
                onChange={(e) => onChange({ worktree: e.target.value })}
              />
            </Field>
          )}
        </>
      )}
      <Select
        label="Harness"
        value={settings.harness}
        options={HARNESSES.map((h) => ({ value: h.id, label: h.label }))}
        onChange={(next) => onChange({ harness: next })}
      />
      <Select
        label="Model"
        value={settings.model}
        options={models}
        onChange={(model) => onChange({ model })}
      />
      <Select
        label="Effort"
        value={settings.effort}
        options={efforts}
        onChange={(effort) => onChange({ effort })}
      />
      <Field label="Name" optional>
        <chakra.input
          aria-label="Name"
          autoComplete="off"
          spellCheck={false}
          {...field}
          h="40px"
          value={settings.name}
          onChange={(e) => onChange({ name: e.target.value })}
        />
      </Field>
    </Box>
  )
}
