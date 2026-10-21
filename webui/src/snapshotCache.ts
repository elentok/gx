import { readPageCache, writePageCache } from "./pageCache";
import type { Snapshot } from "./snapshot";

export interface SnapshotCache {
  load(): Snapshot | null;
  save(snapshot: Snapshot): void;
}

export const sessionSnapshotCache: SnapshotCache = {
  load: () => readPageCache<Snapshot>("snapshot"),
  save: (snapshot) => writePageCache("snapshot", snapshot),
};
