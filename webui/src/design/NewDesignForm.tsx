import { Box, chakra, Spinner, Text } from "@chakra-ui/react"
import { lazy, Suspense, useEffect, useRef, useState, type FormEvent } from "react"
import { useNavigate, useParams } from "react-router"
import { Button } from "../ui/button/Button"
import type { DesignDraft } from "../generated/server"
import {
  createDraft,
  deleteDraft,
  getLastProject,
  listDrafts,
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
import { AttachButton, imageFiles, useAttachIcon } from "./ComposerImages"
import { DraftSlots, type DraftImages as DraftImagesState } from "./useDraftImages"
import { useShell } from "../shell/ShellContext"
import { DraftIcon } from "../shell/SidebarRows"
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
  list: listDrafts,
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

const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface.raised",
  borderWidth: "1px",
  borderColor: "hairline",
  borderRadius: "8px",
  _placeholder: { color: "text.dim" },
} as const

const PLACEHOLDER = "What is the problem? Who hits it? What does done look like?"

interface PageProps {
  /** The draft to open; the route's `:id` by default. */
  id?: string
  /** The projects to choose from; the epic list's by default. */
  projects?: string[]
  open?: OpenDesign
  /** Runs with the new Design's address once its map exists; `startFailure` is why the session did not start. */
  onStarted?: (address: string, startFailure?: string) => void
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

  if (!loaded) {
    return (
      <DetailPage title={{ kind: "Design draft" }}>
        <Box display="flex" alignItems="center" justifyContent="center" py="10">
          {loaded === undefined ? <Spinner /> : <Text color="text.dim">Draft not found</Text>}
        </Box>
      </DetailPage>
    )
  }
  return (
    <DraftForm
      key={loaded.draft.id}
      projects={projects}
      open={props.open}
      onStarted={props.onStarted}
      drafts={drafts}
      loaded={loaded}
    />
  )
}

function DraftForm({
  projects,
  open = postOpenDesign,
  onStarted,
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
  useDraftLifecycle({ drafts, autosave })

  const removeNow = () => {
    autosave.remove()
    navigate("/epics")
  }
  // An empty draft has nothing to lose, so it goes at once.
  const askRemove = () => (autosave.hasContent() ? setConfirming(true) : removeNow())
  const renameFocus = () => slugInput.current?.focus()
  // The Chord host calls these through the registry, so it always sees the latest closures.
  const actions = useRef({ remove: askRemove, rename: renameFocus })
  actions.current = { remove: askRemove, rename: renameFocus }
  useEffect(
    () =>
      registerDraftPage({
        remove: () => actions.current.remove(),
        rename: () => actions.current.rename(),
      }),
    [],
  )

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
          if (onStarted) onStarted(address, res.start_error)
          else {
            navigate(designHref(address).slice(1), { state: { startFailure: res.start_error } })
            shell?.closeSliding()
          }
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
        onSubmit={submit}
        flex="1"
        minH="0"
        display="flex"
        flexDirection="column"
        overflow="hidden"
      >
        <Box display="flex" alignItems="center" gap="2" px="3" pt="3" flexShrink="0">
          <DraftIcon />
          <Text
            as="h1"
            fontSize="lg"
            fontWeight="600"
            color={draft.slug.trim() ? "text" : "text.dim"}
            minW="0"
            truncate
          >
            {label}
          </Text>
          <chakra.button
            type="button"
            aria-label="Rename"
            title="Rename"
            onClick={renameFocus}
            color="text.dim"
            h="32px"
            w="32px"
            flexShrink="0"
            borderRadius="8px"
            _hover={{ bg: "surface", color: "text" }}
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden
            >
              <path d="M3 13l.6-3 6.9-6.9a1.3 1.3 0 0 1 1.8 0l.6.6a1.3 1.3 0 0 1 0 1.8L6 12.4z" />
            </svg>
          </chakra.button>
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
        </Box>
        <Box
          display="flex"
          alignItems="center"
          gap="2"
          p="3"
          borderBottomWidth="1px"
          borderColor="hairline"
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
            aria-invalid={!!slugError}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            ref={slugInput}
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
        {slugError && (
          <Text role="alert" fontSize="sm" color="tone.waiting.fg" px="3" pt="2" flexShrink="0">
            {slugError}
          </Text>
        )}
        {moved && (
          <Text fontSize="sm" color="text.dim" px="3" pt="2" flexShrink="0">
            {movedFrom} no longer exists; moved to {draft.project}
          </Text>
        )}
        {banner && <DraftBanner actions={bannerActions} />}
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
                label="Prompt"
                value={draft.idea}
                onChange={(idea) => patch({ idea })}
                placeholder={PLACEHOLDER}
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
            aria-label="Prompt"
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
        <Box flexShrink="0" px="3">
          <DraftImages state={imgs} />
        </Box>
        {sheetMode && reason && (
          <Box px="3" pb="2" flexShrink="0">
            {reasonText}
          </Box>
        )}
        <Box
          display="flex"
          alignItems="center"
          gap="2"
          p="3"
          borderTopWidth="1px"
          borderColor="hairline"
          bg="surface.raised"
          flexShrink="0"
        >
          {confirming ? (
            <>
              <Text fontSize="sm" flex="1" minW="0" color="tone.paused.fg" fontWeight="600">
                Remove this draft and its images?
              </Text>
              <Button
                type="button"
                emphasis="ghost"
                tone="muted"
                onClick={() => setConfirming(false)}
              >
                Keep
              </Button>
              <Button type="button" tone="paused" onClick={removeNow}>
                Remove
              </Button>
            </>
          ) : (
            <>
              {attachIcon && <AttachButton onClick={() => fileInput.current?.click()} />}
              <Button type="button" emphasis="ghost" tone="muted" onClick={askRemove}>
                Remove
              </Button>
              <Box flex="1" />
              {!sheetMode && reasonText}
              <Button type="submit" tone="running" disabled={pending || !!reason || !!banner}>
                {pending && <Spinner size="sm" />}
                Start design
              </Button>
            </>
          )}
        </Box>
      </Box>
    </DetailPage>
  )
}
