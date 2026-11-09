import { Portal } from "@chakra-ui/react";
import { useLocation, useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { currentAddress } from "./SidebarRows";
import { fetchWorktrees, postCreateSession } from "../agents/newSession";
import { NewSessionForm } from "../agents/NewSessionForm";
import { NewDesignForm } from "../design/NewDesignForm";
import { isCurrent, useEpicList } from "../epics/list";
import { designHref } from "../hrefs";
import { closeNewItem, useNewItemRequest } from "./newItemOpen";
import { useShell } from "./ShellContext";

/** Renders the New design / New chat form for the open request; navigates only once the item exists. */
export function NewItemHost() {
  const request = useNewItemRequest();
  return (
    <Portal>
      {request?.kind === "design" && <DesignDialog filterProject={request.filterProject} />}
      {request?.kind === "agent" && <AgentDialog prefillProject={request.prefillProject} />}
    </Portal>
  );
}

/** The project of the open epic, Design or session page; undefined elsewhere. An opener's own project (the Epics filter) wins over it. */
function useRouteProject(): string | undefined {
  const { pathname } = useLocation();
  const { list } = useEpicList();
  const snapshot = useConnectionState(useConnectionContext()).snapshot;
  const address = currentAddress(pathname);
  if (address) return list?.epics.find((row) => isCurrent(row, address))?.project;
  const [, kind, id] = pathname.split("/");
  if (kind === "agents" && id) return snapshot?.sessions?.find((s) => s.id === decodeURIComponent(id))?.project;
  return undefined;
}

function DesignDialog({ filterProject }: { filterProject?: string }) {
  const navigate = useNavigate();
  const shell = useShell();
  const { list } = useEpicList();
  const routeProject = useRouteProject();
  const projects = [...new Set([...(list?.projects ?? []), ...(list?.epics ?? []).map((e) => e.project)])].sort();
  return (
    <NewDesignForm
      projects={projects}
      filterProject={filterProject ?? routeProject}
      onStarted={(address, startFailure) => {
        closeNewItem();
        navigate(designHref(address).slice(1), { state: { startFailure } });
        shell?.closeSliding();
      }}
      onClose={closeNewItem}
    />
  );
}

function AgentDialog({ prefillProject }: { prefillProject?: string }) {
  const navigate = useNavigate();
  const shell = useShell();
  const routeProject = useRouteProject();
  return (
    <NewSessionForm
      prefillProject={prefillProject ?? routeProject}
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
