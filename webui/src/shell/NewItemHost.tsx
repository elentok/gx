import { Portal } from "@chakra-ui/react";
import { useNavigate } from "react-router";
import { fetchWorktrees, postCreateSession } from "../agents/newSession";
import { NewSessionForm } from "../agents/NewSessionForm";
import { NewDesignForm } from "../design/NewDesignForm";
import { useEpicList } from "../epics/list";
import { designHref } from "../hrefs";
import { closeNewItem, useNewItemRequest } from "./newItemOpen";
import { useShell } from "./ShellContext";

/** Renders the New design / New chat form for the open request; navigates only once the item exists. */
export function NewItemHost() {
  const request = useNewItemRequest();
  return (
    <Portal>
      {request?.kind === "design" && <DesignDialog filterProject={request.filterProject} />}
      {request?.kind === "agent" && <AgentDialog />}
    </Portal>
  );
}

function DesignDialog({ filterProject }: { filterProject?: string }) {
  const navigate = useNavigate();
  const shell = useShell();
  const { list } = useEpicList();
  const projects = [...new Set([...(list?.projects ?? []), ...(list?.epics ?? []).map((e) => e.project)])].sort();
  return (
    <NewDesignForm
      projects={projects}
      filterProject={filterProject}
      onStarted={(address, startFailure) => {
        closeNewItem();
        navigate(designHref(address).slice(1), { state: { startFailure } });
        shell?.closeSliding();
      }}
      onClose={closeNewItem}
    />
  );
}

function AgentDialog() {
  const navigate = useNavigate();
  const shell = useShell();
  return (
    <NewSessionForm
      fetchTrees={fetchWorktrees}
      create={postCreateSession}
      onClose={closeNewItem}
      onCreated={(id) => {
        closeNewItem();
        navigate(`/agents/${id}`);
        shell?.closeSliding();
      }}
    />
  );
}
