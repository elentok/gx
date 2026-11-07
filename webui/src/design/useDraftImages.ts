import { useState, useSyncExternalStore } from "react";
import { draftImageUrl, type RemoveDraftImage, type UploadDraftImage } from "./draftImageApi";
import { checkImages } from "./imageLimits";
import type { ComposerImage } from "./useComposerImages";

interface Entry {
  key: string;
  /** The saved image; the server's list is the truth for these. */
  id?: string;
  /** An upload this tab runs or can retry. */
  file?: File;
  failed?: string;
  /** A file refused before any upload: its reason, remove only. */
  refused?: string;
}

export interface DraftImages {
  images: ComposerImage[];
  readyIds: string[];
  uploading: number;
  /** Every failed tile, refused ones included. */
  failed: number;
  /** The last limit refusal, "" when none. */
  error: string;
  /** Every tile but the refused ones: what the 10-image limit counts. */
  count: number;
  addFiles: (files: File[]) => Promise<void>;
  retry: (key: string) => void;
  remove: (key: string) => void;
  /** Replaces the list with the server's (a loaded or new draft). */
  reset: (ids: string[]) => void;
  urlOf: (id: string) => string;
}

/**
 * One draft's tiles, outside the form so an upload outlives it: switching drafts remounts the form, and the upload
 * finishes into the slot of the draft it began on.
 */
export class ImageSlot {
  entries: Entry[];
  private subs = new Set<() => void>();
  constructor(ids: string[] = []) {
    this.entries = ids.map((id) => ({ key: id, id }));
  }
  set(entries: Entry[]) {
    this.entries = entries;
    this.subs.forEach((f) => f());
  }
  subscribe = (f: () => void) => {
    this.subs.add(f);
    return () => void this.subs.delete(f);
  };
  /** True while a tile is still an upload or a failure: the slot must outlive its form. */
  get pending() {
    return this.entries.some((e) => !e.id);
  }
  /** The server's list replaces the saved tiles; uploads and failures stay. */
  syncSaved(ids: string[]) {
    this.set([...ids.map((id) => ({ key: id, id })), ...this.entries.filter((e) => !e.id)]);
  }
}

/**
 * Image state of a New Design draft: the server's ids plus the uploads this tab runs. `ensureDraft` resolves to the
 * draft's id, saving the text first when the draft has none yet; `draftId` reads the current id for urls and removes.
 */
export function useDraftImages({
  slot,
  draftId,
  ensureDraft,
  upload: send,
  remove: erase,
}: {
  slot: ImageSlot;
  draftId: () => string | undefined;
  ensureDraft: () => Promise<string>;
  upload: UploadDraftImage;
  remove: RemoveDraftImage;
}): DraftImages {
  const entries = useSyncExternalStore(slot.subscribe, () => slot.entries);
  const change = (fn: (cur: Entry[]) => Entry[]) => slot.set(fn(slot.entries));
  const [error, setError] = useState("");
  const patch = (key: string, p: Partial<Entry>) => change((cur) => cur.map((e) => (e.key === key ? { ...e, ...p } : e)));
  const counted = () => slot.entries.filter((e) => !e.refused).length;

  const upload = (key: string, file: File) => {
    patch(key, { file, failed: undefined });
    ensureDraft()
      .then(async (draft) => ({ draft, id: await send(draft, file) }))
      .then(
        ({ draft, id }) => {
          // Removed while uploading: the file just saved has no owner left.
          if (!slot.entries.some((e) => e.key === key)) return void erase(draft, id).catch(() => undefined);
          change((cur) => cur.map((e) => (e.key === key ? { key, id } : e)));
        },
        (err) => {
          if (slot.entries.some((e) => e.key === key)) patch(key, { failed: err instanceof Error ? err.message : "Upload failed" });
        },
      );
  };

  const addFiles = async (files: File[]) => {
    const { accepted, error: err, refused } = await checkImages(files, counted());
    setError(err);
    change((cur) => [...cur, ...refused.map((r) => ({ key: crypto.randomUUID(), refused: r.message }))]);
    for (const file of accepted) {
      const key = crypto.randomUUID();
      change((cur) => [...cur, { key }]);
      upload(key, file);
    }
  };

  const retry = (key: string) => {
    const e = slot.entries.find((x) => x.key === key);
    if (e?.failed && e.file) upload(key, e.file);
  };

  const remove = (key: string) => {
    const e = slot.entries.find((x) => x.key === key);
    change((cur) => cur.filter((x) => x.key !== key));
    const draft = draftId();
    if (e?.id && draft) void erase(draft, e.id).catch(() => undefined);
  };

  const reset = (ids: string[]) => {
    setError("");
    change(() => ids.map((id) => ({ key: id, id })));
  };

  const urlOf = (id: string) => draftImageUrl(draftId() ?? "", id);
  const images = entries.map((e): ComposerImage => {
    if (e.id) return { key: e.key, state: "ready", url: urlOf(e.id), canRetry: false };
    if (e.refused) return { key: e.key, state: "failed", message: e.refused, canRetry: false };
    if (e.failed) return { key: e.key, state: "failed", message: e.failed, canRetry: true };
    return { key: e.key, state: "uploading", canRetry: false };
  });
  return {
    images,
    readyIds: entries.flatMap((e) => (e.id ? [e.id] : [])),
    uploading: images.filter((i) => i.state === "uploading").length,
    failed: images.filter((i) => i.state === "failed").length,
    error,
    count: entries.filter((e) => !e.refused).length,
    addFiles,
    retry,
    remove,
    reset,
    urlOf,
  };
}
