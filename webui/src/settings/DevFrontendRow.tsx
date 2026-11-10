import { useDev } from "../dev";
import { SettingsRow, type SettingsRowProps } from "./SettingsRow";

/** The dev-frontend row: switched off, with the reason, while gx has no Dev checkout. The snapshot's Dev block repaints on dev-changed. */
export function DevFrontendRow(props: SettingsRowProps) {
  const lockReason = useDev().has_checkout ? undefined : "Off: gx has no Dev checkout. Set the Dev checkout path below.";
  return <SettingsRow {...props} lockReason={lockReason} />;
}
