import { expect, test } from "vitest"
import { commandBlock } from "./taskCommand"

test.each([
  ["python3 -c 'import os\nprint(1)'", { text: "import os\nprint(1)", language: "python" }],
  [`python -c "print(\\"hi\\")"`, { text: 'print("hi")', language: "python" }],
  [
    "python3 - <<'EOF'\nimport sys\nprint(2)\nEOF",
    { text: "import sys\nprint(2)", language: "python" },
  ],
  ["pnpm run build", { text: "pnpm run build", language: "bash" }],
  ["python3 -c 'print(1", { text: "python3 -c 'print(1", language: "bash" }],
  ["python3 - <<EOF\nprint(1)", { text: "python3 - <<EOF\nprint(1)", language: "bash" }],
])("commandBlock(%j)", (command, want) => {
  expect(commandBlock(command)).toEqual(want)
})
