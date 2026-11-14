import { Portal } from "@chakra-ui/react"
import { lazy, Suspense, useEffect } from "react"
import { useNavigate } from "react-router"
import { fetchWorktrees, postCreateSession } from "../agents/newSession"
import { createDraft } from "../design/drafts"
import { draftHref } from "../hrefs"
import { closeNewItem, useNewItemRequest } from "./newItemOpen"
import { useShell } from "./ShellContext"

const NewSessionForm = lazy(() =>
  import("../agents/NewSessionForm").then(({ NewSessionForm }) => ({ default: NewSessionForm })),
)

/** Renders the New design / New chat form for the open request; navigates only once the item exists. */
export function NewItemHost() {
  const request = useNewItemRequest()
  return (
    <Portal>
      <Suspense fallback={null}>
        {request?.kind === "design" && <DesignDialog />}
        {request?.kind === "agent" && <AgentDialog />}
      </Suspense>
    </Portal>
  )
}

/** Until the shared new-draft action lands: creates a draft and opens its page. */
function DesignDialog() {
  const navigate = useNavigate()
  const shell = useShell()
  const workspace = shell?.workspace
  useEffect(() => {
    let live = true
    createDraft(workspace)
      .then((id) => {
        if (!live) return
        navigate(draftHref(id).slice(1))
        shell?.closeSliding()
      })
      .catch(() => {})
      .finally(() => live && closeNewItem())
    return () => {
      live = false
    }
    // Once per request.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  return null
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
