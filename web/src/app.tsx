import { Tabs } from "./components/Tabs.tsx"
import { Icon } from "./components/Icon.tsx"
import {
  Badge,
  Button,
  Cmd,
  Panel,
  SyncStatus,
  type SyncState,
} from "./components/primitives.tsx"

interface Worktree {
  branch: string
  state: SyncState
  ahead?: number
  behind?: number
}

const WORKTREES: Worktree[] = [
  { branch: "main", state: "synced" },
  { branch: "feat/ask-ai", state: "ahead", ahead: 3 },
  { branch: "fix/diff-nav", state: "behind", behind: 2 },
  { branch: "spike/kitty", state: "diverged", ahead: 4, behind: 1 },
]

function Hero() {
  return (
    <Panel class="hero" title="gx" rightTitle="~/dev/gx">
      <img
        class="hero-logo"
        src="/logo-440.webp"
        width={200}
        height={219}
        alt="gx — a goblin in a terminal window holding a graffiti 'gx' wordmark"
      />
      <p class="hero-tagline">a git TUI for reviewing changes, fast</p>
      <div class="hero-ctas">
        <Button href="#install" primary icon="copy">
          Install
        </Button>
        <Button
          href="https://github.com/elentok/gx"
          icon="github"
          trailingIcon="star"
        >
          GitHub
        </Button>
      </div>
    </Panel>
  )
}

function WorktreesPreview() {
  return (
    <Panel
      title={
        <>
          <Icon name="worktree" /> worktrees
        </>
      }
      rightTitle="~/dev/gx"
    >
      <table class="wt-table">
        <tbody>
          {WORKTREES.map((wt) => (
            <tr key={wt.branch}>
              <td>
                <Icon name="branch" color="var(--mauve)" /> {wt.branch}
              </td>
              <td class="wt-sync">
                <SyncStatus state={wt.state} ahead={wt.ahead} behind={wt.behind} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </Panel>
  )
}

function BadgesPreview() {
  return (
    <Panel inset title="badges">
      <div class="badge-row">
        <Badge variant="green">staged</Badge>
        <Badge variant="yellow">modified</Badge>
        <Badge variant="orange">untracked</Badge>
        <Badge variant="mauve">renamed</Badge>
        <Badge variant="blue">info</Badge>
        <Badge variant="lime">AI</Badge>
        <Badge>default</Badge>
        <Badge variant="deepbg">deepbg</Badge>
      </div>
    </Panel>
  )
}

function Actions() {
  return (
    <Panel id="install" title="install">
      <div class="cmd-row">
        <Cmd>brew install --cask gx</Cmd>
        <Cmd>go install github.com/elentok/gx@latest</Cmd>
      </div>
      <p class="ai-hint">
        press <kbd>a</kbd>
        <kbd>a</kbd> to Ask AI, <kbd>a</kbd>
        <kbd>y</kbd> to Yank for AI
      </p>
    </Panel>
  )
}

export function App() {
  return (
    <>
      <Hero />
      <Tabs />
      <WorktreesPreview />
      <BadgesPreview />
      <Actions />
    </>
  )
}
