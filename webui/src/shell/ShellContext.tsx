import { createContext, useContext } from "react";

export interface Shell {
  sidebarOpen: boolean;
  toggle: () => void;
  close: () => void;
  /** A page header calls this on mount so the shell skips its own toggle row. */
  claimHeader: () => () => void;
}

/** Undefined outside the shell (gallery, isolated tests): no toggle renders there. */
export const ShellContext = createContext<Shell | undefined>(undefined);

export const useShell = () => useContext(ShellContext);
