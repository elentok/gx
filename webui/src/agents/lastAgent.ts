const LAST_KEY = "gx.agents.last";

// Storage is a convenience; the pages work without it.
export function readLast(): string | null {
  try {
    return localStorage.getItem(LAST_KEY);
  } catch {
    return null;
  }
}

export function writeLast(id: string) {
  try {
    localStorage.setItem(LAST_KEY, id);
  } catch {
    // See above.
  }
}

/** A removed agent must not be restored by the Agents page while the snapshot still lists it. */
export function forgetLast(id: string) {
  try {
    if (localStorage.getItem(LAST_KEY) === id) localStorage.removeItem(LAST_KEY);
  } catch {
    // See above.
  }
}
