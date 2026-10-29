import { useConnectionContext, useConnectionState } from "../connection";
import { actionableCount } from "./rules";

// One source for the top bar badge and the tab title, so the two cannot disagree.
export function useActionableCount(): number {
  const { snapshot } = useConnectionState(useConnectionContext());
  return actionableCount(snapshot?.sessions ?? []);
}
