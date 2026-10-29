import { useEffect } from "react";
import { useConnectionContext, useConnectionState } from "./connection";
import { showToast, toaster } from "./ui/toaster";

const toastId = "node-missing";

// Warns while node is not on the gx server's PATH, since web dependency
// installs then fail. A closed toast comes back only when the page reloads.
export function NodeMissingToast() {
  const missing = useConnectionState(useConnectionContext()).snapshot?.node_missing;
  useEffect(() => {
    if (!missing) {
      toaster.remove(toastId);
      return;
    }
    showToast({
      id: toastId,
      type: "warning",
      title: "node is not on the gx server's PATH",
      description: "Installing web dependencies will fail. Run `gx server install` from a shell that has node.",
      persistent: true,
    });
  }, [missing]);
  return null;
}
