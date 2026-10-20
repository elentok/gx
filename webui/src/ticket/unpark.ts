export interface UnparkBody {
  address: string;
  state: string;
  /** The chosen blocker of an ambiguous-base park. */
  base?: string;
}

export type UnparkResult =
  | { data: { ticket: string; status: string; message?: string } }
  | { refused: true; reason: string; message: string };

export type PostUnpark = (body: UnparkBody) => Promise<UnparkResult>;

/** What the page shows after a successful unpark. */
export function unparkWords(data: { status: string; message?: string }): string {
  return data.message ?? (data.status === "adopted" ? "adopted as done" : "reopened");
}

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postUnpark: PostUnpark = async (body) => {
  const res = await fetch("/v1/tickets/unpark", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await res.json().catch(() => null);
  if (result && (result.data || result.refused)) return result;
  throw new Error(`unpark: HTTP ${res.status}`);
};
