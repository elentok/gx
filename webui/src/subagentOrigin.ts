import { createContext } from "react"

/**
 * The router path of the page subagent links lead back to; on a subagent page it is the one the URL carries.
 * A context, so the components that draw the links need no router.
 */
export const SubagentOriginContext = createContext<string | undefined>(undefined)
