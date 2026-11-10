import { Question, QuestionDots } from "../ui/question-sheet/Question";
import type { GalleryEntry } from "./registry";

export const questionEntries: GalleryEntry[] = [
  {
    name: "Question",
    cases: [
      {
        label: "options",
        node: (
          <Question
            card={{
              label: "1",
              title: "Pick one",
              kind: "options",
              recommended: "a",
              options: [
                { id: "a", label: "First", consequence: "Ships today" },
                { id: "b", label: "Second" },
              ],
            }}
            answer={{ choice: "b" }}
            onChange={() => {}}
            onPicked={() => {}}
          />
        ),
      },
      {
        label: "yes-no",
        node: (
          <Question
            card={{ label: "2", title: "Proceed?", kind: "yes-no", yes: "Go ahead", no: "Stop here" }}
            answer={{}}
            onChange={() => {}}
            onPicked={() => {}}
          />
        ),
      },
      {
        label: "dots",
        node: (
          <QuestionDots
            round={["1", "2", "3"].map((label) => ({ label, title: "", kind: "options" }))}
            answers={{ "1": { choice: "a" } }}
            current={1}
            onPick={() => {}}
          />
        ),
      },
    ],
  },
];
