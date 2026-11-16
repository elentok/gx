/** Token colours for highlight.js markup, shared by the file viewer and the task command block. */
export const tokenStyles = {
  "& .hljs-keyword, & .hljs-built_in, & .hljs-type, & .hljs-literal, & .hljs-selector-tag": {
    color: "tone.running.fg",
  },
  "& .hljs-string, & .hljs-regexp, & .hljs-addition": { color: "tone.done.fg" },
  "& .hljs-number, & .hljs-symbol, & .hljs-attr, & .hljs-attribute": { color: "tone.waiting.fg" },
  "& .hljs-title, & .hljs-function, & .hljs-section": { color: "chart.purple" },
  "& .hljs-comment, & .hljs-quote, & .hljs-meta": { color: "text.faint", fontStyle: "italic" },
  "& .hljs-deletion": { color: "tone.paused.fg" },
} as const
