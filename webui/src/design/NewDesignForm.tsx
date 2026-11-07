import { Box, chakra, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { BottomSheet } from "../ui/modal/BottomSheet";
import type { DesignDraft } from "../generated/server";
import { deleteDraft, draftLabel, listDrafts, readDraft, saveDraft, type DeleteDraft, type ListDrafts, type ReadDraft, type SaveDraft } from "./drafts";
import { postOpenDesign, type OpenDesign } from "./open";
import { clearUnsaved, NEW_DRAFT_KEY, readUnsaved, stashUnsaved } from "./unsaved";
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
const RETRY_MS = 5000;

interface Fields {
  slug: string;
  idea: string;
  project: string;
}

type SaveStatus = "none" | "saving" | "saved" | "unsaved";
type Banner = "conflict" | "gone" | undefined;

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
  const [restored, setRestored] = useState<Restored>();
  useEffect(() => {
    let live = true;
    (async () => {
      const stuck = await saveUnsaved(drafts);
      if (stuck) {
        if (live) {
          setRestored(stuck.state);
          setInitial(stuck.draft);
        }
        return;
      }
      await moveLegacyDraft(drafts);
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
  return <DraftForm {...props} drafts={drafts} initial={initial} restored={restored} />;
}

const LEGACY_DRAFT_KEY = "gx.newDesign.draft";

/** Moves the pre-server localStorage draft to a server draft once. A refusal or failure keeps the key for the next open. */
async function moveLegacyDraft(drafts: DraftsClient) {
  let legacy: Partial<Fields> = {};
  try {
    const raw = localStorage.getItem(LEGACY_DRAFT_KEY);
    if (raw === null) return;
    try {
      legacy = JSON.parse(raw) ?? {};
    } catch {
      // An unreadable key has no text.
    }
    if (hasText(legacy)) {
      const res = await drafts.save({ project: legacy.project ?? "", slug: legacy.slug ?? "", idea: legacy.idea ?? "", revision: 0 });
      if (res.kind !== "ok") return;
    }
    localStorage.removeItem(LEGACY_DRAFT_KEY);
  } catch {
    // Storage or the server is down; the key stays and the move retries.
  }
}

/** Why the form opened on a fallback entry instead of the latest draft. */
type Restored = "unsaved" | "conflict" | "gone";

/**
 * Saves each fallback entry to its draft with its revision. The first one that cannot settle
 * (a refusal, or the server still down) is returned for the form to open on; its entry stays
 * until the form saves, starts or discards it.
 */
async function saveUnsaved(drafts: DraftsClient): Promise<{ draft: DesignDraft; state: Restored } | undefined> {
  for (const [id, e] of Object.entries(readUnsaved())) {
    const fields = { project: e.project, slug: e.slug, idea: e.idea, name: e.name };
    let state: Restored;
    try {
      const res = await drafts.save({ id: id || undefined, ...fields, revision: e.revision });
      if (res.kind === "ok" || res.kind === "deleted") {
        clearUnsaved(id);
        continue;
      }
      state = res.kind;
    } catch {
      state = "unsaved";
    }
    return { draft: { id, images: [], edited_at: "", ...fields, revision: e.revision }, state };
  }
}

function DraftForm({
  projects,
  filterProject,
  open = postOpenDesign,
  onStarted,
  onClose,
  drafts,
  initial,
  restored,
}: FormProps & { drafts: DraftsClient; initial: DesignDraft | null; restored?: Restored }) {
  const sheetMode = !useWide();
  const [draft, setDraft] = useState<Fields>(() => ({
    slug: initial?.slug ?? "",
    idea: initial?.idea ?? "",
    project: startingProject(initial ?? {}, projects, filterProject),
  }));
  const [status, setStatus] = useState<SaveStatus>(restored === "unsaved" ? "unsaved" : "none");
  const [confirming, setConfirming] = useState(false);
  const { run, pending } = useDesignWrite();

  const latest = useRef(draft);
  // `synced` is the fields as the server last had them, so an unchanged form saves nothing. A restored entry never reached the server.
  const server = useRef({ id: initial?.id || undefined, revision: initial?.revision ?? 0, name: initial?.name, synced: restored ? "" : JSON.stringify(draft) });
  // Set after a failed save until one succeeds; a leave then keeps the text in localStorage.
  const failing = useRef(restored === "unsaved");
  // Fallback keys this form wrote or inherited, cleared together once the text is settled.
  const stashed = useRef<string[]>(restored ? [initial?.id ?? NEW_DRAFT_KEY] : []);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const chain = useRef<Promise<void>>(Promise.resolve());
  const finished = useRef(false);
  // Set while Start is in flight: a save then could recreate the draft Start is about to delete.
  const starting = useRef(false);
  // The banner's ref twin, so a flush in flight sees it before a render.
  const blocked = useRef<Banner>(restored === "conflict" || restored === "gone" ? restored : undefined);
  const [banner, setBanner] = useState<Banner>(blocked.current);
  const [images, setImages] = useState(initial?.images ?? []);

  const persist = async () => {
    if (finished.current || starting.current) return;
    const s = server.current;
    const fields = latest.current;
    const key = JSON.stringify(fields);
    if (key === s.synced) return setStatus(s.id ? "saved" : "none");
    if (!s.id && !fields.slug && !fields.idea) return setStatus("none");
    let res;
    try {
      res = await drafts.save({ id: s.id, ...fields, name: s.name, revision: s.revision });
    } catch {
      // The text stays in memory and is retried on the next keystroke and every RETRY_MS.
      failing.current = true;
      stash();
      return setStatus("unsaved");
    }
    failing.current = false;
    if (res.kind === "ok") {
      Object.assign(s, { id: res.draft.id, revision: res.draft.revision, synced: key });
      settle();
      return setStatus("saved");
    }
    if (res.kind === "deleted") {
      Object.assign(s, { id: undefined, revision: 0, name: undefined, synced: key });
      settle();
      return setStatus("none");
    }
    raise(res.kind);
  };

  /** Keeps the unsaved text in localStorage for the next open. */
  const stash = () => {
    const s = server.current;
    const key = s.id ?? NEW_DRAFT_KEY;
    stashUnsaved(key, { revision: s.revision, ...latest.current, name: s.name });
    if (!stashed.current.includes(key)) stashed.current.push(key);
  };
  /** The text reached the server, or Start or Discard took it over. */
  const settle = () => {
    clearUnsaved(...stashed.current);
    stashed.current = [];
  };

  /** A refusal pauses autosave until the person picks a way out of the banner. */
  const raise = (kind: "conflict" | "gone") => {
    stash();
    blocked.current = kind;
    setBanner(kind);
    setStatus("none");
  };
  /** Reads the server's copy; a draft that is gone raises the gone banner instead. */
  const readServer = async () => {
    const res = await drafts.read(server.current.id!);
    if (res.kind === "ok") return res.draft;
    raise("gone");
  };
  const resume = () => {
    blocked.current = undefined;
    setBanner(undefined);
    flush();
  };
  const keepMine = async () => {
    const theirs = await readServer();
    if (theirs) {
      server.current.revision = theirs.revision;
      resume();
    }
  };
  const loadTheirs = async () => {
    const theirs = await readServer();
    if (!theirs) return;
    const next = { slug: theirs.slug, idea: theirs.idea, project: theirs.project };
    Object.assign(server.current, { revision: theirs.revision, name: theirs.name, synced: JSON.stringify(next) });
    latest.current = next;
    setDraft(next);
    setImages(theirs.images);
    settle();
    resume();
  };
  const keepAsNew = () => {
    Object.assign(server.current, { id: undefined, revision: 0, synced: "" });
    setImages([]);
    resume();
  };

  const flush = () => {
    clearTimeout(timer.current);
    if (finished.current || blocked.current) return;
    chain.current = chain.current.then(persist).catch(() => setStatus("none"));
  };
  const stopSaving = () => {
    finished.current = true;
    clearTimeout(timer.current);
    settle();
  };

  useEffect(() => {
    const leave = () => {
      // The save below may not finish before the page goes, so the fallback is written first.
      if (!finished.current && (failing.current || blocked.current) && JSON.stringify(latest.current) !== server.current.synced) stash();
      flush();
    };
    window.addEventListener("pagehide", leave);
    return () => {
      window.removeEventListener("pagehide", leave);
      leave();
    };
    // flush reads refs only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (status !== "unsaved") return;
    const id = setInterval(flush, RETRY_MS);
    return () => clearInterval(id);
    // flush reads refs only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status]);

  const patch = (p: Partial<Fields>) => {
    const next = { ...draft, ...p };
    setDraft(next);
    latest.current = next;
    setStatus(!blocked.current && (next.slug || next.idea || server.current.id) ? "saving" : "none");
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
      async () => {
        starting.current = true;
        clearTimeout(timer.current);
        try {
          // The draft id must be the one a save in flight is creating.
          await chain.current;
          const res = await open({ epic: slug, new_design: true, idea: draft.idea, project: draft.project, draft_id: server.current.id });
          if (!res.refused) {
            finished.current = true;
            settle();
          }
          return res;
        } finally {
          starting.current = false;
          // A failed Start keeps the draft, so the text typed meanwhile still saves.
          flush();
        }
      },
      {
        onDone: (res) => {
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
            {status === "saving" ? "Saving..." : status === "unsaved" ? "Not saved" : "Saved"}
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
      {banner && (
        <Box role="alert" display="flex" alignItems="center" flexWrap="wrap" gap="2" mx="3" mt="3" px="3" py="10px" borderRadius="8px" bg="tone.waiting.bg" color="tone.waiting.fg" fontSize="13px" flexShrink="0">
          <Text flex="1" minW="0">
            {banner === "conflict" ? "This draft changed on another device." : "This draft was deleted or started on another device."}
          </Text>
          {(banner === "conflict" ? [["Load theirs", loadTheirs] as const, ["Keep mine", keepMine] as const] : [["Keep as new draft", keepAsNew] as const]).map(([text, onClick]) => (
            <chakra.button key={text} type="button" onClick={onClick} h="30px" px="10px" borderRadius="6px" borderWidth="1px" borderColor="currentColor" cursor="pointer">
              {text}
            </chakra.button>
          ))}
        </Box>
      )}
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
              Discard this draft and its images?
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
            <Button type="button" emphasis="ghost" tone="muted" onClick={() => (hasText(draft) || images.length ? setConfirming(true) : discard())}>
              Discard
            </Button>
            <Box flex="1" />
            {!sheetMode && reason && (
              <Text fontSize="sm" color="tone.paused.fg">
                {reason}
              </Text>
            )}
            <Button type="submit" tone="running" disabled={pending || !!reason || !!banner}>
              {pending && <Spinner size="sm" />}
              Start design
            </Button>
          </>
        )}
      </Box>
    </BottomSheet>
  );
}
