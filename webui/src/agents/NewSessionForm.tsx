import { Box, chakra, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useState, type FormEvent, type KeyboardEvent } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import type { ProjectWorktrees } from "../generated/server";
import { Button } from "../ui/button/Button";
import { BottomSheet } from "../ui/modal/BottomSheet";
import { refusalText, useWrite } from "../useWrite";
import { fetchWorktrees, postCreateSession, type CreateSession, type FetchWorktrees } from "./newSession";

/** `dir` is a picked worktree (empty: the project root); `create` names a new worktree and wins over `dir`. */
interface NewSessionDraft {
  project: string;
  dir: string;
  create: string;
  name: string;
  prompt: string;
}

const DRAFT_KEY = "gx.newSession.draft";

function readDraft(): Partial<NewSessionDraft> {
  try {
    return JSON.parse(localStorage.getItem(DRAFT_KEY) ?? "{}");
  } catch {
    return {};
  }
}

function writeDraft(draft: NewSessionDraft | undefined) {
  try {
    if (draft) localStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
    else localStorage.removeItem(DRAFT_KEY);
  } catch {
    // The draft is a convenience; the open form still holds it.
  }
}

const baseName = (path: string) => path.replace(/\/+$/, "").split("/").pop() ?? path;

const focusRing = { borderColor: "tone.running.fg", boxShadow: "0 0 0 3px {colors.tone.running.bg}" };
const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface",
  borderWidth: "1px",
  borderColor: "transparent",
  borderRadius: "8px",
  px: "3",
  _placeholder: { color: "text.dim" },
  _focusVisible: { outline: "none", ...focusRing },
} as const;

function Field({ label, optional, children }: { label: string; optional?: boolean; children: React.ReactNode }) {
  return (
    <chakra.label display="flex" flexDirection="column" gap="1" position="relative">
      <Text as="span" fontSize="sm" color="text.muted">
        {label}
        {optional && <i> · optional</i>}
      </Text>
      {children}
    </chakra.label>
  );
}

interface Option {
  key: string;
  label: string;
  path: string;
  /** Set on the "+ Create worktree" row. */
  create?: string;
}

/** The New session form: Project, one Worktree picker, Name and First prompt. The draft lives in localStorage until create or discard. */
export function NewSessionForm({
  prefillProject,
  fetchTrees = fetchWorktrees,
  create = postCreateSession,
  onCreated,
  onClose,
}: {
  /** The selected session's project. */
  prefillProject?: string;
  fetchTrees?: FetchWorktrees;
  create?: CreateSession;
  /** Runs with the new session's id once the server accepted the create. */
  onCreated: (id: string) => void;
  /** Closing keeps the draft. */
  onClose: () => void;
}) {
  const [trees, setTrees] = useState<ProjectWorktrees[]>();
  const [loadError, setLoadError] = useState(false);
  const [draft, setDraft] = useState<NewSessionDraft>(() => {
    const saved = readDraft();
    const text = !!(saved.name?.trim() || saved.prompt?.trim() || saved.dir || saved.create);
    return {
      project: (text && saved.project) || prefillProject || saved.project || "",
      dir: saved.dir ?? "",
      create: saved.create ?? "",
      name: saved.name ?? "",
      prompt: saved.prompt ?? "",
    };
  });
  // The text being typed in the Worktree picker; null shows the current choice.
  const [query, setQuery] = useState<string | null>(null);
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

  const q = query?.trim() ?? "";
  const options: Option[] = [];
  if (q && !rows.some((r) => r.key !== "root" && r.label === q)) {
    options.push({ key: "create", label: `+ Create worktree “${q}”`, path: "", create: q });
  }
  options.push(...rows.filter((r) => !q || r.label.toLowerCase().includes(q.toLowerCase())));

  const patch = (p: Partial<NewSessionDraft>) => {
    const next = { ...draft, project, ...p };
    setDraft(next);
    writeDraft(next);
  };
  const pick = (o: Option) => {
    patch(o.create ? { create: o.create, dir: "" } : { create: "", dir: o.key === "root" ? "" : o.path });
    setQuery(null);
  };
  const discard = () => {
    writeDraft(undefined);
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
          name: draft.name.trim() || undefined,
          prompt: draft.prompt.trim() || undefined,
        }),
      {
        onDone: (res) => {
          writeDraft(undefined);
          onCreated(res.session!.id);
        },
      },
    );
  };

  const worktreeKey = (e: KeyboardEvent) => {
    if (e.key === "Enter" && query !== null) {
      e.preventDefault();
      if (options[0]) pick(options[0]);
    } else if (e.key === "Escape" && query !== null) {
      e.stopPropagation();
      setQuery(null);
    }
  };

  const failure = refusal ? refusalText(refusal) : error;
  return (
    <BottomSheet label="New session" onClose={onClose} fillViewport desktop={{ borderRadius: "lg", w: "460px" }} as="form" onSubmit={submit} bg="surface.raised" p="0" gap="0" overflow="hidden">
      <Box display="flex" alignItems="center" p="3" borderBottomWidth="1px" borderColor="border" flexShrink="0">
        <Text as="h3" flex="1" fontFamily="condensed" fontWeight="600" textTransform="uppercase">
          New session
        </Text>
        <chakra.button type="button" aria-label="Close" title="Close (keeps the draft)" onClick={onClose} color="text.dim" fontSize="xl" lineHeight="1" w="36px" h="36px" borderRadius="8px" _hover={{ bg: "surface", color: "text" }}>
          ×
        </chakra.button>
      </Box>
      <Box flex="1" minH="0" overflowY="auto" p="3" display="flex" flexDirection="column" gap="3">
        {!trees ? (
          <Text color="text.muted">{loadError ? "Could not load the projects." : "Loading projects…"}</Text>
        ) : (
          <>
            <Field label="Project">
              <chakra.select aria-label="Project" value={project} onChange={(e) => patch({ project: e.target.value, dir: "", create: "" })} {...field} h="40px">
                {projects.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </chakra.select>
            </Field>
            <Field label="Worktree">
              <chakra.input
                role="combobox"
                aria-label="Worktree"
                aria-expanded={query !== null}
                aria-controls="new-session-worktrees"
                autoComplete="off"
                autoCorrect="off"
                autoCapitalize="off"
                spellCheck={false}
                {...field}
                h="40px"
                value={query ?? (draft.create || picked?.label || "")}
                onFocus={() => setQuery((q) => q ?? "")}
                onBlur={() => setQuery(null)}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={worktreeKey}
              />
              {query !== null && (
                <Box
                  id="new-session-worktrees"
                  role="listbox"
                  aria-label="Worktrees"
                  position="absolute"
                  top="100%"
                  left="0"
                  right="0"
                  zIndex="1"
                  maxH="200px"
                  overflowY="auto"
                  bg="surface.raised"
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="8px"
                >
                  {options.map((o) => (
                    <chakra.div
                      key={o.key}
                      role="option"
                      aria-selected={o.create ? false : o.path === picked?.path && !draft.create}
                      display="flex"
                      gap="3"
                      px="3"
                      py="2"
                      cursor="pointer"
                      color={o.create ? "tone.done.fg" : "text"}
                      bg={o.create ? "surface" : undefined}
                      _hover={{ bg: "surface" }}
                      // Keeps the input focused so its blur does not close the list before the click lands.
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => pick(o)}
                    >
                      <span>{o.label}</span>
                      {o.path && (
                        <Text as="span" ml="auto" fontSize="xs" color="text.dim" truncate>
                          {o.path}
                        </Text>
                      )}
                    </chakra.div>
                  ))}
                </Box>
              )}
            </Field>
            <Field label="Name" optional>
              <chakra.input aria-label="Name" autoComplete="off" spellCheck={false} {...field} h="40px" placeholder={worktreeName} value={draft.name} onChange={(e) => patch({ name: e.target.value })} />
            </Field>
            <Field label="First prompt" optional>
              <chakra.textarea aria-label="First prompt" {...field} py="2" minH="96px" resize="vertical" value={draft.prompt} onChange={(e) => patch({ prompt: e.target.value })} />
            </Field>
          </>
        )}
        {failure && (
          <Text role="alert" fontSize="sm" color="tone.deadlocked.fg">
            {failure}
          </Text>
        )}
      </Box>
      <Box display="flex" alignItems="center" justifyContent="flex-end" gap="2" p="3" borderTopWidth="1px" borderColor="border" bg="surface" flexShrink="0">
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
