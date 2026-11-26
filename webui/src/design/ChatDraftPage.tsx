import { Box, chakra, Spinner, Text } from "@chakra-ui/react"
import { LuPencil } from "react-icons/lu"
import { lazy, Suspense, useEffect, useRef, useState, type ClipboardEvent } from "react"
import { useNavigate, useParams } from "react-router"
import type { FetchWorktrees } from "../agents/newSession"
import { usePromptFocus } from "../agents/promptFocus"
import { registerChatDraftPage } from "../shell/draftPageActions"
import { PROMPT_BOX_SELECTOR } from "../ui/expandable-text/look"
import type { ChatDraft } from "../generated/server"
import { HistoryButton, HistoryHost } from "../history/HistoryButton"
import { HistoryNotices } from "../history/HistoryNotices"
import { useDraftHistory } from "../history/usePickHistory"
import { postChatPick, type ListHistory, type PickChatHistory } from "../history/api"
import { useLatest } from "../hooks"
import { agentPath } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { StateHeader } from "../page/StateHeader"
import { useClaimHeader } from "../page/useClaimHeader"
import { MascotToggle } from "../shell/MascotToggle"
import { ChatDraftIcon } from "../shell/SidebarRows"
import type { ViMode } from "../ui/expandable-text/CodeEditor"
import { ModeText } from "../ui/expandable-text/ModeText"
import { submitOnCmdEnter } from "../ui/expandable-text/PromptBox"
import { TextBox } from "../ui/text-box/TextBox"
import { useReadyWhen } from "../useReady"
import { useViMode } from "../vimode"
import { realChatDrafts, type ChatDraftsClient } from "./chatDrafts"
import type { FetchDefaults } from "./chatDraftDefaults"
import { ChatDraftChips, ChatDraftCrumb, useLaunchDefaults, useTrees } from "./ChatDraftSettings"
import { imageFiles } from "./ComposerImages"
import { chatDraftImages } from "./draftImageApi"
import { DraftImages } from "./DraftImages"
import { DraftSlots, useDraftImages } from "./useDraftImages"
import { DraftBanner } from "./DraftBanner"
import { DraftFooter } from "./DraftFooter"
import { useDraftMenu } from "./useDraftMenu"
import { SaveIndicator } from "./SaveIndicator"
import { useChatDraftAutosave } from "./useChatDraftAutosave"
import { CREATING_STATE, REMOVED_NOTE, useLeaveGone } from "./useDraftLifecycle"

const CodeEditor = lazy(() => import("../ui/expandable-text/CodeEditor"))

interface PageProps {
  /** The draft to open; the route's `:id` by default. */
  id?: string
  drafts?: ChatDraftsClient
  /** Replaces the project list fetch, for tests. */
  fetchTrees?: FetchWorktrees
  /** Replaces the launch-default fetch, for tests. */
  fetchDefaults?: FetchDefaults
  pickHistory?: PickChatHistory
  listHistory?: ListHistory
}

/** The chat draft page: the brief and launch settings of a chat that does not exist yet, autosaved until Remove. */
export function ChatDraftPage({
  drafts = realChatDrafts,
  fetchTrees,
  fetchDefaults,
  pickHistory,
  listHistory,
  ...props
}: PageProps) {
  const params = useParams()
  const id = props.id ?? params.id ?? ""
  const leaveGone = useLeaveGone(agentPath, CREATING_STATE)
  // `undefined` is still loading; `null` is no such draft.
  const [loaded, setLoaded] = useState<ChatDraft | null>()
  // The client is fixed for the page's life, so the read uses the latest of them without re-running.
  const latest = useLatest({ drafts, leaveGone })
  useEffect(() => {
    let live = true
    latest.current.drafts
      .read(id)
      .then((res) => {
        if (!live) return
        if (res.kind === "gone") latest.current.leaveGone(res.started)
        else setLoaded(res.kind === "ok" ? res.draft : null)
      })
      .catch(() => live && setLoaded(null))
    return () => {
      live = false
    }
  }, [id])

  useReadyWhen(loaded !== undefined, loaded ? "ready" : "empty", "network")

  // After a route change `loaded` still holds the previous draft until the new read lands.
  const stale = !!loaded && loaded.id !== id
  if (!loaded || stale) {
    return (
      <DetailPage title={{ kind: "Chat draft" }}>
        <StateHeader title="Chat draft" />
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
    <ChatDraftForm
      key={loaded.id}
      drafts={drafts}
      initial={loaded}
      fetchTrees={fetchTrees}
      fetchDefaults={fetchDefaults}
      pickHistory={pickHistory}
      listHistory={listHistory}
    />
  )
}

const PLACEHOLDER = "What should the new chat start from?"

function ChatDraftForm({
  drafts,
  initial,
  fetchTrees,
  fetchDefaults,
  pickHistory = postChatPick,
  listHistory,
}: {
  drafts: ChatDraftsClient
  initial: ChatDraft
  fetchTrees?: FetchWorktrees
  fetchDefaults?: FetchDefaults
  pickHistory?: PickChatHistory
  listHistory?: ListHistory
}) {
  const navigate = useNavigate()
  const leaveGone = useLeaveGone(agentPath, CREATING_STATE)
  const {
    facts,
    brief,
    edit,
    settings,
    editSettings,
    status,
    banner,
    bannerActions,
    pickHistory: pick,
    remove,
    start,
    starting,
    startError,
  } = useChatDraftAutosave({ drafts, initial, onGone: leaveGone })
  const trees = useTrees(fetchTrees)
  const harness = settings.harness === "codex" ? "codex" : "claude"
  const defaults = useLaunchDefaults(harness, fetchDefaults)
  // An upload or failed tile stays with its draft.
  const slot = useRef(new DraftSlots()).current.current(initial.images)
  // The draft exists from the start, so an upload never has to save the text first.
  const imgs = useDraftImages({
    slot,
    draftId: () => initial.id,
    ensureDraft: async () => initial.id,
    upload: chatDraftImages.upload,
    remove: chatDraftImages.remove,
    url: chatDraftImages.url,
  })
  // An empty draft is a normal state: it opens with the placeholder and cannot start yet.
  // The empty brief and a pending save block Start with no reason text.
  const startReason = imgs.uploading
    ? "Uploading images..."
    : imgs.failed
      ? "Remove or retry the failed images."
      : undefined
  const startBlocked = !brief.trim() || !!startReason || banner || starting
  const viMode = useViMode()
  const [viModeNow, setViModeNow] = useState<ViMode>("INSERT")
  // The mascot toggle leads this header, so the shell drops its floating one.
  useClaimHeader()

  const formRef = useRef<HTMLDivElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const titleInput = useRef<HTMLInputElement>(null)
  const focusBrief = () => formRef.current?.querySelector<HTMLElement>(PROMPT_BOX_SELECTOR)?.focus()
  // New chat draft opens this page asking for the brief's focus, and on an empty page it asks again.
  usePromptFocus(formRef)
  // A draft with no brief and no images has nothing to lose, so the registry removes it with no confirm.
  const empty = !brief.trim() && imgs.count === 0
  const draftMenu = useDraftMenu({
    kind: "chat-draft",
    id: initial.id,
    name: settings.name || facts.label,
    empty,
    deleteNow: remove,
    onRemoved: () => navigate("/epics", { state: { note: REMOVED_NOTE } }),
  })
  const pageState = useLatest({
    empty,
    focusBrief,
    deleteNow: remove,
    rename: () => titleInput.current?.focus(),
  })
  useEffect(
    () =>
      registerChatDraftPage({
        isEmpty: () => pageState.current.empty,
        focusBrief: () => pageState.current.focusBrief(),
        deleteNow: () => pageState.current.deleteNow(),
        rename: () => pageState.current.rename(),
      }),
    [],
  )

  const history = useDraftHistory({
    pick: (entry) =>
      pick((revision) => pickHistory({ entry_id: entry.id, draft_id: initial.id, revision })),
    onPicked: (draft) => imgs.reset(draft?.images ?? []),
    focus: focusBrief,
  })

  const startNow = () => {
    if (startBlocked) return
    void start((session) => navigate(agentPath(session), { state: CREATING_STATE }))
  }
  const pasted = (e: ClipboardEvent) => {
    const files = imageFiles(e.clipboardData?.files)
    if (files.length === 0) return
    e.preventDefault()
    void imgs.addFiles(files)
  }

  return (
    <DetailPage fill title={{ kind: "Chat draft", name: facts.label }}>
      {/* Not a <form>: nothing is submitted, Start calls `startNow`. */}
      <Box ref={formRef} flex="1" minH="0" display="flex" flexDirection="column" overflow="hidden">
        <Box display="flex" alignItems="center" gap="2" pt="3" flexShrink="0">
          <MascotToggle />
          <ChatDraftIcon />
          <TextBox
            kind="line"
            look="bare"
            size="title"
            label="Title"
            hideLabel
            name="chat-title"
            ref={titleInput}
            placeholder="Untitled chat"
            value={settings.name}
            onChange={(e) => editSettings({ name: e.currentTarget.value })}
            flex="1"
            minW="0"
          />
          <chakra.button
            type="button"
            aria-label="Rename"
            title="Rename"
            onClick={() => titleInput.current?.focus()}
            color="text.dim"
            h="32px"
            w="32px"
            flexShrink="0"
            display="grid"
            placeItems="center"
            borderRadius="8px"
            _hover={{ bg: "surface", color: "text" }}
          >
            <LuPencil size={14} />
          </chakra.button>
          <HistoryButton onClick={history.show} />
          <SaveIndicator status={status} />
          {draftMenu.menu}
        </Box>
        <ChatDraftCrumb
          settings={settings}
          trees={trees}
          worktreePath={facts.worktree_path}
          onChange={editSettings}
        />
        <HistoryNotices notices={history.notices} />
        {banner && <DraftBanner actions={bannerActions} />}
        {history.open && (
          <HistoryHost
            list="chat-drafts"
            pending={imgs.uploading > 0}
            listHistory={listHistory}
            onRestore={history.restore}
            onClose={history.hide}
          />
        )}
        <Box
          flex="1"
          minH="0"
          mx="0"
          my="3"
          display="flex"
          flexDirection="column"
          bg="surface.raised"
          borderWidth="1px"
          borderColor="hairline"
          borderRadius="12px"
          overflow="hidden"
          onPaste={pasted}
        >
          {viMode ? (
            <Box flex="1" minH="0" display="flex" flexDirection="column" gap="1" p="2">
              <Suspense fallback={<Spinner />}>
                <CodeEditor
                  label="Brief"
                  value={brief}
                  onChange={edit}
                  placeholder={PLACEHOLDER}
                  selection={{ start: brief.length, end: brief.length }}
                  viMode
                  bare
                  onSubmit={startNow}
                  onModeChange={setViModeNow}
                />
              </Suspense>
            </Box>
          ) : (
            <TextBox
              kind="prose"
              look="bare"
              page
              label="Brief"
              hideLabel
              name="chat-brief"
              flex="1"
              minH="0"
              resize="none"
              placeholder={PLACEHOLDER}
              value={brief}
              onChange={(e) => edit(e.currentTarget.value)}
              onKeyDown={(e) => submitOnCmdEnter(e, startNow)}
            />
          )}
          <Box flexShrink="0" px="0">
            <DraftImages state={imgs} />
          </Box>
          <Box display="flex" alignItems="center" gap="2" px="3" py="2" flexShrink="0">
            {viMode && <ModeText mode={viModeNow} />}
            <Box flex="1" />
            <ChatDraftChips
              settings={settings}
              tree={trees.find((p) => p.project === settings.project)}
              defaults={defaults}
              onChange={editSettings}
            />
          </Box>
        </Box>
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
        {startError && (
          <Box role="alert" pb="2" fontSize="sm" color="tone.error.fg" flexShrink="0">
            <Text>{startError}</Text>
            {facts.worktree_path && (
              <Text>
                Start made a worktree at {facts.worktree_path}; pressing Start again runs there
              </Text>
            )}
          </Box>
        )}
        <DraftFooter
          startLabel="Start chat"
          canAttach
          startReason={startReason}
          startDisabled={startBlocked}
          pending={starting}
          onAttach={() => fileInput.current?.click()}
          onRemove={draftMenu.remove}
          onStart={startNow}
        />
      </Box>
    </DetailPage>
  )
}
