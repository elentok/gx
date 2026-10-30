import { apiFetch } from "../apiFetch";
import { WriteError } from "../useWrite";

/** POSTs the body as JSON and returns the parsed answer. A non-200 answer throws a WriteError. */
export async function postJSON<T>(path: string, body: object = {}): Promise<T> {
  const res = await apiFetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  if (!res.ok) throw new WriteError(`Request failed (HTTP ${res.status}).`);
  return res.json();
}
