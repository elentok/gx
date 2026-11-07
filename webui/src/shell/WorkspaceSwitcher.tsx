import { Box, chakra, Text } from "@chakra-ui/react";
import { useState, type FormEvent } from "react";
import type { Workspace, WorkspaceResult } from "../generated/server";
import { Button } from "../ui/button/Button";
import { GxMenu } from "../ui/menu/Menu";
import { Modal } from "../ui/modal/Modal";
import { refusalText, useWrite } from "../useWrite";
import { postWorkspace } from "../workspaces/postWorkspace";

export type CreateWorkspace = (name: string) => Promise<WorkspaceResult>;

export const postCreateWorkspace = (name: string) => postWorkspace<WorkspaceResult>("create", { name });

/** Asks for a name and creates the workspace; a refusal (a clash, say) shows under the field and keeps the dialog open. */
export function NewWorkspaceDialog({
  create = postCreateWorkspace,
  onCreated,
  onClose,
}: {
  create?: CreateWorkspace;
  onCreated: (name: string) => void;
  onClose: () => void;
}) {
  const [name, setName] = useState("");
  const { run, pending, refusal, error } = useWrite({ toastRefusal: false });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    void run(() => create(trimmed), { onDone: () => onCreated(trimmed) });
  };
  const problem = refusal ? refusalText(refusal) : error;
  return (
    <Modal label="New workspace" onClose={onClose} bg="surface.raised" borderRadius="lg" p="4" w="360px" maxW="90vw">
      <form onSubmit={submit}>
        <Box display="flex" flexDirection="column" gap="3">
          <Text fontWeight="600">New workspace</Text>
          <chakra.input
            aria-label="Workspace name"
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Name"
            h="36px"
            px="3"
            fontSize="16px"
            bg="surface"
            color="text"
            borderWidth="1px"
            borderColor="border"
            borderRadius="8px"
          />
          {problem && (
            <Text role="alert" color="tone.waiting.fg" fontSize="sm">
              {problem}
            </Text>
          )}
          <Box display="flex" justifyContent="flex-end" gap="2">
            <Button type="button" emphasis="secondary" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={pending || !name.trim()}>
              Create
            </Button>
          </Box>
        </Box>
      </form>
    </Modal>
  );
}

/** The row under the sidebar header: the active workspace, and a menu to switch, create or manage. Nothing here writes to the server except Create. */
export function WorkspaceSwitcher({
  workspaces,
  active,
  onPick,
  onNew,
  onManage,
}: {
  workspaces: Workspace[];
  active: string;
  onPick: (name: string) => void;
  onNew: () => void;
  onManage: () => void;
}) {
  const activeNeeds = workspaces.find((w) => w.name === active)?.needs_you;
  return (
    <GxMenu
      placement="bottom-start"
      items={[
        ...workspaces.map((w) => ({
          value: `ws:${w.name}`,
          label: w.default ? (
            <>
              {w.name}
              <Text as="span" color="text.faint" ml="2">
                default
              </Text>
            </>
          ) : (
            w.name
          ),
          hint: w.needs_you ? (
            <Text as="span" color="tone.waiting.fg">
              {w.needs_you} needs you
            </Text>
          ) : undefined,
          inlineHint: true,
          onSelect: () => onPick(w.name),
        })),
        { kind: "separator" } as const,
        { value: "new", label: "New workspace…", onSelect: onNew },
        { value: "manage", label: "Manage workspaces", onSelect: onManage },
      ]}
      trigger={
        <chakra.button
          type="button"
          aria-label={`Workspace: ${active}`}
          display="flex"
          alignItems="center"
          gap="2"
          w="100%"
          h="40px"
          px="3"
          color="text"
          fontFamily="inherit"
          fontSize="17px"
          fontWeight="600"
          cursor="pointer"
          _hover={{ bg: "surface.raised" }}
        >
          <Box as="span" overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap">
            {active}
          </Box>
          <Box as="span" flex="none" color="text.dim" display="grid">
            <svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden>
              <path d="m3 4.5 3 3 3-3" />
            </svg>
          </Box>
          {activeNeeds ? (
            <Box as="span" ml="auto" flex="none" color="tone.waiting.fg" fontSize="12px" fontWeight="500">
              {activeNeeds} needs you
            </Box>
          ) : null}
        </chakra.button>
      }
    />
  );
}
