import { useState } from "react";

/** `dir` is a picked worktree (empty: the project root); `create` names a new worktree and wins over `dir`. */
export interface NewSessionDraft {
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

/** Text a person left blank is sent as absent. */
export const trimmed = (s: string) => s.trim() || undefined;

/** The form's draft, kept in localStorage until `clear`. */
export function useNewSessionDraft(prefillProject?: string) {
  const [draft, setDraft] = useState<NewSessionDraft>(() => {
    const saved = readDraft();
    const text = !!(trimmed(saved.name ?? "") || trimmed(saved.prompt ?? "") || saved.dir || saved.create);
    return {
      project: (text && saved.project) || prefillProject || saved.project || "",
      dir: saved.dir ?? "",
      create: saved.create ?? "",
      name: saved.name ?? "",
      prompt: saved.prompt ?? "",
    };
  });
  const update = (p: Partial<NewSessionDraft>) => {
    const next = { ...draft, ...p };
    setDraft(next);
    writeDraft(next);
  };
  return { draft, update, clear: () => writeDraft(undefined) };
}
