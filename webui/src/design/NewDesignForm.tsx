import { Box, chakra, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { BottomSheet } from "../ui/modal/BottomSheet";
import type { DesignDraft } from "../generated/server";
import { deleteDraft, draftLabel, listDrafts, readDraft, saveDraft, type DeleteDraft, type ListDrafts, type ReadDraft, type SaveDraft } from "./drafts";
import { postOpenDesign, type OpenDesign } from "./open";
import { useDesignWrite } from "./SessionControls";
import { useWide } from "../useWide";

/** The draft calls the form makes; the prop defaults to the real ones. */
export interface DraftsClient {
  list: ListDrafts;
  read: ReadDraft;
  save: SaveDraft;
  remove: DeleteDraft;
}

const realDrafts: DraftsClient = { list: listDrafts, read: readDraft, save: saveDraft, remove: deleteDraft };

const AUTOSAVE_MS = 800;

interface Fields {
  slug: string;
  idea: string;
  project: string;
}

type SaveStatus = "none" | "saving" | "saved";

const hasText = (d: Partial<Fields>) => !!(d.slug?.trim() || d.idea?.trim());

/** A draft with text keeps its project, then the page's filter, then the draft's, then the first. */
function startingProject(draft: Partial<Fields>, projects: string[], filterProject?: string): string {
  const known = (p?: string) => (p && projects.includes(p) ? p : undefined);
  const text = hasText(draft);
  return (text && known(draft.project)) || known(filterProject) || known(draft.project) || projects[0] || "";
}

const focusRing = { borderColor: "tone.running.fg", boxShadow: "0 0 0 3px {colors.tone.running.bg}" };
const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface",
  borderWidth: "1px",
  borderColor: "transparent",
  borderRadius: "8px",
  _placeholder: { color: "text.dim" },
  _focusVisible: { outline: "none", ...focusRing },
} as const;

const sheetProps = {
  label: "New Design",
  fillViewport: true,
  desktop: { borderRadius: "lg", w: "720px", h: "520px" },
  bg: "surface.raised",
  p: "0",
  gap: "0",
  overflow: "hidden",
} as const;

interface FormProps {
  projects: string[];
  /** The Epics page's project filter, when it is on one project. */
  filterProject?: string;
  open?: OpenDesign;
  /** Runs with the new Design's address once its map exists; `startFailure` is why the session did not start. */
  onStarted: (address: string, startFailure?: string) => void;
  /** Closing keeps the draft. */
  onClose: () => void;
  drafts?: DraftsClient;
}

/** The New Design writing sheet: epic name and rough idea for a Design with no map yet. It opens on the latest server draft and autosaves it until Start or Discard. */
export function NewDesignForm({ drafts = realDrafts, ...props }: FormProps) {
  // `undefined` is still loading; `null` is no draft to open.
  const [initial, setInitial] = useState<DesignDraft | null | undefined>();
  useEffect(() => {
    let live = true;
    (async () => {
      const [latest] = await drafts.list();
      const res = latest ? await drafts.read(latest.id) : undefined;
      if (live) setInitial(res?.kind === "ok" ? res.draft : null);
    })().catch(() => live && setInitial(null));
    return () => {
      live = false;
    };
    // The client is fixed for the form's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (initial === undefined) {
    return (
      <BottomSheet {...sheetProps} onClose={props.onClose}>
        <Box flex="1" display="flex" alignItems="center" justifyContent="center">
          <Spinner />
        </Box>
      </BottomSheet>
    );
  }
  return <DraftForm {...props} drafts={drafts} initial={initial} />;
}

function DraftForm({
  projects,
  filterProject,
  open = postOpenDesign,
  onStarted,
  onClose,
  drafts,
  initial,
}: FormProps & { drafts: DraftsClient; initial: DesignDraft | null }) {
  const sheetMode = !useWide();
  const [draft, setDraft] = useState<Fields>(() => ({
    slug: initial?.slug ?? "",
    idea: initial?.idea ?? "",
    project: startingProject(initial ?? {}, projects, filterProject),
  }));
  const [status, setStatus] = useState<SaveStatus>("none");
  const [confirming, setConfirming] = useState(false);
  const { run, pending } = useDesignWrite();

  const latest = useRef(draft);
  // `synced` is the fields as the server last had them, so an unchanged form saves nothing.
  const server = useRef({ id: initial?.id, revision: initial?.revision ?? 0, name: initial?.name, synced: JSON.stringify(draft) });
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const chain = useRef<Promise<void>>(Promise.resolve());
  const finished = useRef(false);

  const persist = async () => {
    const s = server.current;
    const fields = latest.current;
    const key = JSON.stringify(fields);
    for (let attempt = 0; attempt < 2; attempt++) {
      if (key === s.synced) return setStatus(s.id ? "saved" : "none");
      if (!s.id && !fields.slug && !fields.idea) return setStatus("none");
      const res = await drafts.save({ id: s.id, ...fields, name: s.name, revision: s.revision });
      if (res.kind === "ok") {
        Object.assign(s, { id: res.draft.id, revision: res.draft.revision, synced: key });
        return setStatus("saved");
      }
      if (res.kind === "deleted") {
        Object.assign(s, { id: undefined, revision: 0, name: undefined, synced: key });
        return setStatus("none");
      }
      if (res.kind === "gone") {
        Object.assign(s, { id: undefined, revision: 0, name: undefined });
      } else {
        // Another tab saved first; this form's text wins on the retry.
        const current = await drafts.read(s.id!);
        if (current.kind !== "ok") return;
        s.revision = current.draft.revision;
      }
    }
  };

  const flush = () => {
    clearTimeout(timer.current);
    if (finished.current) return;
    chain.current = chain.current.then(persist).catch(() => setStatus("none"));
  };
  const stopSaving = () => {
    finished.current = true;
    clearTimeout(timer.current);
  };

  useEffect(() => {
    const leave = () => flush();
    window.addEventListener("pagehide", leave);
    return () => {
      window.removeEventListener("pagehide", leave);
      leave();
    };
    // flush reads refs only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const patch = (p: Partial<Fields>) => {
    const next = { ...draft, ...p };
    setDraft(next);
    latest.current = next;
    setStatus(next.slug || next.idea || server.current.id ? "saving" : "none");
    clearTimeout(timer.current);
    timer.current = setTimeout(flush, AUTOSAVE_MS);
  };
  const close = () => {
    flush();
    onClose();
  };
  const discard = () => {
    stopSaving();
    // Waits for an in-flight save, which is what gives a new draft its id.
    chain.current = chain.current
      .then(async () => {
        if (server.current.id) await drafts.remove(server.current.id);
      })
      .catch(() => {});
    onClose();
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const slug = draft.slug.trim();
    run(
      () => open({ epic: slug, new_design: true, idea: draft.idea, project: draft.project }),
      {
        onDone: (res) => {
          stopSaving();
          // With start_error the map exists though no session started; the Design page offers the retry.
          onStarted(`${draft.project}:${slug}`, res.start_error);
        },
      },
    );
  };

  const label = draft.slug || draft.idea || server.current.name ? draftLabel({ name: server.current.name, ...draft }) : draftLabel();
  const reason = !draft.slug.trim() ? "Name the epic to start." : !draft.idea.trim() ? "Write an idea to start." : undefined;

  return (
    <BottomSheet {...sheetProps} onClose={close} as="form" onSubmit={submit}>
      <Box display="flex" alignItems="center" gap="2" px="3" pt="2" flexShrink="0">
        <Text fontWeight="600" minW="0" truncate>
          {label}
        </Text>
        <Box flex="1" />
        {status !== "none" && (
          <Text fontSize="xs" color="text.dim" flexShrink="0" whiteSpace="nowrap">
            {status === "saved" && (
              <chakra.span aria-hidden color="tone.done.fg">
                ✓{" "}
              </chakra.span>
            )}
            {status === "saving" ? "Saving..." : "Saved"}
          </Text>
        )}
      </Box>
      <Box display="flex" alignItems="center" gap="2" p="3" borderBottomWidth="1px" borderColor="border" flexShrink="0">
        {projects.length > 1 && (
          <>
            <chakra.select
              aria-label="Project"
              value={draft.project}
              onChange={(e) => patch({ project: e.target.value })}
              fontSize="16px"
              h="40px"
              px="3"
              borderRadius="8px"
              bg="tone.running.bg"
              color="tone.running.fg"
              fontWeight="600"
            >
              {projects.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </chakra.select>
            <Text color="text.dim" fontSize="lg">
              /
            </Text>
          </>
        )}
        <chakra.input
          aria-label="Epic slug"
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          spellCheck={false}
          autoFocus
          {...field}
          flex="1"
          minW="0"
          h="40px"
          px="3"
          fontWeight="600"
          placeholder="my-epic"
          value={draft.slug}
          onChange={(e) => patch({ slug: e.target.value })}
        />
        <chakra.button
          type="button"
          aria-label="Close"
          title="Close (keeps the draft)"
          onClick={close}
          color="text.dim"
          fontSize="xl"
          lineHeight="1"
          w="36px"
          h="36px"
          flexShrink="0"
          borderRadius="8px"
          _hover={{ bg: "surface", color: "text" }}
        >
          ×
        </chakra.button>
      </Box>
      <chakra.textarea
        aria-label="Rough idea"
        {...field}
        flex="1"
        minH="0"
        m="3"
        resize="none"
        px="4"
        py="3"
        lineHeight="1.6"
        placeholder="What is the problem? Who hits it? What does done look like?"
        value={draft.idea}
        onChange={(e) => patch({ idea: e.target.value })}
      />
      {sheetMode && reason && !confirming && (
        <Text fontSize="sm" color="tone.paused.fg" px="3" pb="2" flexShrink="0">
          {reason}
        </Text>
      )}
      <Box display="flex" alignItems="center" gap="2" p="3" borderTopWidth="1px" borderColor="border" bg="surface" flexShrink="0">
        {confirming ? (
          <>
            <Text flex="1" fontSize="sm" color="tone.paused.fg" fontWeight="600">
              Discard this draft?
            </Text>
            <Button type="button" emphasis="ghost" tone="muted" onClick={() => setConfirming(false)}>
              Keep
            </Button>
            <Button type="button" tone="paused" onClick={discard}>
              Discard
            </Button>
          </>
        ) : (
          <>
            <Button type="button" emphasis="ghost" tone="muted" onClick={() => (hasText(draft) ? setConfirming(true) : discard())}>
              Discard
            </Button>
            <Box flex="1" />
            {!sheetMode && reason && (
              <Text fontSize="sm" color="tone.paused.fg">
                {reason}
              </Text>
            )}
            <Button type="submit" tone="running" disabled={pending || !!reason}>
              {pending && <Spinner size="sm" />}
              Start design
            </Button>
          </>
        )}
      </Box>
    </BottomSheet>
  );
}
