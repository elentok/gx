import { Box, chakra, Popover, Portal, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { Field, field } from "../agents/formField"
import { fetchWorktrees, type FetchWorktrees } from "../agents/newSession"
import { EFFORTS, HARNESSES, tierOptions, type Harness } from "../agents/tierOptions"
import { baseName, WorktreePicker, type Option } from "../agents/WorktreePicker"
import type { ProjectWorktrees } from "../generated/server"
import { useLatest } from "../hooks"
import { modelName } from "../modelName"
import { fetchLaunchDefaults, type FetchDefaults, type LaunchDefaults } from "./chatDraftDefaults"
import type { ChatSettings } from "./useChatDraftAutosave"

const TRUNK_NAMES = ["main", "master"]
const harnessOf = (s: ChatSettings): Harness => (s.harness === "codex" ? "codex" : "claude")

/** The project list, empty until it arrives or if it fails. */
export function useTrees(fetchTrees: FetchWorktrees = fetchWorktrees): ProjectWorktrees[] {
  const [trees, setTrees] = useState<ProjectWorktrees[]>([])
  // The fetcher is fixed for the page's life.
  const latestFetch = useLatest(fetchTrees)
  useEffect(() => {
    let live = true
    latestFetch.current().then(
      (list) => live && setTrees(list),
      () => {},
    )
    return () => {
      live = false
    }
  }, [])
  return trees
}

/** What the harness launches with when model and effort stay empty; empty until known. */
export function useLaunchDefaults(
  harness: Harness,
  fetcher: FetchDefaults = fetchLaunchDefaults,
): LaunchDefaults {
  const [got, setGot] = useState<{ harness: Harness } & LaunchDefaults>()
  // The fetcher is fixed for the page's life.
  const latestFetch = useLatest(fetcher)
  useEffect(() => {
    let live = true
    latestFetch.current(harness).then(
      (d) => live && setGot({ harness, ...d }),
      () => {},
    )
    return () => {
      live = false
    }
  }, [harness])
  // Another harness's defaults are never shown while this one loads.
  return got?.harness === harness ? got : { model: "", effort: "" }
}

/** The project and folder pickers: the same comboboxes as the New chat dialog, in one breadcrumb row. */
export function ChatDraftCrumb({
  settings,
  trees,
  worktreePath,
  onChange,
}: {
  settings: ChatSettings
  trees: ProjectWorktrees[]
  /** Set once a Start made the worktree: the folder is then that worktree and fixed. */
  worktreePath: string
  onChange: (patch: Partial<ChatSettings>) => void
}) {
  const current = trees.find((p) => p.project === settings.project)
  const trunk = current?.worktrees.find((w) => TRUNK_NAMES.includes(baseName(w)))
  const rows: Option[] = [
    { key: "source", label: "Source folder", path: current?.root ?? "source" },
    { key: "trunk", label: "Trunk checkout", path: trunk ?? "trunk" },
    // The root and the trunk are the two rows above; every other worktree is listed by its name.
    ...(current?.worktrees ?? [])
      .filter((w) => w !== trunk && w !== current?.root)
      .map((w) => ({ key: w, label: baseName(w), path: w })),
  ]
  const creating = settings.folder === "worktree" ? settings.worktree : ""
  const picked =
    settings.folder === "dir"
      ? rows.find((r) => r.path === settings.dir)
      : (rows.find((r) => r.key === settings.folder) ?? rows[0])
  return (
    <Box display="flex" alignItems="center" gap="2" pt="2" flexShrink="0">
      <Box flex="0 1 40%" minW="0">
        <WorktreePicker
          label="Project"
          hideLabel
          canCreate={false}
          rows={trees.map((p) => ({ key: p.project, label: p.project, path: "" }))}
          picked={{ key: settings.project, label: settings.project, path: "" }}
          // A picked worktree belongs to the old project.
          onPick={(o) =>
            onChange(
              settings.folder === "dir" ? { project: o.key, folder: "source" } : { project: o.key },
            )
          }
        />
      </Box>
      <Text color="text.dim" fontSize="lg" aria-hidden>
        /
      </Text>
      <Box flex="1" minW="0">
        {worktreePath ? (
          <Text fontSize="sm" truncate title={worktreePath}>
            Worktree {worktreePath}
          </Text>
        ) : (
          <WorktreePicker
            label="Folder"
            hideLabel
            rows={rows}
            picked={picked}
            creating={creating}
            onPick={(o) =>
              onChange(
                o.create
                  ? { folder: "worktree", worktree: o.create }
                  : o.key === "source" || o.key === "trunk"
                    ? { folder: o.key }
                    : { folder: "dir", dir: o.path },
              )
            }
          />
        )}
      </Box>
    </Box>
  )
}

type Opt = { value: string; label: string }

/**
 * One row per label, in the order labels first appear. The tiers can name the same model as the default
 * row, or as each other. Of equal rows the selected one stays, else the first, so the select always has the
 * value it holds.
 */
function uniqueByLabel(list: Opt[], selected: string): Opt[] {
  const kept = new Map<string, Opt>()
  for (const o of list) {
    const had = kept.get(o.label)
    if (!had || (o.value === selected && had.value !== selected)) kept.set(o.label, o)
  }
  return [...kept.values()]
}

/** The harness, model and effort choices. An unset model or effort is labelled with what the chat will launch with. */
function useChoices(
  settings: ChatSettings,
  tree: ProjectWorktrees | undefined,
  defaults: LaunchDefaults,
  onChange: (patch: Partial<ChatSettings>) => void,
) {
  const harness = harnessOf(settings)
  const usingDefaultModel = settings.model === ""
  // An unset model launches on the worker tier, so that is the default; the probe's default row only stands in
  // when the project configures no worker model.
  const workerModel = tierOptions(tree, harness).find((t) => t.id === "worker")?.model
  const defaultModel = workerModel || defaults.model
  // A tier the server gave no model is not offered: its id is not a model name.
  const candidates: Opt[] = [
    {
      value: "",
      label: defaultModel ? String(modelName(defaultModel, harness)) : "Model",
    },
    ...tierOptions(tree, harness).flatMap((t) =>
      t.model ? [{ value: t.model, label: String(modelName(t.model, harness)) }] : [],
    ),
  ]
  // A pinned or older model the list does not offer still shows by its name.
  if (settings.model && !candidates.some((m) => m.value === settings.model))
    candidates.push({ value: settings.model, label: String(modelName(settings.model, harness)) })
  const models = uniqueByLabel(candidates, settings.model)
  // The default effort belongs to the default model only.
  const defaultEffort = usingDefaultModel ? defaults.effort : ""
  const efforts: Opt[] = [
    { value: "", label: defaultEffort || "Auto" },
    ...EFFORTS[harness].map((e) => ({ value: e, label: e })),
  ]
  const harnesses: Opt[] = HARNESSES.map((h) => ({ value: h.id, label: h.label }))
  const labelOf = (list: Opt[], value: string) =>
    list.find((o) => o.value === value)?.label ?? value
  const choices = [
    {
      label: "Harness",
      value: settings.harness,
      options: harnesses,
      disabled: false,
      onChange: (next: string) => onChange({ harness: next }),
    },
    {
      label: "Model",
      value: settings.model,
      options: models,
      disabled: false,
      onChange: (model: string) => onChange({ model }),
    },
    {
      label: "Effort",
      value: settings.effort,
      options: efforts,
      // An effort needs a model, so with none known there is nothing to pair it with.
      disabled: usingDefaultModel && !defaultModel,
      onChange: (effort: string) =>
        // The server refuses an effort with no model: pin the model that would have been used.
        onChange(effort && usingDefaultModel ? { effort, model: defaultModel } : { effort }),
    },
  ]
  // The pill's text: the model and effort in use, led by the harness only when it is not Claude.
  const summary = [
    ...(harness === "claude" ? [] : [labelOf(harnesses, settings.harness)]),
    labelOf(models, settings.model),
    labelOf(efforts, settings.effort),
  ].join(" · ")
  return { choices, summary }
}

/**
 * The harness, model and effort controls: one pill that opens a popover with all three. An unset model or effort shows what the chat will launch
 * with (`defaults`), or its plain name when the server could not say. Never "Default".
 */
export function ChatDraftChips(props: {
  settings: ChatSettings
  tree: ProjectWorktrees | undefined
  defaults: LaunchDefaults
  onChange: (patch: Partial<ChatSettings>) => void
}) {
  const { choices, summary } = useChoices(
    props.settings,
    props.tree,
    props.defaults,
    props.onChange,
  )
  const [open, setOpen] = useState(false)
  return (
    <Popover.Root
      open={open}
      onOpenChange={(e) => setOpen(e.open)}
      positioning={{ placement: "top-end", gutter: 8 }}
      lazyMount
      unmountOnExit
    >
      <Popover.Trigger asChild>
        <chakra.button
          type="button"
          aria-label="Chat settings"
          display="flex"
          alignItems="center"
          gap="1.5"
          h="32px"
          px="2.5"
          minW="0"
          maxW="100%"
          fontSize="13px"
          color="text"
          bg="surface"
          borderWidth="1px"
          borderColor="hairline"
          borderRadius="full"
        >
          <Text as="span" truncate>
            {summary}
          </Text>
          <Text as="span" aria-hidden color="text.dim" fontSize="10px" flexShrink="0">
            ▾
          </Text>
        </chakra.button>
      </Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content
            role="dialog"
            aria-label="Chat settings"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="12px"
            boxShadow="lg"
            color="text"
            w="min(320px, calc(100vw - 32px))"
            p="4"
            gap="3"
            css={{ "--popover-bg": "colors.surface.raised" }}
          >
            {choices.map((c) => (
              <Field key={c.label} label={c.label} caret>
                <chakra.select
                  aria-label={c.label}
                  value={c.value}
                  disabled={c.disabled}
                  onChange={(e) => c.onChange(e.target.value)}
                  {...field}
                  h="40px"
                  appearance="none"
                  pr="8"
                >
                  {!c.options.some((o) => o.value === c.value) && (
                    <option value={c.value}>{c.value}</option>
                  )}
                  {c.options.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </chakra.select>
              </Field>
            ))}
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}
