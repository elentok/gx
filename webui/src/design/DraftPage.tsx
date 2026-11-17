import { Box, chakra, Spinner, Text } from "@chakra-ui/react"
import { DraftHeader, field } from "./DraftHeader"
import { DraftFooter } from "./DraftFooter"
import { lazy, Suspense, useEffect, useRef, useState, type FormEvent } from "react"
import { useLocation, useNavigate, useParams } from "react-router"
import type { DesignDraft } from "../generated/server"
import {
  createDraft,
  deleteDraft,
  getLastProject,
  pingDraft,
  readDraft,
  removeDraftIfEmpty,
  saveDraft,
  saveDraftFields,
  setLastProject,
  type DraftsClient,
} from "./drafts"
import { removeDraftImage, uploadDraftImage } from "./draftImageApi"
import { DraftImages } from "./DraftImages"
import { imageFiles, useAttachIcon } from "./ComposerImages"
import { DraftSlots, type DraftImages as DraftImagesState } from "./useDraftImages"
import { useShell } from "../shell/ShellContext"
import { useEpicList } from "../epics/list"
import { DetailPage } from "../page/DetailPage"
import { designHref } from "../hrefs"
import { postOpenDesign, type OpenDesign } from "./open"
import { saveUnsaved, type Restored } from "./unsaved"
import { useDesignWrite } from "./SessionControls"
import { useWide } from "../useWide"
import { useViMode } from "../vimode"
import { useReadyWhen } from "../useReady"
import { ModeText } from "../ui/expandable-text/ModeText"
import type { ViMode } from "../ui/expandable-text/CodeEditor"
import { DraftBanner } from "./DraftBanner"
import { type Fields } from "./draftFields"
import { slugProblem } from "./draftSlug"
import { useDraftAutosave } from "./useDraftAutosave"
import { useDraftLifecycle, useLeaveGone } from "./useDraftLifecycle"
import { registerDraftPage } from "../shell/draftPageActions"

const CodeEditor = lazy(() => import("../ui/expandable-text/CodeEditor"))

const realDrafts: DraftsClient = {
  read: readDraft,
  save: saveDraft,
  saveFields: saveDraftFields,
  remove: deleteDraft,
  ping: pingDraft,
  removeIfEmpty: removeDraftIfEmpty,
  getLastProject,
  setLastProject,
  uploadImage: uploadDraftImage,
  removeImage: removeDraftImage,
}

/** Why Start is unavailable, or undefined when it is ready. */
function startBlocker(draft: Fields, imgs: DraftImagesState): string | undefined {
  if (!draft.idea.trim()) return "Write a prompt first"
  if (!draft.slug.trim()) return "Name the epic to start."
  const problem = slugProblem(draft.slug)
  if (problem) return problem
  if (imgs.failed) return `Remove the failed ${imgs.failed === 1 ? "image" : "images"} first`
  if (imgs.uploading) return "Wait for the upload to finish"
}

const PLACEHOLDER = "What is the problem? Who hits it? What does done look like?"

interface PageProps {
  /** The draft to open; the route's `:id` by default. */
  id?: string
  /** The projects to choose from; the epic list's by default. */
  projects?: string[]
  open?: OpenDesign
  drafts?: DraftsClient
}

interface Loaded {
  draft: DesignDraft
  restored?: Restored
  lastProject: string
}

/** The Design draft page: the prompt, epic name and images of a Design with no map yet. It autosaves the draft until Start or Remove. */
export function DraftPage({ drafts = realDrafts, ...props }: PageProps) {
  const params = useParams()
  const id = props.id ?? params.id ?? ""
  const leaveGone = useLeaveGone()
  const { list } = useEpicList()
  const projects =
    props.projects ??
    [...new Set([...(list?.projects ?? []), ...(list?.epics ?? []).map((e) => e.project)])].sort()
  // `undefined` is still loading; `null` is no such draft.
  const [loaded, setLoaded] = useState<Loaded | null>()
  useEffect(() => {
    let live = true
    ;(async () => {
      const lastProject = await drafts.getLastProject().catch(() => "")
      const stuck = await saveUnsaved(drafts.save)
      const mine = stuck?.draft.id === id ? stuck : undefined
      const res = await drafts.read(id).catch(() => undefined)
      let next: Loaded | null = null
      if (res?.kind === "ok") {
        // A prompt the server never took replaces its copy; its revision is the one the save was made on.
        next = mine
          ? {
              draft: { ...res.draft, idea: mine.draft.idea, revision: mine.draft.revision },
              restored: mine.state,
              lastProject,
            }
          : { draft: res.draft, lastProject }
      } else if (!res && mine) {
        next = { draft: mine.draft, restored: mine.state, lastProject }
      }
      if (!live) return
      // A bookmarked draft that was started or removed sends the person where it went.
      if (res?.kind === "gone") leaveGone(res.started)
      else setLoaded(next)
    })().catch(() => live && setLoaded(null))
    return () => {
      live = false
    }
    // The client is fixed for the page's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  useReadyWhen(loaded !== undefined, loaded ? "ready" : "empty", "network")

  // After a route change `loaded` still holds the previous draft until the new read lands; show the spinner, not its form.
  const stale = !!loaded && loaded.draft.id !== id
  if (!loaded || stale) {
    return (
      <DetailPage title={{ kind: "Design draft" }}>
        <Box display="flex" alignItems="center" justifyContent="center" py="10">
          {loaded === undefined || stale ? (
            <Spinner />
          ) : (
            <Text color="text.dim">Draft not found</Text>
          )}
        </Box>
      </DetailPage>
    )
  }
  return (
    <DraftForm
      key={loaded.draft.id}
      projects={projects}
      open={props.open}
      drafts={drafts}
      loaded={loaded}
    />
  )
}

function DraftForm({
  projects,
  open = postOpenDesign,
  drafts,
  loaded: { draft: initial, restored, lastProject },
}: Omit<PageProps, "id" | "projects"> & {
  projects: string[]
  drafts: DraftsClient
  loaded: Loaded
}) {
  const sheetMode = !useWide()
  const shell = useShell()
  const navigate = useNavigate()
  const slugInput = useRef<HTMLInputElement | null>(null)
  // An upload or failed tile stays with its draft.
  const slot = useRef(new DraftSlots()).current.current(initial.images)
  const autosave = useDraftAutosave({ drafts, initial, restored, projects, lastProject, slot })
  const {
    draft,
    label,
    patch,
    pickProject,
    status,
    banner,
    bannerActions,
    moved,
    movedFrom,
    imgs,
  } = autosave
  const { run, pending } = useDesignWrite()
  const fileInput = useRef<HTMLInputElement>(null)
  const attachIcon = useAttachIcon()
  const viMode = useViMode()
  const [viModeNow, setViModeNow] = useState<ViMode>("INSERT")
  const [confirming, setConfirming] = useState(false)
  // Like PageHeader: the mascot toggle leads this header, so the shell drops its floating one.
  const claimHeader = shell?.claimHeader
  useEffect(() => claimHeader?.(), [claimHeader])
  useDraftLifecycle({ drafts, autosave })

  const removeNow = () => {
    autosave.remove()
    navigate("/epics")
  }
  // An empty draft has nothing to lose, so it goes at once.
  const askRemove = () => (autosave.hasContent() ? setConfirming(true) : removeNow())
  const renameFocus = () => slugInput.current?.focus()
  // The Chord host calls these through the registry, so it always sees the latest closures.
  const formRef = useRef<HTMLFormElement | null>(null)
  const putFocus = () =>
    formRef.current
      ?.querySelector<HTMLElement>("textarea[name='design-idea'], [contenteditable='true']")
      ?.focus()
  // The menu that started New design returns focus to its trigger a moment later, so for a short
  // while focus that lands outside the form goes back to the Prompt until the person acts.
  const stopHold = useRef<() => void>(() => {})
  const focusPrompt = () => {
    stopHold.current()
    putFocus()
    const back = (e: FocusEvent) => {
      if (!formRef.current?.contains(e.target as Node)) putFocus()
    }
    const stop = () => {
      clearTimeout(timer)
      document.removeEventListener("focusin", back)
      document.removeEventListener("pointerdown", stop, true)
      document.removeEventListener("keydown", stop, true)
    }
    const timer = setTimeout(stop, 2000)
    document.addEventListener("focusin", back)
    document.addEventListener("pointerdown", stop, true)
    document.addEventListener("keydown", stop, true)
    stopHold.current = stop
  }
  const actions = useRef({ remove: askRemove, rename: renameFocus, focusPrompt })
  actions.current = { remove: askRemove, rename: renameFocus, focusPrompt }
  useEffect(
    () =>
      registerDraftPage({
        remove: () => actions.current.remove(),
        rename: () => actions.current.rename(),
        isEmpty: () => !autosave.hasContent(),
        focusPrompt: () => actions.current.focusPrompt(),
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )
  // A page opened by New design asks for the Prompt's focus; a click or a bookmark does not.
  const { key: locationKey, state: locationState } = useLocation()
  const focusWanted = (locationState as { focusPrompt?: boolean } | null)?.focusPrompt === true
  useEffect(() => {
    if (focusWanted) focusPrompt()
    return () => stopHold.current()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [locationKey, focusWanted])

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
          }),
        ),
      {
        onDone: (res) => {
          // With start_error the map exists though no session started; the Design page offers the retry.
          const address = `${draft.project}:${slug}`
          navigate(designHref(address).slice(1), { state: { startFailure: res.start_error } })
          shell?.closeSliding()
        },
      },
    )
  }

  const reason = startBlocker(draft, imgs)
  const slugError = slugProblem(draft.slug)
  const reasonText = reason && (
    <Text fontSize="12px" color="tone.waiting.fg">
      {reason}
    </Text>
  )

  return (
    <DetailPage fill title={{ kind: "Design draft", name: label }}>
      <Box
        as="form"
        ref={formRef}
        onSubmit={submit}
        flex="1"
        minH="0"
        display="flex"
        flexDirection="column"
        overflow="hidden"
      >
        <DraftHeader
          label={label}
          project={draft.project}
          projects={projects}
          slug={draft.slug}
          slugError={slugError}
          slugInput={slugInput}
          status={status}
          onRename={renameFocus}
          onPickProject={pickProject}
          onSlug={(slug) => patch({ slug })}
        />
        {slugError && (
          <Text role="alert" fontSize="sm" color="tone.waiting.fg" pt="2" flexShrink="0">
            {slugError}
          </Text>
        )}
        {moved && (
          <Text fontSize="sm" color="text.dim" pt="2" flexShrink="0">
            {movedFrom} no longer exists; moved to {draft.project}
          </Text>
        )}
        {banner && <DraftBanner actions={bannerActions} />}
        {viMode ? (
          <Box
            flex="1"
            minH="0"
            mx="0"
            my="3"
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
                label="Prompt"
                value={draft.idea}
                onChange={(idea) => patch({ idea })}
                placeholder={PLACEHOLDER}
                selection={{ start: draft.idea.length, end: draft.idea.length }}
                onSelectionChange={() => {}}
                viMode
                onModeChange={setViModeNow}
                autoFocus={focusWanted}
              />
            </Suspense>
            <ModeText mode={viModeNow} />
          </Box>
        ) : (
          <chakra.textarea
            aria-label="Prompt"
            name="design-idea"
            autoComplete="off"
            {...field}
            flex="1"
            minH="0"
            mx="0"
            my="3"
            resize="none"
            px="4"
            py="3"
            lineHeight="1.6"
            placeholder={PLACEHOLDER}
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
        <Box flexShrink="0">
          <DraftImages state={imgs} />
        </Box>
        {sheetMode && reason && (
          <Box pb="2" flexShrink="0">
            {reasonText}
          </Box>
        )}
        <DraftFooter
          confirming={confirming}
          canAttach={attachIcon}
          reasonText={!sheetMode && reasonText}
          startDisabled={pending || !!reason || !!banner}
          pending={pending}
          onAttach={() => fileInput.current?.click()}
          onAskRemove={askRemove}
          onKeep={() => setConfirming(false)}
          onRemove={removeNow}
        />
      </Box>
    </DetailPage>
  )
}
