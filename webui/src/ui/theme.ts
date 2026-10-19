import { createSystem, defaultConfig, defineConfig } from "@chakra-ui/react";
import { TONES, type Tone } from "./tone";

// The only file allowed to hold raw colors and font names; see raw-style.test.ts.
const toneColors: Record<Tone, { fg: string; bg: string }> = {
  running: { fg: "#3fb97a", bg: "rgba(63, 185, 122, 0.16)" },
  waiting: { fg: "#e0a43a", bg: "rgba(224, 164, 58, 0.16)" },
  parked: { fg: "#6a9bd8", bg: "rgba(106, 155, 216, 0.16)" },
  deadlocked: { fg: "#e5574f", bg: "rgba(229, 87, 79, 0.16)" },
  muted: { fg: "#8b94a1", bg: "rgba(139, 148, 161, 0.14)" },
};

const toneTokens = Object.fromEntries(
  TONES.map((t) => [t, { fg: { value: toneColors[t].fg }, bg: { value: toneColors[t].bg } }]),
);

export const themeConfig = defineConfig({
  globalCss: {
    "html, body": {
      bg: "surface",
      color: "text",
      fontFamily: "text",
    },
  },
  theme: {
    tokens: {
      colors: {
        surface: { DEFAULT: { value: "#0e1116" }, raised: { value: "#151a21" } },
        border: { DEFAULT: { value: "#262d38" } },
        text: {
          DEFAULT: { value: "#e6e9ee" },
          muted: { value: "#9aa4b2" },
          dim: { value: "#626c7a" },
        },
        tone: toneTokens,
      },
      fonts: {
        condensed: { value: "'Saira Condensed', sans-serif" },
        text: { value: "'Saira', sans-serif" },
      },
      fontSizes: {
        xs: { value: "11px" },
        sm: { value: "12px" },
        md: { value: "14px" },
        lg: { value: "16px" },
        xl: { value: "20px" },
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
      },
    },
  },
});

export const system = createSystem(defaultConfig, themeConfig);
