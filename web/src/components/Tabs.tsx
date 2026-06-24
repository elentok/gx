import { useState } from "preact/hooks"

// The tab bar is the one stateful widget on the page; it hydrates on the client.
const TABS = ["status", "log", "stash", "worktrees"] as const

export function Tabs() {
  const [active, setActive] = useState<(typeof TABS)[number]>("status")
  return (
    <nav class="tabbar" aria-label="gx tabs">
      {TABS.map((tab) => (
        <button
          key={tab}
          class="tab"
          role="tab"
          type="button"
          aria-selected={tab === active}
          onClick={() => setActive(tab)}
        >
          {tab}
        </button>
      ))}
    </nav>
  )
}
