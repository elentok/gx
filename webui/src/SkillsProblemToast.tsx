import { useEffect } from "react";
import { useConnectionContext, useConnectionState } from "./connection";
import { showToast, toaster } from "./ui/toaster";

const toastId = "skills-install-problem";

// Shows the problem the last `gx server start` skill install hit, until the
// person closes it or a clean install clears it. A closed toast comes back
// only when the problem text changes or the page reloads.
export function SkillsProblemToast() {
  const problem = useConnectionState(useConnectionContext()).snapshot?.skills_install_problem;
  useEffect(() => {
    if (!problem) {
      toaster.remove(toastId);
      return;
    }
    showToast({ id: toastId, type: "warning", title: "gx could not install its skills", description: problem, persistent: true });
  }, [problem]);
  return null;
}
