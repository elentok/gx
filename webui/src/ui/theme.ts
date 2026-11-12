import { createSystem, defaultConfig, defineConfig, type SystemStyleObject } from "@chakra-ui/react"
import { KINDS, TONES, type Kind, type Tone } from "./tone"

// The only file allowed to hold raw colors and font names; see raw-style.test.ts.
const toneColors: Record<Tone, { fg: string; bg: string; text?: string }> = {
  running: { fg: "#6cb4ff", bg: "rgba(108, 180, 255, 0.16)" },
  waiting: { fg: "#f2b845", bg: "rgba(242, 184, 69, 0.16)" },
  paused: { fg: "#ff7066", bg: "rgba(255, 112, 102, 0.16)", text: "#ffb3ad" },
  deadlocked: { fg: "#ff7066", bg: "rgba(255, 112, 102, 0.16)" },
  done: { fg: "#55d996", bg: "rgba(85, 217, 150, 0.16)" },
  muted: { fg: "#b4bcc8", bg: "rgba(180, 188, 200, 0.14)" },
}

const toneTokens = Object.fromEntries(
  TONES.map((t) => {
    const { fg, bg, text } = toneColors[t]
    return [
      t,
      {
        fg: { value: fg },
        bg: { value: bg },
        ...(text && { text: { value: text } }),
      },
    ]
  }),
)

const kindColors: Record<Kind, { fg: string; bg: string }> = {
  design: { fg: "#c4a7ff", bg: "rgba(196, 167, 255, 0.13)" },
  implementation: { fg: "#7ad8d0", bg: "rgba(122, 216, 208, 0.13)" },
  oneoff: { fg: "#f0a6ca", bg: "rgba(240, 166, 202, 0.13)" },
  followups: { fg: "#b4bcc8", bg: "rgba(180, 188, 200, 0.13)" },
}

const kindTokens = Object.fromEntries(
  KINDS.map((k) => [k, { fg: { value: kindColors[k].fg }, bg: { value: kindColors[k].bg } }]),
)

export const themeConfig = defineConfig({
  globalCss: {
    "html, body": {
      bg: "surface",
      color: "text",
      fontFamily: "text",
    },
    // iOS Safari zooms into a focused field whose font is under 16px. Chakra's recipe layer sits
    // above globalCss, hence !important; desktop keeps the small fields.
    "input, textarea, select": {
      "@media (pointer: coarse)": { fontSize: "16px !important" },
    },
    // The one keyboard focus ring: a 2px gap in the surrounding surface, then a 2px blue ring.
    // !important because recipes and component styles sit above globalCss. The sidebar sets
    // --gx-focus-gap to its own colour; full-width sidebar rows opt into the inset form.
    ":focus-visible": {
      outline: "none !important",
      boxShadow:
        "0 0 0 2px var(--gx-focus-gap, {colors.surface}), 0 0 0 4px {colors.tone.running.fg} !important",
    },
    // A menu holds focus itself and marks the keyboard-highlighted item, so the ring belongs to that item.
    "[role='menu']:focus-visible": { boxShadow: "none !important" },
    "[role='menu']:focus-visible [role='menuitem'][data-highlighted]": {
      boxShadow: "0 0 0 2px {colors.surface.raised}, 0 0 0 4px {colors.tone.running.fg} !important",
    },
    // A field inside a box that draws the ring itself (PromptBox): the field shows none.
    "[data-focus-ring='none']:focus-visible": { boxShadow: "none !important" },
    "[data-focus-ring='inset']:focus-visible": {
      borderRadius: "8px",
      boxShadow: "inset 0 0 0 2px {colors.tone.running.fg} !important",
    },
    // Registered so the live Card's border gradient can animate its angle. The cast is because
    // Chakra types globalCss values as style objects, which @property's descriptors are not.
    "@property --gx-orbit-angle": {
      syntax: "'<angle>'",
      inherits: "false",
      initialValue: "0deg",
    } as SystemStyleObject,
  },
  theme: {
    keyframes: {
      orbit: { to: { "--gx-orbit-angle": "360deg" } },
      chatDot: {
        "0%, 60%, 100%": { transform: "translateY(0)", opacity: 0.5 },
        "30%": { transform: "translateY(-2px)", opacity: 1 },
      },
      chatShimmer: {
        from: { maskPosition: "100% 0" },
        to: { maskPosition: "0% 0" },
      },
    },
    // border is semantic so it replaces Chakra's built-in light/dark "border" token.
    semanticTokens: {
      colors: {
        border: { DEFAULT: { value: "#3d4654" } },
      },
    },
    tokens: {
      colors: {
        surface: {
          DEFAULT: { value: "#0a0d11" },
          raised: { value: "#141920" },
        },
        sidebar: {
          DEFAULT: { value: "#11161d" },
          hover: { value: "rgba(255, 255, 255, 0.06)" },
          current: { value: "rgba(108, 180, 255, 0.14)" },
          selected: { value: "rgba(108, 180, 255, 0.24)" },
        },
        overlay: { value: "rgba(0, 0, 0, 0.7)" },
        hairline: {
          DEFAULT: { value: "#252c36" },
          folded: { value: "#1d232c" },
        },
        text: {
          DEFAULT: { value: "#ffffff" },
          muted: { value: "#c9d0da" },
          dim: { value: "#a3acba" },
          faint: { value: "#6b7584" },
        },
        tone: toneTokens,
        kind: kindTokens,
        chat: { value: "#ec6fa8" },
        // Day-chart series: Claude blue, Codex purple, then the group colors. Not the harness
        // colors, which are orange and green.
        chart: {
          claude: { value: "#6cb4ff" },
          codex: { value: "#b48cff" },
          blue: { value: "#6cb4ff" },
          purple: { value: "#b48cff" },
          amber: { value: "#e8c25a" },
          green: { value: "#5fd39a" },
          muted: { value: "#6b7584" },
        },
        harness: { claude: { value: "#c2622d" }, codex: { value: "#2a8a6a" } },
      },
      fonts: {
        condensed: { value: "'Saira Condensed', sans-serif" },
        text: { value: "'Saira', sans-serif" },
        mono: { value: "ui-monospace, SFMono-Regular, Menlo, monospace" },
      },
      fontSizes: {
        xs: { value: "13px" },
        sm: { value: "14px" },
        md: { value: "15px" },
        // Phone-readable reading size, a step above md.
        base: { value: "16px" },
        lg: { value: "17px" },
        subtitle: { value: "18px" },
        xl: { value: "22px" },
        title: { value: "24px" },
        "2xl": { value: "30px" },
      },
      spacing: {
        1: { value: "4px" },
        2: { value: "8px" },
        3: { value: "12px" },
        4: { value: "16px" },
        6: { value: "24px" },
      },
      radii: {
        sm: { value: "3px" },
        md: { value: "6px" },
        lg: { value: "10px" },
      },
      sizes: {
        bar: { value: "6px" },
        barThin: { value: "4px" },
      },
    },
  },
})

export const system = createSystem(defaultConfig, themeConfig)
