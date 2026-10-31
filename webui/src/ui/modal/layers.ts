import { useCallback, useEffect, useRef } from "react";

// Mounted layers, oldest first. Module-level so a Modal and the editor's jump menu share one order.
const stack: object[] = [];

/** Whether any layer is open right now. Call at event time. */
export const hasLayer = (): boolean => stack.length > 0;

/**
 * Registers the calling layer while it is mounted and `active`. Returns a function to call at
 * event time that says whether this layer is the topmost.
 */
export function useIsTopLayer(active = true): () => boolean {
  const token = useRef({});
  useEffect(() => {
    if (!active) return;
    const mine = token.current;
    stack.push(mine);
    return () => {
      stack.splice(stack.indexOf(mine), 1);
    };
  }, [active]);
  return useCallback(() => stack.at(-1) === token.current, []);
}
