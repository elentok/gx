import { Box, chakra, Spinner, Text } from "@chakra-ui/react"
import { lazy, Suspense, useCallback, useEffect, useRef, useState, type FormEvent } from "react"
import { Button } from "../ui/button/Button"
import { BottomSheet } from "../ui/modal/BottomSheet"
import type { DesignDraft } from "../generated/server"
import {
  deleteDraft,
  draftLabel,
  getLastProject,
  listDrafts,
  openLatest,
  readDraft,
  saveDraft,
  setLastProject,
  type DeleteDraft,
  type GetLastProject,
  type ListDrafts,
  type ReadDraft,
  type SaveDraft,
  type SetLastProject,
} from "./drafts"
import {
  removeDraftImage,
  uploadDraftImage,
  type RemoveDraftImage,
  type UploadDraftImage,
} from "./draftImageApi"
import { DraftImages } from "./DraftImages"
import { AttachButton, imageFiles, useAttachIcon } from "./ComposerImages"
import { DraftSlots, type DraftImages as DraftImagesState, type ImageSlot } from "./useDraftImages"
import { useShell } from "../shell/ShellContext"
import { postOpenDesign, type OpenDesign } from "./open"
import { saveUnsaved, type Restored } from "./unsaved"
import { moveLegacyDraft } from "./legacyDraft"
import { useDesignWrite } from "./SessionControls"
import { useWide } from "../useWide"
import { useViMode } from "../vimode"
import { ModeText } from "../ui/expandable-text/ModeText"
import type { ViMode } from "../ui/expandable-text/CodeEditor"

const CodeEditor = lazy(() => import("../ui/expandable-text/CodeEditor"))
import { useFocusFirstField } from "../ui/modal/useFocusFirstField"
import { DraftSwitcher } from "./DraftSwitcher"
import { DraftBanner } from "./DraftBanner"
import { hasText, type Fields } from "./draftFields"
import { useDraftAutosave } from "./useDraftAutosave"
import { useDraftSwitching, type SwitchDraft } from "./useDraftSwitching"

/** The draft calls the form makes; the prop defaults to the real ones. */
export interface DraftsClient {
  list: ListDrafts
  read: ReadDraft
  save: SaveDraft
  remove: DeleteDraft
  getLastProject: GetLastProject
  setLastProject: SetLastProject
  uploadImage: UploadDraftImage
  removeImage: RemoveDraftImage
}

const realDrafts: DraftsClient = {
  list: listDrafts,
  read: readDraft,
  save: saveDraft,
  remove: deleteDraft,
  getLastProject,
  setLastProject,
  uploadImage: uploadDraftImage,
  removeImage: removeDraftImage,
}

/** Why Start is unavailable, or undefined when it is ready. */
function startBlocker(draft: Fields, imgs: DraftImagesState): string | undefined {
  if (!draft.slug.trim()) return "Name the epic to start."
  if (!draft.idea.trim()) return "Write an idea to start."
  if (imgs.failed) return `Remove the failed ${imgs.failed === 1 ? "image" : "images"} first`
  if (imgs.uploading) return "Wait for the upload to finish"
}

const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface",
  borderWidth: "1px",
  borderColor: "transparent",
  borderRadius: "8px",
  _placeholder: { color: "text.dim" },
} as const

const sheetProps = {
  label: "New Design",
  fillViewport: true,
  desktop: { borderRadius: "lg", w: "720px", h: "520px" },
  bg: "surface.raised",
  p: "0",
  gap: "0",
  overflow: "hidden",
} as const

interface FormProps {
  projects: string[]
  open?: OpenDesign
  /** Runs with the new Design's address once its map exists; `startFailure` is why the session did not start. */
  onStarted: (address: string, startFailure?: string) => void
  /** Closing keeps the draft. */
  onClose: () => void
  drafts?: DraftsClient
}

/** The New Design writing sheet: epic name and rough idea for a Design with no map yet. It opens on the latest server draft and autosaves it until Start or Discard. */
export function NewDesignForm({ drafts = realDrafts, ...props }: FormProps) {
  // `undefined` is still loading; `null` is no draft to open.
  const [initial, setInitial] = useState<DesignDraft | null | undefined>()
  const [restored, setRestored] = useState<Restored>()
  // A new generation remounts the form on another draft.
  const [generation, setGeneration] = useState(0)
  const slots = useRef(new DraftSlots())
  // The server's last project used; a server that cannot answer leaves the first project.
  const [lastProject, setLast] = useState("")
  const fetchLast = () => drafts.getLastProject().catch(() => "")
  useEffect(() => {
    let live = true
    ;(async () => {
      const last = await fetchLast()
      const stuck = await saveUnsaved(drafts.save)
      if (stuck) {
        if (live) {
          setLast(last)
          setRestored(stuck.state)
          setInitial(stuck.draft)
        }
        return
      }
      await moveLegacyDraft(drafts.save)
      const latest = await openLatest(drafts)
      if (live) {
        setLast(last)
        setInitial(latest)
      }
    })().catch(() => live && setInitial(null))
    return () => {
      live = false
    }
    // The client is fixed for the form's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (initial === undefined) {
    return (
      <BottomSheet {...sheetProps} onClose={props.onClose}>
        <Box flex="1" display="flex" alignItems="center" justifyContent="center">
          <Spinner />
        </Box>
      </BottomSheet>
    )
  }
  const slot = slots.current.current(initial?.images)
  const switchTo: SwitchDraft = async (next, leaving) => {
    // An upload or failed tile stays with its draft, so returning finds it.
    slots.current.switchTo(next, leaving)
    // A blank draft starts on the project last picked, which this form may just have set.
    if (!next) setLast(await fetchLast())
    setRestored(undefined)
    setInitial(next)
    setGeneration((g) => g + 1)
  }
  return (
    <DraftForm
      key={generation}
      {...props}
      drafts={drafts}
      initial={initial}
      lastProject={lastProject}
      restored={restored}
      slot={slot}
      onSwitch={switchTo}
    />
  )
}

function DraftForm({
  projects,
  open = postOpenDesign,
  onStarted,
  onClose,
  drafts,
  initial,
  lastProject,
  restored,
  slot,
  onSwitch,
}: FormProps & {
  drafts: DraftsClient
  initial: DesignDraft | null
  lastProject: string
  restored?: Restored
  slot: ImageSlot
  onSwitch: SwitchDraft
}) {
  const sheetMode = !useWide()
  const workspace = useShell()?.workspace
  const slugInput = useRef<HTMLInputElement | null>(null)
  const focusFirst = useFocusFirstField()
  // Stable, so a re-render does not run the focus again.
  const slugRef = useCallback(
    (el: HTMLInputElement | null) => {
      slugInput.current = el
      focusFirst(el)
    },
    [focusFirst],
  )
  const autosave = useDraftAutosave({ drafts, initial, restored, projects, lastProject, slot })
  const {
    draft,
    patch,
    pickProject,
    name,
    saveName,
    status,
    banner,
    bannerActions,
    moved,
    movedFrom,
    imgs,
  } = autosave
  const { switchDraft, newDraft, deleteOther, loadDrafts } = useDraftSwitching({
    drafts,
    autosave,
    onSwitch,
    focusSlug: () => slugInput.current?.focus(),
  })
  const [confirming, setConfirming] = useState(false)
  // The name field under the title row; `undefined` is closed.
  const [naming, setNaming] = useState<string>()
  const { run, pending } = useDesignWrite()
  const fileInput = useRef<HTMLInputElement>(null)
  const attachIcon = useAttachIcon()
  const viMode = useViMode()
  const [viModeNow, setViModeNow] = useState<ViMode>("INSERT")

  const close = () => {
    autosave.flush()
    onClose()
  }
  const discard = () => {
    autosave.discard()
    onClose()
  }
  const commitName = (text: string) => {
    setNaming(undefined)
    saveName(text)
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const slug = draft.slug.trim()
    run(
      () =>
        autosave.start((draft_id) =>
          open({
            epic: slug,
            new_design: true,
            idea: draft.idea,
            project: draft.project,
            draft_id,
            ...(workspace ? { workspace } : {}),
          }),
        ),
      {
        onDone: (res) => {
          // With start_error the map exists though no session started; the Design page offers the retry.
          onStarted(`${draft.project}:${slug}`, res.start_error)
        },
      },
    )
  }

  const hasWork = hasText(draft) || !!name || imgs.count > 0
  // The title follows the name field while it is open.
  const shownName = naming !== undefined ? naming.trim() || undefined : name
  const label =
    draft.slug || draft.idea || shownName ? draftLabel({ name: shownName, ...draft }) : draftLabel()
  const reason = startBlocker(draft, imgs)

  return (
    <BottomSheet {...sheetProps} onClose={close} as="form" onSubmit={submit}>
      <Box
        position="relative"
        display="flex"
        alignItems="center"
        gap="2"
        px="3"
        pt="2"
        flexShrink="0"
      >
        <DraftSwitcher
          label={label}
          currentId={autosave.internals.server.current.id}
          load={loadDrafts}
          onPick={switchDraft}
          onNew={newDraft}
          onRename={() => setNaming(name ?? "")}
          onDelete={deleteOther}
        />
        <Box flex="1" />
        {status !== "none" && (
          <Text fontSize="xs" color="text.dim" flexShrink="0" whiteSpace="nowrap">
            {status === "saved" && (
              <chakra.span aria-hidden color="tone.done.fg">
                ✓{" "}
              </chakra.span>
            )}
            {status === "saving" ? "Saving..." : status === "unsaved" ? "Not saved" : "Saved"}
          </Text>
        )}
        <chakra.button
          type="button"
          aria-label="Close"
          title="Close (keeps the draft)"
          onClick={close}
          color="text.dim"
          fontSize="sm"
          fontWeight="600"
          h="36px"
          px="2"
          flexShrink="0"
          borderRadius="8px"
          _hover={{ bg: "surface", color: "text" }}
        >
          Close
        </chakra.button>
      </Box>
      {naming !== undefined && (
        <Box display="flex" alignItems="center" gap="2" px="3" pt="2" flexShrink="0">
          <chakra.input
            aria-label="Draft name"
            autoComplete="off"
            autoFocus
            {...field}
            flex="1"
            minW="0"
            h="34px"
            px="3"
            placeholder="Name this draft (optional)"
            value={naming}
            onChange={(e) => setNaming(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                commitName(naming)
              }
            }}
          />
          <Button type="button" emphasis="ghost" tone="muted" onClick={() => commitName(naming)}>
            Done
          </Button>
        </Box>
      )}
      <Box
        display="flex"
        alignItems="center"
        gap="2"
        p="3"
        borderBottomWidth="1px"
        borderColor="border"
        flexShrink="0"
      >
        {projects.length > 1 && (
          <>
            <chakra.select
              aria-label="Project"
              value={draft.project}
              onChange={(e) => pickProject(e.target.value)}
              fontSize="16px"
              h="40px"
              px="3"
              borderRadius="8px"
              bg="tone.running.bg"
              color="tone.running.fg"
              fontWeight="600"
            >
              {projects.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </chakra.select>
            <Text color="text.dim" fontSize="lg">
              /
            </Text>
          </>
        )}
        <chakra.input
          aria-label="Epic slug"
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          spellCheck={false}
          ref={slugRef}
          {...field}
          flex="1"
          minW="0"
          h="40px"
          px="3"
          fontWeight="600"
          placeholder="my-epic"
          value={draft.slug}
          onChange={(e) => patch({ slug: e.target.value })}
        />
      </Box>
      {moved && (
        <Text fontSize="sm" color="text.dim" px="3" pt="2" flexShrink="0">
          {movedFrom} no longer exists; moved to {draft.project}
        </Text>
      )}
      {banner && <DraftBanner kind={banner} actions={bannerActions} />}
      {viMode ? (
        <Box
          flex="1"
          minH="0"
          m="3"
          display="flex"
          flexDirection="column"
          gap="1"
          onPaste={(e) => {
            const files = imageFiles(e.clipboardData?.files)
            if (files.length === 0) return
            e.preventDefault()
            void imgs.addFiles(files)
          }}
        >
          <Suspense fallback={<Spinner />}>
            <CodeEditor
              label="Rough idea"
              value={draft.idea}
              onChange={(idea) => patch({ idea })}
              placeholder="What is the problem? Who hits it? What does done look like?"
              selection={{ start: draft.idea.length, end: draft.idea.length }}
              onSelectionChange={() => {}}
              viMode
              onModeChange={setViModeNow}
              autoFocus={false}
            />
          </Suspense>
          <ModeText mode={viModeNow} />
        </Box>
      ) : (
        <chakra.textarea
          aria-label="Rough idea"
          name="design-idea"
          autoComplete="off"
          {...field}
          flex="1"
          minH="0"
          m="3"
          resize="none"
          px="4"
          py="3"
          lineHeight="1.6"
          placeholder="What is the problem? Who hits it? What does done look like?"
          value={draft.idea}
          onChange={(e) => patch({ idea: e.target.value })}
          onPaste={(e) => {
            const files = imageFiles(e.clipboardData?.files)
            if (files.length === 0) return
            e.preventDefault()
            void imgs.addFiles(files)
          }}
        />
      )}
      <input
        ref={fileInput}
        type="file"
        accept="image/*"
        multiple
        hidden
        data-testid="image-input"
        onChange={(e) => {
          void imgs.addFiles(imageFiles(e.target.files))
          e.target.value = ""
        }}
      />
      <Box flexShrink="0" px="3">
        <DraftImages state={imgs} />
      </Box>
      {sheetMode && reason && !confirming && (
        <Text fontSize="12px" color="tone.waiting.fg" px="3" pb="2" flexShrink="0">
          {reason}
        </Text>
      )}
      <Box
        display="flex"
        alignItems="center"
        gap="2"
        p="3"
        borderTopWidth="1px"
        borderColor="border"
        bg="surface"
        flexShrink="0"
      >
        {confirming ? (
          <>
            <Text flex="1" fontSize="sm" color="tone.paused.fg" fontWeight="600">
              Discard this draft and its images?
            </Text>
            <Button
              type="button"
              emphasis="ghost"
              tone="muted"
              onClick={() => setConfirming(false)}
            >
              Keep
            </Button>
            <Button type="button" tone="paused" onClick={discard}>
              Discard
            </Button>
          </>
        ) : (
          <>
            {attachIcon && <AttachButton onClick={() => fileInput.current?.click()} />}
            <Button
              type="button"
              emphasis="ghost"
              tone="muted"
              onClick={() => (hasWork ? setConfirming(true) : discard())}
            >
              Discard
            </Button>
            <Box flex="1" />
            {!sheetMode && reason && (
              <Text fontSize="12px" color="tone.waiting.fg">
                {reason}
              </Text>
            )}
            <Button type="submit" tone="running" disabled={pending || !!reason || !!banner}>
              {pending && <Spinner size="sm" />}
              Start design
            </Button>
          </>
        )}
      </Box>
    </BottomSheet>
  )
}
