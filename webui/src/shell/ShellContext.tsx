import { createContext, useContext } from "react";

export interface Shell {
  sidebarOpen: boolean;
  toggle: () => void;
  close: () => void;
  /** Closes the sidebar when it slides over the page (phone, tablet); a docked sidebar stays. */
  closeSliding: () => void;
  /** A page header calls this on mount so the shell skips its own toggle row. */
  claimHeader: () => () => void;
  /** The Active workspace's name; empty until it resolves. New Designs and chats are created in it. */
  workspace?: string;
}

/** Undefined outside the shell (gallery, isolated tests): no toggle renders there. */
export const ShellContext = createContext<Shell | undefined>(undefined);

export const useShell = () => useContext(ShellContext);
