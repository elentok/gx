import { useConnectionContext, useConnectionState } from "../connection";
import { Strip } from "../ui/strip/Strip";

/** One plain line while the live snapshot says migrating. Blocks nothing; goes when migration ends. */
export function MigratingStrip() {
  const { snapshot } = useConnectionState(useConnectionContext());
  return snapshot?.migrating ? <Strip tone="waiting">Migrating tickets</Strip> : null;
}
