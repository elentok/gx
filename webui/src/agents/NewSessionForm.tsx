import { Box, chakra, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from "react";
import { useCompletion } from "../commands/useCompletion";
import type { CommandTarget, FetchCommands } from "../commands/useCommandList";
import { modelName } from "../modelName";
import { useWide } from "../useWide";
import { useConnectionContext, useConnectionState } from "../connection";
import type { ProjectWorktrees } from "../generated/server";
import { Button } from "../ui/button/Button";
import { BottomSheet } from "../ui/modal/BottomSheet";
import { refusalText, useWrite } from "../useWrite";
import { Field, field } from "./formField";
import { fetchWorktrees, postCreateSession, type CreateSession, type FetchWorktrees } from "./newSession";
import { trimmed, useNewSessionDraft, type NewSessionDraft } from "./useNewSessionDraft";
import { baseName, WorktreePicker, type Option } from "./WorktreePicker";

export const TIERS = [
  { id: "light", label: "Light" },
  { id: "worker", label: "Worker" },
  { id: "strongest", label: "Strongest" },
];

// The picker is Claude-only and a tier reads as the family: "Worker (Claude Sonnet)".
const tierModel = (id: string) => modelName(id, "claude");

/** The First prompt textarea with the `/` menu. The session is always Claude; the list is read for the chosen worktree, or the project root when one is being created. */
function PromptField({ value, onChange, target, fetcher }: { value: string; onChange: (text: string) => void; target: CommandTarget; fetcher?: FetchCommands }) {
  const area = useRef<HTMLTextAreaElement>(null);
  const [cursor, setCursor] = useState(0);
  const pending = useRef<number | null>(null);
  const wide = useWide();
  const c = useCompletion({ target, harness: "claude", fetcher, sends: false, listMaxH: "250px", placement: wide ? "above" : "below" }, value, cursor, false, (text, at) => {
    pending.current = at;
    onChange(text);
    setCursor(at);
  });
  useLayoutEffect(() => {
    if (pending.current == null) return;
    area.current?.setSelectionRange(pending.current, pending.current);
    pending.current = null;
  }, [value]);
  return (
    <Box position="relative">
      <chakra.textarea
        ref={area}
        aria-label="First prompt"
        {...field}
        w="100%"
        py="2"
        minH="96px"
        resize="vertical"
        value={value}
        onChange={(e) => {
          setCursor(e.target.selectionStart);
          onChange(e.target.value);
        }}
        onSelect={(e) => setCursor(e.currentTarget.selectionStart)}
        onKeyDown={c.onKeyDown}
      />
      {c.menu}
    </Box>
  );
}

/** The New session form: Project, one Worktree picker, Name and First prompt. The draft lives in localStorage until create or discard. */
export function NewSessionForm({
  prefillProject,
  fetchTrees = fetchWorktrees,
  create = postCreateSession,
  fetchCommands,
  onCreated,
  onClose,
}: {
  /** The selected session's project. */
  prefillProject?: string;
  fetchTrees?: FetchWorktrees;
  create?: CreateSession;
  /** Replaces the command-list fetch, for tests. */
  fetchCommands?: FetchCommands;
  /** Runs with the new session's id once the server accepted the create. */
  onCreated: (id: string) => void;
  /** Closing keeps the draft. */
  onClose: () => void;
}) {
  const [trees, setTrees] = useState<ProjectWorktrees[]>();
  const [loadError, setLoadError] = useState(false);
  const [tier, setTier] = useState("worker");
  const { draft, update, clear } = useNewSessionDraft(prefillProject);
  const { connected } = useConnectionState(useConnectionContext());
  const { run, pending, refusal, error } = useWrite();

  useEffect(() => {
    let live = true;
    fetchTrees().then(
      (list) => live && setTrees(list),
      () => live && setLoadError(true),
    );
    return () => {
      live = false;
    };
  }, [fetchTrees]);

  const projects = trees?.map((p) => p.project) ?? [];
  const project = projects.includes(draft.project) ? draft.project : (projects[0] ?? "");
  const current = trees?.find((p) => p.project === project);
  const rows: Option[] = current
    ? [
        { key: "root", label: "Project root", path: current.root },
        ...current.worktrees.map((path) => ({ key: path, label: baseName(path), path })),
      ]
    : [];
  const picked = rows.find((r) => r.path === draft.dir) ?? rows[0];
  const worktreeName = draft.create || (picked ? baseName(picked.path) : "");

  const patch = (p: Partial<NewSessionDraft>) => update({ project, ...p });
  const pick = (o: Option) => patch(o.create ? { create: o.create, dir: "" } : { create: "", dir: o.key === "root" ? "" : o.path });
  const discard = () => {
    clear();
    onClose();
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!picked || !connected) return;
    run(
      () =>
        create({
          project,
          ...(draft.create ? { worktree: draft.create } : { dir: picked.path }),
          name: trimmed(draft.name),
          prompt: trimmed(draft.prompt),
          tier,
        }),
      {
        onDone: (res) => {
          clear();
          onCreated(res.session!.id);
        },
      },
    );
  };

  const failure = refusal ? refusalText(refusal) : error;
  return (
    <BottomSheet label="New session" onClose={onClose} fillViewport desktop={{ borderRadius: "lg", w: "460px" }} as="form" onSubmit={submit} bg="surface.raised" p="0" gap="0" overflow="hidden">
      <Box display="flex" alignItems="center" px="5" pt="4.5" pb="1.5" flexShrink="0">
        <Text as="h3" flex="1" fontFamily="condensed" fontSize="24px" fontWeight="600" textTransform="uppercase" letterSpacing="0.02em">
          New session
        </Text>
        <chakra.button type="button" aria-label="Close" title="Close (keeps the draft)" onClick={onClose} color="text.dim" fontSize="xl" lineHeight="1" w="36px" h="36px" borderRadius="8px" _hover={{ bg: "surface", color: "text" }}>
          ×
        </chakra.button>
      </Box>
      <Box flex="1" minH="0" overflowY="auto" px="5" pt="2.5" pb="3" display="flex" flexDirection="column" gap="4.5">
        {!trees ? (
          <Text color="text.muted">{loadError ? "Could not load the projects." : "Loading projects…"}</Text>
        ) : (
          <>
            <Field label="Project" caret>
              <chakra.select aria-label="Project" value={project} onChange={(e) => patch({ project: e.target.value, dir: "", create: "" })} {...field} h="40px" appearance="none" pr="8">
                {projects.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </chakra.select>
            </Field>
            <WorktreePicker rows={rows} picked={picked} creating={draft.create} onPick={pick} />
            <Field label="Name" optional>
              <chakra.input aria-label="Name" autoComplete="off" spellCheck={false} {...field} h="40px" placeholder={worktreeName} value={draft.name} onChange={(e) => patch({ name: e.target.value })} />
            </Field>
            <Field label="Tier" caret>
              <chakra.select aria-label="Tier" value={tier} onChange={(e) => setTier(e.target.value)} {...field} h="40px" appearance="none" pr="8">
                {TIERS.map((t) => (
                  <option key={t.id} value={t.id}>
                    {current?.tiers[t.id] ? `${t.label} (${tierModel(current.tiers[t.id])})` : t.label}
                  </option>
                ))}
              </chakra.select>
            </Field>
            <Field label="First prompt" optional>
              <PromptField fetcher={fetchCommands} value={draft.prompt} onChange={(prompt) => patch({ prompt })} target={{ project, dir: draft.create || !picked ? (current?.root ?? "") : picked.path }} />
            </Field>
          </>
        )}
        {failure && (
          <Text role="alert" fontSize="sm" color="tone.deadlocked.fg">
            {failure}
          </Text>
        )}
      </Box>
      <Box display="flex" alignItems="center" justifyContent="flex-end" gap="2.5" px="5" pt="1" pb="5" flexShrink="0">
        <Button type="button" emphasis="ghost" tone="muted" onClick={discard}>
          Cancel
        </Button>
        <Button type="submit" disabled={pending || !connected || !picked}>
          {pending && <Spinner size="sm" />}
          Create session
        </Button>
      </Box>
    </BottomSheet>
  );
}
