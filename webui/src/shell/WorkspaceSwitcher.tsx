import { Box, chakra, Menu, Portal, Text } from "@chakra-ui/react";
import { useState, type FormEvent } from "react";
import { apiFetch } from "../apiFetch";
import type { Workspace, WorkspaceResult } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { refusalText, useWrite, WriteError } from "../useWrite";

export type CreateWorkspace = (name: string) => Promise<WorkspaceResult>;

export async function postCreateWorkspace(name: string): Promise<WorkspaceResult> {
  const res = await apiFetch("/v1/workspaces/create", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
  return (await res.json()) as WorkspaceResult;
}

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
  return (
    <Menu.Root positioning={{ placement: "bottom-start" }} lazyMount unmountOnExit>
      <Menu.Trigger asChild>
        <chakra.button
          type="button"
          aria-label={`Workspace: ${active}`}
          display="flex"
          alignItems="center"
          gap="2"
          w="100%"
          minH="34px"
          px="3"
          color="text"
          fontWeight="600"
          cursor="pointer"
          _hover={{ bg: "surface.raised" }}
        >
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d="M2.5 4.5 6 8l3.5-3.5" />
          </svg>
          <Box as="span" overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap">
            {active}
          </Box>
        </chakra.button>
      </Menu.Trigger>
      <Portal>
        <Menu.Positioner>
          <Menu.Content bg="surface.raised" borderWidth="1px" borderColor="border" minW="56">
            {workspaces.map((w) => (
              <Menu.Item key={w.name} value={`ws:${w.name}`} onClick={() => onPick(w.name)} cursor="pointer" color="text" justifyContent="space-between" _highlighted={{ bg: "surface" }}>
                <Box as="span" fontWeight={w.name === active ? "600" : "400"}>
                  {w.name}
                </Box>
                {w.needs_you ? (
                  <Text as="span" color="tone.waiting.fg" fontSize="sm" aria-label={`${w.needs_you} need you`}>
                    {w.needs_you}
                  </Text>
                ) : null}
              </Menu.Item>
            ))}
            <Menu.Separator />
            <Menu.Item value="new" onClick={onNew} cursor="pointer" color="text" _highlighted={{ bg: "surface" }}>
              New workspace…
            </Menu.Item>
            <Menu.Item value="manage" onClick={onManage} cursor="pointer" color="text" _highlighted={{ bg: "surface" }}>
              Manage workspaces
            </Menu.Item>
          </Menu.Content>
        </Menu.Positioner>
      </Portal>
    </Menu.Root>
  );
}
