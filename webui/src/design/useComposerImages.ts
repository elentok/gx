import { useCallback, useRef, useState } from "react";
import { useImageDraft } from "../drafts";
import { checkImages } from "./imageLimits";
import { deleteSessionImage, sessionImageUrl, uploadSessionImage } from "./sessionImages";

export const MSG_CUT_OFF = "Upload cut off";

export interface ComposerImage {
  key: string;
  state: "ready" | "uploading" | "failed";
  /** Ready images only: the saved file, also what a thumbnail restored from the draft shows. */
  url?: string;
  message?: string;
  /** False for a failed upload whose file this tab no longer has: remove only. */
  canRetry: boolean;
}

export interface ComposerImages {
  images: ComposerImage[];
  /** Ready ids in draft order. */
  readyIds: string[];
  uploading: number;
  failed: number;
  /** An upload still runs or failed: Send stays off. */
  blocking: boolean;
  /** Text or a ready image: a ready image makes the turn sendable without text. */
  hasContent: (text: string) => boolean;
  /** The one can-send rule. */
  canSend: (text: string) => boolean;
  /** The `images` field of a prompt post, empty when no image is ready. */
  payload: { images?: string[] };
  /** The last limit refusal, "" when none. */
  error: string;
  addFiles: (files: File[]) => Promise<void>;
  retry: (key: string) => void;
  remove: (key: string) => void;
  /** Empties the draft once a send succeeded. */
  clear: () => void;
}

/** Image draft of a session plus the uploads this tab runs. A pending entry this tab does not run reads as cut off. */
export function useComposerImages(sessionId: string): ComposerImages {
  const [draft, update] = useImageDraft(sessionId);
  // Files and failures of uploads started by this tab only; never persisted.
  const local = useRef(new Map<string, { file: File; failed?: string }>());
  const count = useRef(draft.length);
  count.current = draft.length;
  const [, rerender] = useState(0);
  const touch = () => rerender((n) => n + 1);

  const upload = useCallback(
    (key: string, file: File) => {
      local.current.set(key, { file });
      touch();
      uploadSessionImage(sessionId, file).then(
        (id) => {
          // Removed while uploading: the file just saved has no owner left.
          if (!local.current.has(key)) {
            void deleteSessionImage(sessionId, id).catch(() => undefined);
            return;
          }
          local.current.delete(key);
          update((cur) => cur.map((e) => (e.key === key ? { ...e, id } : e)));
          touch();
        },
        (err) => {
          if (!local.current.has(key)) return;
          local.current.set(key, { file, failed: err instanceof Error ? err.message : "Upload failed" });
          touch();
        },
      );
    },
    [sessionId, update],
  );

  const [error, setError] = useState("");
  // Refused files stay as removable thumbnails with their own reason; they never reach the draft.
  const [refused, setRefused] = useState<{ key: string; message: string }[]>([]);
  const addFiles = useCallback(
    async (files: File[]) => {
      const { accepted, error, refused: bad } = await checkImages(files, count.current);
      setError(error);
      setRefused((cur) => [...cur, ...bad.map((b) => ({ key: crypto.randomUUID(), message: b.message }))]);
      for (const file of accepted) {
        const key = crypto.randomUUID();
        update((cur) => [...cur, { key }]);
        upload(key, file);
      }
    },
    [update, upload],
  );

  const retry = useCallback(
    (key: string) => {
      const entry = local.current.get(key);
      if (entry?.failed) upload(key, entry.file);
    },
    [upload],
  );

  const remove = useCallback(
    (key: string) => {
      const entry = draft.find((e) => e.key === key);
      setRefused((cur) => cur.filter((r) => r.key !== key));
      local.current.delete(key);
      update((cur) => cur.filter((e) => e.key !== key));
      touch();
      if (entry?.id) void deleteSessionImage(sessionId, entry.id).catch(() => undefined);
    },
    [draft, sessionId, update],
  );

  // After a send the server owns the files: drop the draft without deleting them.
  const clear = useCallback(() => {
    local.current.clear();
    setRefused([]);
    update(() => []);
    touch();
  }, [update]);

  const images: ComposerImage[] = draft.map((e) => {
    const mine = local.current.get(e.key);
    if (e.id) return { key: e.key, state: "ready", url: sessionImageUrl(sessionId, e.id), canRetry: false };
    if (mine && !mine.failed) return { key: e.key, state: "uploading", canRetry: false };
    if (mine?.failed) return { key: e.key, state: "failed", message: mine.failed, canRetry: true };
    return { key: e.key, state: "failed", message: MSG_CUT_OFF, canRetry: false };
  });
  // Refused thumbnails do not count as failed uploads: they never block a send.
  const failed = images.filter((i) => i.state === "failed").length;
  images.push(...refused.map((r): ComposerImage => ({ key: r.key, state: "failed", message: r.message, canRetry: false })));
  const readyIds = draft.flatMap((e) => (e.id ? [e.id] : []));
  const uploading = images.filter((i) => i.state === "uploading").length;
  const blocking = uploading > 0 || failed > 0;
  const hasContent = (text: string) => text.trim() !== "" || readyIds.length > 0;
  return {
    images,
    readyIds,
    uploading,
    failed,
    blocking,
    hasContent,
    canSend: (text) => hasContent(text) && !blocking,
    payload: readyIds.length > 0 ? { images: readyIds } : {},
    error,
    addFiles,
    retry,
    remove,
    clear,
  };
}
