import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";
import type { DesignDraft } from "../generated/server";
import { useConnectionContext, useConnectionState } from "../connection";
import { defaultProject } from "../workspaces/defaultProject";
import type { DraftsClient } from "./NewDesignForm";
import { hasText, type Fields } from "./draftFields";
import { useDraftImages, type ImageSlot } from "./useDraftImages";
import { clearUnsaved, NEW_DRAFT_KEY, stashUnsaved, type Restored } from "./unsaved";

const MSG_NEEDS_TEXT = "Write the epic name or an idea first";

const AUTOSAVE_MS = 800;
const RETRY_MS = 5000;

export type SaveStatus = "none" | "saving" | "saved" | "unsaved";
export type Banner = "conflict" | "gone" | undefined;

/** `editing` saves; `starting` is Start in flight (a save then could recreate the draft Start is about to delete); `finished` is Start, Discard or a switch having taken the draft over. */
type Phase = "editing" | "starting" | "finished";

/**
 * The open draft's text, its autosave and the images that share its id. The images live here because an
 * upload needs the draft saved first (`ensureDraft`) and a loaded copy replaces both.
 */
export function useDraftAutosave({
  drafts,
  initial,
  restored,
  projects,
  lastProject,
  slot,
}: {
  drafts: DraftsClient;
  initial: DesignDraft | null;
  restored?: Restored;
  projects: string[];
  lastProject: string;
  slot: ImageSlot;
}) {
  const { pathname } = useLocation();
  const sessions = useConnectionState(useConnectionContext()).snapshot?.sessions ?? [];
  // A saved draft keeps its project; one whose project is gone is moved as a new draft would start.
  const keptProject = initial && projects.includes(initial.project) ? initial.project : undefined;
  const [draft, setDraft] = useState<Fields>(() => ({
    slug: initial?.slug ?? "",
    idea: initial?.idea ?? "",
    project: keptProject ?? defaultProject({ pathname, sessions, projects, lastProject }),
  }));
  const [movedFrom] = useState(() => (initial && !keptProject && initial.project ? initial.project : undefined));
  const [moved, setMoved] = useState(movedFrom !== undefined);
  const [status, setStatus] = useState<SaveStatus>(restored === "unsaved" ? "unsaved" : initial?.id && !restored ? "saved" : "none");
  const [name, setName] = useState(initial?.name);
  // A name change is a text save even when the fields are unchanged.
  const nameDirty = useRef(false);

  const latest = useRef(draft);
  // `synced` is the fields as the server last had them, so an unchanged form saves nothing. A restored entry never reached the server.
  // A moved draft starts out of sync, so its first save writes the new project; `unedited` is the fields as moved, which that save stamps without a new edited-at.
  const unedited = useRef(movedFrom !== undefined ? JSON.stringify(draft) : undefined);
  const server = useRef({
    id: initial?.id || undefined,
    revision: initial?.revision ?? 0,
    name: initial?.name,
    synced: restored ? "" : JSON.stringify(movedFrom !== undefined ? { ...draft, project: movedFrom } : draft),
  });
  // Set after a failed save until one succeeds; a leave then keeps the text in localStorage.
  const failing = useRef(restored === "unsaved");
  // Fallback keys this form wrote or inherited, cleared together once the text is settled.
  const stashed = useRef<string[]>(restored ? [initial?.id ?? NEW_DRAFT_KEY] : []);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const chain = useRef<Promise<void>>(Promise.resolve());
  const phase = useRef<Phase>("editing");
  // The banner's ref twin, so a flush in flight sees it before a render.
  const blocked = useRef<Banner>(restored === "conflict" || restored === "gone" ? restored : undefined);
  const [banner, setBanner] = useState<Banner>(blocked.current);

  /** The draft's id for an image upload; a draft that has none yet is saved first, which needs some text. */
  const ensureDraft = async () => {
    await chain.current;
    if (!server.current.id) {
      flush();
      await chain.current;
    }
    if (!server.current.id) throw new Error(MSG_NEEDS_TEXT);
    return server.current.id;
  };
  const imgs = useDraftImages({
    slot,
    draftId: () => server.current.id,
    ensureDraft,
    upload: drafts.uploadImage,
    remove: drafts.removeImage,
  });

  const persist = async () => {
    if (phase.current !== "editing") return;
    const s = server.current;
    const fields = latest.current;
    const key = JSON.stringify(fields);
    if (key === s.synced && !nameDirty.current) return setStatus(s.id ? "saved" : "none");
    if (!s.id && !fields.slug && !fields.idea && !s.name) return setStatus("none");
    let res;
    try {
      res = await drafts.save({ id: s.id, ...fields, name: s.name, revision: s.revision, ...(key === unedited.current && { keep_edited_at: true }) });
    } catch {
      // The text stays in memory and is retried on the next keystroke and every RETRY_MS.
      failing.current = true;
      stash();
      return setStatus("unsaved");
    }
    failing.current = false;
    if (res.kind === "ok") {
      unedited.current = undefined;
      Object.assign(s, { id: res.draft.id, revision: res.draft.revision, synced: key });
      nameDirty.current = false;
      settle();
      return setStatus("saved");
    }
    if (res.kind === "deleted") {
      Object.assign(s, { id: undefined, revision: 0, name: undefined, synced: key });
      nameDirty.current = false;
      setName(undefined);
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
  const bannerActions = {
    keepMine: async () => {
      const theirs = await readServer();
      if (theirs) {
        server.current.revision = theirs.revision;
        resume();
      }
    },
    loadTheirs: async () => {
      const theirs = await readServer();
      if (!theirs) return;
      const next = { slug: theirs.slug, idea: theirs.idea, project: theirs.project };
      Object.assign(server.current, { revision: theirs.revision, name: theirs.name, synced: JSON.stringify(next) });
      latest.current = next;
      nameDirty.current = false;
      setName(theirs.name);
      setDraft(next);
      imgs.reset(theirs.images);
      settle();
      resume();
    },
    keepAsNew: () => {
      Object.assign(server.current, { id: undefined, revision: 0, synced: "" });
      imgs.reset([]);
      resume();
    },
  };

  const flush = () => {
    clearTimeout(timer.current);
    if (phase.current === "finished" || blocked.current) return;
    chain.current = chain.current.then(persist).catch(() => setStatus("none"));
  };
  const stopSaving = () => {
    phase.current = "finished";
    clearTimeout(timer.current);
    settle();
  };

  // Every effect below reads refs only, so `flush` is safe to call from a stale closure.
  useEffect(() => {
    const leave = () => {
      // The save below may not finish before the page goes, so the fallback is written first.
      if (phase.current !== "finished" && (failing.current || blocked.current) && JSON.stringify(latest.current) !== server.current.synced) stash();
      flush();
    };
    window.addEventListener("pagehide", leave);
    return () => {
      window.removeEventListener("pagehide", leave);
      leave();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (movedFrom !== undefined) flush();
    // Once, on open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (status !== "unsaved") return;
    const id = setInterval(flush, RETRY_MS);
    return () => clearInterval(id);
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
  const pickProject = (project: string) => {
    setMoved(false);
    patch({ project });
    drafts.setLastProject(project).catch(() => {});
  };
  const saveName = (text: string) => {
    const next = text.trim() || undefined;
    if (next === server.current.name) return;
    server.current.name = next;
    nameDirty.current = true;
    setName(next);
    setStatus((cur) => (!blocked.current && server.current.id ? "saving" : cur));
    flush();
  };
  /** Discards the draft: stops saving and deletes it once an in-flight save has given a new draft its id. */
  const discard = () => {
    stopSaving();
    chain.current = chain.current
      .then(async () => {
        if (server.current.id) await drafts.remove(server.current.id);
      })
      .catch(() => {});
  };
  /** Runs Start's server call with saves held back; a result that is not `refused` finishes the draft. */
  const start = async <T extends { refused?: unknown }>(call: (draftId: string | undefined) => Promise<T>) => {
    phase.current = "starting";
    clearTimeout(timer.current);
    try {
      // The draft id must be the one a save in flight is creating.
      await chain.current;
      const res = await call(server.current.id);
      if (!res.refused) {
        phase.current = "finished";
        settle();
      }
      return res;
    } finally {
      if (phase.current === "starting") phase.current = "editing";
      // A failed Start keeps the draft, so the text typed meanwhile still saves.
      flush();
    }
  };

  // A name or images keep a draft with no text from being blank.
  const hasContent = () => hasText(latest.current) || !!server.current.name || imgs.count > 0;

  return {
    draft,
    patch,
    pickProject,
    name,
    saveName,
    status,
    banner,
    bannerActions,
    moved,
    movedFrom,
    imgs,
    discard,
    start,
    flush,
    /** For the switching hook: the parts of the autosave machine a draft switch drives. */
    internals: { server, chain, timer, phase, blocked, hasContent, flush, stopSaving },
  };
}

export type DraftAutosave = ReturnType<typeof useDraftAutosave>;
