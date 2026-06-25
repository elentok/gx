import { useState } from "preact/hooks"
import { Icon } from "./components/Icon.tsx"
import { Button, Cmd, Panel } from "./components/primitives.tsx"

/* ── Static data ───────────────────────────────────────────────── */

interface Pillar {
  id: string
  label: string
  gif: string
  gifAlt: string
  captions: string[]
}

const PILLARS: Pillar[] = [
  {
    id: "review",
    label: "review & stage",
    gif: "/demos/demo-status.gif",
    gifAlt: "gx status view — staging changes at the hunk and line level",
    captions: [
      "Stage by file, hunk, or individual line",
      "Side-by-side ↔ unified diff, adjustable context",
      "Inline image diffs (kitty graphics protocol)",
      "ay  Yank for AI   ·   aa  Ask AI",
    ],
  },
  {
    id: "log",
    label: "inspect history",
    gif: "/demos/demo-log.gif",
    gifAlt: "gx log view — amend, reword, and browse commit history",
    captions: [
      "Amend, reword, or interactive-rebase from the log",
      "Commit show + stash split view",
      "Path-filtered history",
      "Bump version directly in the log",
    ],
  },
  {
    id: "worktrees",
    label: "manage worktrees",
    gif: "/demos/demo-worktrees.gif",
    gifAlt: "gx worktrees view — create, sync, and switch between worktrees",
    captions: [
      "Sync / rebase-status table at a glance",
      "Create, clone, or bulk-delete worktrees",
      "First-class .bare layout support",
      "Yank / paste paths between worktrees",
    ],
  },
]

/* ── 1. Hero ───────────────────────────────────────────────────── */

function Hero() {
  return (
    <Panel class="hero" title="gx" rightTitle="~/dev/gx">
      <div class="hero-brand">
        <img
          class="hero-logo"
          src="/logo-440.webp"
          width={240}
          height={264}
          alt="gx goblin mascot"
        />
        <p class="hero-tagline">a git TUI for reviewing changes, fast</p>
      </div>
      <img
        class="hero-demo"
        src="/demos/demo-status.gif"
        alt="gx status view — staging changes at the hunk and line level"
      />
      <div class="hero-ctas">
        <Button href="#install" icon="copy">
          Install
        </Button>
        <Button href="https://github.com/elentok/gx" icon="github" trailingIcon="star">
          GitHub
        </Button>
      </div>
    </Panel>
  )
}

/* ── 2. Install ────────────────────────────────────────────────── */

function Install() {
  return (
    <Panel id="install" title="install in seconds">
      <div class="cmd-row">
        <Cmd copy>brew install --cask gx</Cmd>
        <Cmd copy>go install github.com/elentok/gx@latest</Cmd>
      </div>
    </Panel>
  )
}

/* ── 3. The Why ────────────────────────────────────────────────── */

function TheWhy() {
  return (
    <Panel title="the why — reviewing AI code">
      <div class="why-body">
        <p>
          When parallel worktrees each carry a feature branch authored by an AI assistant, you
          review constantly — code you didn't write, at volume, across contexts that blur together.
          gx was built for exactly this.
        </p>
        <p>
          Keyboard-driven review, precise line-level staging, and two commands that close the
          loop: <kbd>a</kbd>
          <kbd>y</kbd> yanks the diff straight to your clipboard for any LLM,{" "}
          <kbd>a</kbd>
          <kbd>a</kbd> asks Claude inline. No context-switching, no copy-paste ceremony.
        </p>
        <p class="why-coda">Review first. Commit when you're sure.</p>
      </div>
    </Panel>
  )
}

/* ── 4. Feature Showcase ───────────────────────────────────────── */

function FeatureShowcase() {
  const [active, setActive] = useState(0)
  const pillar = PILLARS[active]

  return (
    <Panel title="features" class="showcase">
      <nav class="tabbar showcase-tabs" aria-label="feature pillars">
        {PILLARS.map((p, i) => (
          <button
            key={p.id}
            class="tab"
            role="tab"
            type="button"
            aria-selected={i === active}
            onClick={() => setActive(i)}
          >
            {p.label}
          </button>
        ))}
      </nav>
      <img
        key={pillar.gif}
        class="showcase-gif"
        src={pillar.gif}
        alt={pillar.gifAlt}
      />
      <ul class="showcase-captions">
        {pillar.captions.map((caption) => (
          <li key={caption} class="showcase-caption">
            <Icon name="check" color="var(--accent)" />
            {caption}
          </li>
        ))}
      </ul>
    </Panel>
  )
}

/* ── 5. Footer ─────────────────────────────────────────────────── */

function Footer() {
  return (
    <footer class="footer">
      <nav class="footer-links" aria-label="footer links">
        <a href="https://github.com/elentok/gx">
          <Icon name="github" /> GitHub
        </a>
        <a href="https://github.com/elentok/gx/blob/main/CHANGELOG.md">Changelog</a>
        <a href="https://github.com/elentok/gx/blob/main/README.md">README</a>
        <a href="https://github.com/elentok/gx/blob/main/LICENSE">License</a>
      </nav>
      <p class="footer-byline">by David Elentok</p>
    </footer>
  )
}

/* ── Root ──────────────────────────────────────────────────────── */

export function App() {
  return (
    <>
      <Hero />
      <Install />
      <TheWhy />
      <FeatureShowcase />
      <Footer />
    </>
  )
}
