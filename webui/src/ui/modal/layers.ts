import { useEffect, useRef } from "react";

// Mounted layers, oldest first. Module-level so a Modal and the editor's jump menu share one order.
const stack: object[] = [];

/** Registers the calling layer while it is mounted. Returns a function to call at event time that says whether this layer is the topmost. */
export function useIsTopLayer(): () => boolean {
  const token = useRef({});
  useEffect(() => {
    const mine = token.current;
    stack.push(mine);
    return () => {
      stack.splice(stack.indexOf(mine), 1);
    };
  }, []);
  return () => stack[stack.length - 1] === token.current;
}
