import { Portal } from "@chakra-ui/react"
import { lazy, Suspense } from "react"
import { useNavigate } from "react-router"
import { fetchWorktrees, postCreateSession } from "../agents/newSession"
import { startNewChatDraft, useNewDesignContext } from "../design/newDesign"
import { closeNewItem, useNewItemRequest } from "./newItemOpen"
import { useShell } from "./ShellContext"
import { useTwoFingerSwipeDown } from "./useTwoFingerSwipeDown"

const newChatDraft = () => void startNewChatDraft()

const NewSessionForm = lazy(() =>
  import("../agents/NewSessionForm").then(({ NewSessionForm }) => ({ default: NewSessionForm })),
)

/** Renders the New chat form for the open request, and keeps the New design action pointed at the current page. */
export function NewItemHost() {
  const request = useNewItemRequest()
  useNewDesignContext()
  useTwoFingerSwipeDown(newChatDraft)
  return (
    <Portal>
      <Suspense fallback={null}>{request?.kind === "agent" && <AgentDialog />}</Suspense>
    </Portal>
  )
}

function AgentDialog() {
  const navigate = useNavigate()
  const shell = useShell()
  return (
    <NewSessionForm
      fetchTrees={fetchWorktrees}
      create={postCreateSession}
      onClose={closeNewItem}
      onCreated={(id) => {
        closeNewItem()
        navigate(`/agents/${id}`, { state: { creating: true } })
        shell?.closeSliding()
      }}
    />
  )
}
