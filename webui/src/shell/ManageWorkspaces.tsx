import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import { apiFetch } from "../apiFetch";
import type {
  Workspace,
  WorkspaceRequest,
  WorkspaceResult,
} from "../generated/server";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { refusalText, useWrite, WriteError } from "../useWrite";

export interface WorkspaceWriters {
  rename: (name: string, to: string) => Promise<WorkspaceResult>;
  remove: (name: string) => Promise<WorkspaceResult>;
  reorder: (order: string[]) => Promise<WorkspaceResult>;
}

async function post(
  route: string,
  body: WorkspaceRequest,
): Promise<WorkspaceResult> {
  const res = await apiFetch(`/v1/workspaces/${route}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok)
    throw new WriteError(
      (await res.text().catch(() => "")).trim() || `HTTP ${res.status}`,
    );
  return (await res.json()) as WorkspaceResult;
}

export const postWorkspaceWriters: WorkspaceWriters = {
  rename: (name, to) => post("rename", { name, to }),
  remove: (name) => post("delete", { name }),
  reorder: (order) => post("reorder", { name: "", order }),
};

const problemText = (
  refusal: Parameters<typeof refusalText>[0] | undefined,
  error: string | undefined,
) => (refusal ? refusalText(refusal) : error);

function ArrowButton({
  label,
  up,
  disabled,
  onClick,
}: {
  label: string;
  up?: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <chakra.button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      display="grid"
      placeItems="center"
      w="28px"
      h="28px"
      color="text.dim"
      cursor="pointer"
      borderRadius="6px"
      _hover={{ bg: "surface.raised" }}
      _disabled={{
        opacity: 0.3,
        cursor: "default",
        _hover: { bg: "transparent" },
      }}
    >
      <svg
        width="12"
        height="12"
        viewBox="0 0 12 12"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        aria-hidden
      >
        <path d={up ? "m3 7.5 3-3 3 3" : "m3 4.5 3 3 3-3"} />
      </svg>
    </chakra.button>
  );
}

function Row({
  workspace,
  first,
  last,
  busy,
  writers,
  onMove,
  onDelete,
}: {
  workspace: Workspace;
  first: boolean;
  last: boolean;
  busy: boolean;
  writers: WorkspaceWriters;
  onMove: (delta: -1 | 1) => void;
  onDelete: () => void;
}) {
  const { name } = workspace;
  const [draft, setDraft] = useState(name);
  const { run, refusal, error } = useWrite({ toastRefusal: false });
  const problem = problemText(refusal, error);
  const save = () => {
    const to = draft.trim();
    if (!to || to === name) return setDraft(name);
    void run(() => writers.rename(name, to));
  };
  return (
    <Box as="li" display="flex" flexDirection="column" gap="1">
      <Box display="flex" alignItems="center" gap="1">
        {workspace.default ? (
          <Text flex="1" px="3" color="text" fontSize="16px">
            {name}
            <Text as="span" color="text.faint" ml="2" fontSize="sm">
              default
            </Text>
          </Text>
        ) : (
          <chakra.input
            aria-label={`Rename ${name}`}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={save}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
              // A pending edit is cancelled first; Escape on an untouched field still closes the dialog.
              if (e.key === "Escape" && draft !== name) {
                e.stopPropagation();
                setDraft(name);
              }
            }}
            flex="1"
            h="32px"
            px="3"
            fontSize="16px"
            bg="surface"
            color="text"
            borderWidth="1px"
            borderColor="border"
            borderRadius="8px"
          />
        )}
        <Text color="text.dim" fontSize="sm" minW="16" textAlign="right">
          {workspace.members} {workspace.members === 1 ? "item" : "items"}
        </Text>
        <ArrowButton
          label={`Move ${name} up`}
          up
          disabled={first || busy}
          onClick={() => onMove(-1)}
        />
        <ArrowButton
          label={`Move ${name} down`}
          disabled={last || busy}
          onClick={() => onMove(1)}
        />
        {!workspace.default && (
          <Button
            emphasis="secondary"
            tone="paused"
            aria-label={`Delete ${name}`}
            onClick={onDelete}
          >
            Delete
          </Button>
        )}
      </Box>
      {problem && (
        <Text role="alert" color="tone.waiting.fg" fontSize="sm" px="3">
          {problem}
        </Text>
      )}
    </Box>
  );
}

/** Rename, reorder and delete workspaces. The list is the snapshot's, so every accepted write shows once the server's event arrives. */
export function ManageWorkspaces({
  workspaces,
  writers = postWorkspaceWriters,
  onClose,
}: {
  workspaces: Workspace[];
  writers?: WorkspaceWriters;
  onClose: () => void;
}) {
  const [deleting, setDeleting] = useState<string>();
  const { run, pending, refusal, error } = useWrite({ toastRefusal: false });
  const target = workspaces.find((w) => w.name === deleting);
  const move = (index: number, delta: -1 | 1) => {
    const order = workspaces.map((w) => w.name);
    [order[index], order[index + delta]] = [
      order[index + delta]!,
      order[index]!,
    ];
    void run(() => writers.reorder(order));
  };
  const problem = problemText(refusal, error);
  return (
    <>
      <Modal
        label="Manage workspaces"
        onClose={onClose}
        bg="surface.raised"
        borderRadius="lg"
        p="4"
        w="520px"
        maxW="90vw"
      >
        <Box display="flex" flexDirection="column" gap="3">
          <Text fontWeight="600">Manage workspaces</Text>
          <Box
            as="ul"
            listStyleType="none"
            m="0"
            p="0"
            display="flex"
            flexDirection="column"
            gap="2"
          >
            {workspaces.map((w, i) => (
              <Row
                key={w.name}
                workspace={w}
                first={i === 0}
                last={i === workspaces.length - 1}
                busy={pending}
                writers={writers}
                onMove={(delta) => move(i, delta)}
                onDelete={() => setDeleting(w.name)}
              />
            ))}
          </Box>
          {problem && (
            <Text role="alert" color="tone.waiting.fg" fontSize="sm">
              {problem}
            </Text>
          )}
          <Box display="flex" justifyContent="flex-end">
            <Button emphasis="secondary" onClick={onClose}>
              Close
            </Button>
          </Box>
        </Box>
      </Modal>
      {target && (
        <Modal
          label={`Delete ${target.name}`}
          onClose={() => setDeleting(undefined)}
          maxW="sm"
        >
          <Text>
            {target.members === 0
              ? `Delete ${target.name}? It has no items.`
              : `Delete ${target.name}? Its ${target.members} ${target.members === 1 ? "item goes" : "items go"} back to the Default workspace.`}
          </Text>
          <Box display="flex" gap="2" justifyContent="flex-end">
            <Button
              emphasis="secondary"
              onClick={() => setDeleting(undefined)}
              autoFocus
            >
              Keep
            </Button>
            <Button
              tone="paused"
              disabled={pending}
              onClick={() => {
                setDeleting(undefined);
                void run(() => writers.remove(target.name));
              }}
            >
              Delete
            </Button>
          </Box>
        </Modal>
      )}
    </>
  );
}
