import { clearPageCache } from "./pageCache";

const signedOutListeners = new Set<() => void>();

/** Runs the listener each time the server answers 401. Returns the unsubscribe function. */
export function onSignedOut(listener: () => void): () => void {
  signedOutListeners.add(listener);
  return () => signedOutListeners.delete(listener);
}

/** Drops the cached pages (the snapshot cache included) and tells the auth gate. */
export function signOut(): void {
  clearPageCache();
  signedOutListeners.forEach((l) => l());
}

/** Every web API call goes through here, so a 401 signs the page out in one place. */
export async function apiFetch(...args: Parameters<typeof fetch>): Promise<Response> {
  const res = await fetch(...args);
  if (res.status === 401) signOut();
  return res;
}
