// The only code that formats a model for display. Components take a ModelLabel,
// so tsc rejects a raw ID. Rules: docs/specs/better-epic-header.md "Model names".

declare const modelLabelBrand: unique symbol;
export type ModelLabel = string & { readonly [modelLabelBrand]: true };

const label = (text: string) => text as ModelLabel;

const CLAUDE_ALIASES = new Set(["opus", "sonnet", "haiku"]);
const HARNESS_NAMES = { claude: "Claude", codex: "Codex" } as const;
type KnownHarness = keyof typeof HARNESS_NAMES;

const capitalise = (word: string) => word.charAt(0).toUpperCase() + word.slice(1);

function inferHarness(base: string): KnownHarness | undefined {
  if (base.startsWith("claude-") || CLAUDE_ALIASES.has(base)) return "claude";
  if (base.startsWith("gpt-")) return "codex";
  return undefined;
}

function prettyClaude(base: string): string | undefined {
  if (CLAUDE_ALIASES.has(base)) return `Claude ${capitalise(base)}`;
  const match = /^claude-([a-z]+)-(\d+(?:-\d+)*)$/.exec(base.replace(/-\d{8}$/, ""));
  if (!match) return undefined;
  return `Claude ${capitalise(match[1])} ${match[2].replaceAll("-", ".")}`;
}

function prettyCodex(base: string): string | undefined {
  if (!base.startsWith("gpt-")) return undefined;
  const words = base.split("-").map((w) => (w === "gpt" ? "GPT" : capitalise(w)));
  return `Codex ${words.join("-")}`;
}

export function modelName(id: string, harness?: string): ModelLabel {
  const explicit = harness?.toLowerCase();
  const known: KnownHarness | undefined =
    explicit === "claude" || explicit === "codex" ? explicit : undefined;
  const text = id.trim();

  if (text === "") return label(known ? `${HARNESS_NAMES[known]} CLI default` : "");

  const suffix = /\[([^\]]*)\]$/.exec(text);
  const base = suffix ? text.slice(0, suffix.index) : text;
  const oneM = suffix?.[1].toLowerCase() === "1m" ? " (1M)" : "";

  const resolved = known ?? inferHarness(base);
  if (!resolved) return label(text);

  const pretty = resolved === "claude" ? prettyClaude(base) : prettyCodex(base);
  if (pretty) return label(pretty + oneM);
  return label(known ? `${HARNESS_NAMES[known]} ${text}` : text);
}
