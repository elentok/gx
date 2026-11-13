import { Portal } from "@chakra-ui/react"
import { lazy, Suspense } from "react"
import { useNavigate } from "react-router"
import { fetchWorktrees, postCreateSession } from "../agents/newSession"
import { useEpicList } from "../epics/list"
import { designHref } from "../hrefs"
import { closeNewItem, useNewItemRequest } from "./newItemOpen"
import { useShell } from "./ShellContext"

const NewSessionForm = lazy(() =>
  import("../agents/NewSessionForm").then(({ NewSessionForm }) => ({ default: NewSessionForm })),
)
const NewDesignForm = lazy(() =>
  import("../design/NewDesignForm").then(({ NewDesignForm }) => ({ default: NewDesignForm })),
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

function DesignDialog() {
  const navigate = useNavigate()
  const shell = useShell()
  const { list } = useEpicList()
  const projects = [
    ...new Set([...(list?.projects ?? []), ...(list?.epics ?? []).map((e) => e.project)]),
  ].sort()
  return (
    <NewDesignForm
      projects={projects}
      onStarted={(address, startFailure) => {
        closeNewItem()
        navigate(designHref(address).slice(1), { state: { startFailure } })
        shell?.closeSliding()
      }}
      onClose={closeNewItem}
    />
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
