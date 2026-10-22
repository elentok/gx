import { createSystem, defaultConfig, defineConfig, type SystemStyleObject } from "@chakra-ui/react";
import { TONES, type Tone } from "./tone";

// The only file allowed to hold raw colors and font names; see raw-style.test.ts.
const toneColors: Record<Tone, { fg: string; bg: string }> = {
  running: { fg: "#6cb4ff", bg: "rgba(108, 180, 255, 0.16)" },
  waiting: { fg: "#f2b845", bg: "rgba(242, 184, 69, 0.16)" },
  parked: { fg: "#ff7066", bg: "rgba(255, 112, 102, 0.16)" },
  deadlocked: { fg: "#ff7066", bg: "rgba(255, 112, 102, 0.16)" },
  done: { fg: "#55d996", bg: "rgba(85, 217, 150, 0.16)" },
  muted: { fg: "#b4bcc8", bg: "rgba(180, 188, 200, 0.14)" },
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
    // Registered so the live Card's border gradient can animate its angle. The cast is because
    // Chakra types globalCss values as style objects, which @property's descriptors are not.
    "@property --gx-orbit-angle": { syntax: "'<angle>'", inherits: "false", initialValue: "0deg" } as SystemStyleObject,
  },
  theme: {
    keyframes: {
      orbit: { to: { "--gx-orbit-angle": "360deg" } },
    },
    // border is semantic so it replaces Chakra's built-in light/dark "border" token.
    semanticTokens: {
      colors: {
        border: { DEFAULT: { value: "#3d4654" } },
      },
    },
    tokens: {
      colors: {
        surface: { DEFAULT: { value: "#0a0d11" }, raised: { value: "#141920" } },
        text: {
          DEFAULT: { value: "#ffffff" },
          muted: { value: "#c9d0da" },
          dim: { value: "#a3acba" },
        },
        tone: toneTokens,
      },
      fonts: {
        condensed: { value: "'Saira Condensed', sans-serif" },
        text: { value: "'Saira', sans-serif" },
      },
      fontSizes: {
        xs: { value: "13px" },
        sm: { value: "14px" },
        md: { value: "15px" },
        lg: { value: "17px" },
        xl: { value: "22px" },
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
      },
    },
  },
});

export const system = createSystem(defaultConfig, themeConfig);
