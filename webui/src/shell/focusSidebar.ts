// The `s` chord asks the sidebar to take keyboard focus. A window event keeps chords.ts free of shell state.
export const FOCUS_SIDEBAR_EVENT = "gx:focus-sidebar"

export const focusSidebar = () => window.dispatchEvent(new Event(FOCUS_SIDEBAR_EVENT))
