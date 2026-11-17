import { Box, chakra, Spinner, Text } from "@chakra-ui/react"
import { lazy, Suspense, useEffect, useState, type FormEvent } from "react"
import { useNavigate, useParams } from "react-router"
import type { ChatDraft } from "../generated/server"
import { agentPath } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { MascotToggle } from "../shell/MascotToggle"
import { useShell } from "../shell/ShellContext"
import { DraftIcon } from "../shell/SidebarRows"
import type { ViMode } from "../ui/expandable-text/CodeEditor"
import { ModeText } from "../ui/expandable-text/ModeText"
import { useReadyWhen } from "../useReady"
import { useViMode } from "../vimode"
import { realChatDrafts, type ChatDraftsClient } from "./chatDrafts"
import { DraftBanner } from "./DraftBanner"
import { DraftFooter } from "./DraftFooter"
import { field } from "./DraftHeader"
import { useChatDraftAutosave } from "./useChatDraftAutosave"
import { REMOVED_NOTE, useLeaveGone } from "./useDraftLifecycle"

const CodeEditor = lazy(() => import("../ui/expandable-text/CodeEditor"))

interface PageProps {
  /** The draft to open; the route's `:id` by default. */
  id?: string
  drafts?: ChatDraftsClient
}

/** The chat draft page: the brief of a chat that does not exist yet, autosaved until Remove. */
export function ChatDraftPage({ drafts = realChatDrafts, ...props }: PageProps) {
  const params = useParams()
  const id = props.id ?? params.id ?? ""
  const leaveGone = useLeaveGone(agentPath)
  // `undefined` is still loading; `null` is no such draft.
  const [loaded, setLoaded] = useState<ChatDraft | null>()
  useEffect(() => {
    let live = true
    drafts
      .read(id)
      .then((res) => {
        if (!live) return
        if (res.kind === "gone") leaveGone(res.started)
        else setLoaded(res.kind === "ok" ? res.draft : null)
      })
      .catch(() => live && setLoaded(null))
    return () => {
      live = false
    }
    // The client is fixed for the page's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  useReadyWhen(loaded !== undefined, loaded ? "ready" : "empty", "network")

  // After a route change `loaded` still holds the previous draft until the new read lands.
  const stale = !!loaded && loaded.id !== id
  if (!loaded || stale) {
    return (
      <DetailPage title={{ kind: "Chat draft" }}>
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
  return <ChatDraftForm key={loaded.id} drafts={drafts} initial={loaded} />
}

const PLACEHOLDER = "What should the new chat start from?"

function Fact({ name, value }: { name: string; value: string }) {
  return (
    <Box minW="0">
      <Text as="dt" fontSize="xs" color="text.dim">
        {name}
      </Text>
      <Text as="dd" fontSize="sm" truncate>
        {value}
      </Text>
    </Box>
  )
}

function folderText({ folder, worktree }: ChatDraft) {
  if (folder === "worktree") return worktree ? `New worktree ${worktree}` : "New worktree"
  return folder === "trunk" ? "Trunk checkout" : "Source folder"
}

function ChatDraftForm({ drafts, initial }: { drafts: ChatDraftsClient; initial: ChatDraft }) {
  const shell = useShell()
  const navigate = useNavigate()
  const leaveGone = useLeaveGone(agentPath)
  const { facts, brief, edit, status, banner, bannerActions, remove } = useChatDraftAutosave({
    drafts,
    initial,
    onGone: leaveGone,
  })
  const viMode = useViMode()
  const [viModeNow, setViModeNow] = useState<ViMode>("INSERT")
  const [confirming, setConfirming] = useState(false)
  // Like PageHeader: the mascot toggle leads this header, so the shell drops its floating one.
  const claimHeader = shell?.claimHeader
  useEffect(() => claimHeader?.(), [claimHeader])

  const removeNow = () => {
    remove()
    navigate("/epics", { state: { note: REMOVED_NOTE } })
  }
  // Start comes with the next ticket; the form only keeps Enter from submitting the page.
  const submit = (e: FormEvent) => e.preventDefault()

  return (
    <DetailPage fill title={{ kind: "Chat draft", name: facts.label }}>
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
          <MascotToggle />
          <DraftIcon />
          <Text as="h1" fontSize="lg" fontWeight="600" minW="0" truncate>
            {facts.label}
          </Text>
          <Box flex="1" />
          {status !== "none" && (
            <Text fontSize="xs" color="text.dim" flexShrink="0" whiteSpace="nowrap">
              {status === "saving" ? "Saving..." : status === "unsaved" ? "Not saved" : "Saved"}
            </Text>
          )}
        </Box>
        <Box
          as="dl"
          display="grid"
          gridTemplateColumns="repeat(auto-fit, minmax(140px, 1fr))"
          gap="3"
          p="3"
          borderBottomWidth="1px"
          borderColor="hairline"
          flexShrink="0"
        >
          <Fact name="Project" value={facts.project} />
          <Fact name="Folder" value={folderText(facts)} />
          <Fact name="Harness" value={facts.harness} />
          <Fact name="Model" value={facts.model || "Default"} />
          <Fact name="Effort" value={facts.effort || "Default"} />
          <Fact name="Name" value={facts.name || "None"} />
        </Box>
        {banner && <DraftBanner actions={bannerActions} />}
        {viMode ? (
          <Box flex="1" minH="0" m="3" display="flex" flexDirection="column" gap="1">
            <Suspense fallback={<Spinner />}>
              <CodeEditor
                label="Brief"
                value={brief}
                onChange={edit}
                placeholder={PLACEHOLDER}
                selection={{ start: brief.length, end: brief.length }}
                onSelectionChange={() => {}}
                viMode
                onModeChange={setViModeNow}
              />
            </Suspense>
            <ModeText mode={viModeNow} />
          </Box>
        ) : (
          <chakra.textarea
            aria-label="Brief"
            name="chat-brief"
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
            value={brief}
            onChange={(e) => edit(e.target.value)}
          />
        )}
        <DraftFooter
          startLabel="Start chat"
          removeText="Remove this draft?"
          confirming={confirming}
          canAttach={false}
          reasonText={null}
          startDisabled
          pending={false}
          onAttach={() => {}}
          onAskRemove={() => setConfirming(true)}
          onKeep={() => setConfirming(false)}
          onRemove={removeNow}
        />
      </Box>
    </DetailPage>
  )
}
