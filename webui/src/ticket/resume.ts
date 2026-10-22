export interface ResumeBody {
  address: string;
  state: string;
  /** The chosen blocker of an ambiguous-base pause. */
  base?: string;
}

export type ResumeResult =
  | { data: { ticket: string; status: string; message?: string } }
  | { refused: true; reason: string; message: string };

export type PostResume = (body: ResumeBody) => Promise<ResumeResult>;

/** What the page shows after a successful resume. */
export function resumeWords(data: { status: string; message?: string }): string {
  return data.message ?? (data.status === "adopted" ? "adopted as done" : "reopened");
}

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postResume: PostResume = async (body) => {
  const res = await fetch("/v1/tickets/resume", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await res.json().catch(() => null);
  if (result && (result.data || result.refused)) return result;
  throw new Error(`resume: HTTP ${res.status}`);
};
