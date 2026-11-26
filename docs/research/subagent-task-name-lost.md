# Why task rows lose the subagent name

Ticket: gx:subagent-links-working-card/02. Research only, no product code changed.

## Answer

The name is lost because the whole Agent tool_use is never parsed, so no record carries
the description.

- Claude Code writes `run_in_background` as the **string** `"true"` in many Agent and
  Bash tool_use inputs. Across `~/.claude/projects/*/*.jsonl` there are 1215 lines with
  `"run_in_background":"true"` and 4672 with the boolean `true`.
- `taskUse` declares `RunInBackground bool` (`transcript/taskrecords.go:313`).
  `json.Unmarshal` of a string into a bool fails.
- `taskToolUses` (`transcript/taskrecords.go:318-327`) unmarshals the whole message in one
  call and returns `nil` on any error (`if json.Unmarshal(message, &m) != nil { return nil }`).
  One bad `input` drops **every** Agent, Task, Bash and Monitor tool_use of that assistant
  line. The "assistant" case (`taskrecords.go:132-150`) then builds no record and never
  sets `byUse[id]`.
- The launch result (`toolUseResult.status == "async_launched"`) still arrives. The
  tool_result branch finds no `byUse` entry (`taskrecords.go:179-181`, `r == nil` then
  `continue`), so `byTask` stays empty for that id.
- The fallback at `taskrecords.go:217-226` (`res.launch()` with `byTask[id] == nil`) then
  creates the row: `&TaskRecord{Kind, TaskID, StartedAt}` with `ID = id`. It has no `Label`
  and no `CallID`.
- `server/sessions/chat/tasks.go` `wireTask` copies `r.Label` and `r.CallID` as they are.
  `TaskList.tsx` shows `kindWord` plus `task.label` (empty) plus the time. With no
  `call_id` the row also has no link to the subagent page (`TaskList.tsx:117`).

Reproduced with the real reader on
`~/.claude/projects/-Users-david-dev-gx-question-detection-design/7259529c-6850-404e-b0f8-caa0df405a57.jsonl`
(two background Agents with `"run_in_background":"true"`): both subagent records came back
with `label=""` and `call=""`. In the same reader, a transcript with a boolean `true`
(this session's own) gives a label and call id.

Fix tried in a throwaway copy (a `flexBool` type that accepts `true` and `"true"`): both
records then had their description and call id. The copy was discarded.

## Duplicates

No. Because the tool_use record is never created, only the fallback record exists: one row
per subagent. Both before and after the fix there is one row per launch. A duplicate would
need a tool_use record that was not paired with its result, and that does not happen here.

## Codex

Not affected. `codexsession/rolloutscan.go:248-254` reads `task_name` (a string, `required`
in the `spawn_agent` schema) from the function_call arguments and sets
`Label: a.TaskName` plus `CallID` on the record. It never goes through the fallback. No
`spawn_agent` function_call exists in the local rollouts, so this part is checked against the
tool schema in the rollout headers and the existing tests, not a live call.

## Where the fix belongs

1. The cause (`transcript/taskrecords.go:313`): decode `run_in_background` with a type that
   accepts a boolean or the string `"true"`/`"false"`. Fix it where the field is read, so
   Bash background shells with the string form are also recognised as shells.
2. Make `taskToolUses` robust: decode each content block on its own (or each `input`), so
   one malformed input cannot drop its siblings. This is the reason one odd field costs the
   whole message.
3. Defence at the fallback (`taskrecords.go:217-226`, optional): the async_launched result
   itself carries `description`. Add `Description` to `taskRecordResult` and use it as the
   label when a launch has no tool_use. This keeps the name even if tool_use parsing fails
   again. It does not restore `CallID`, so (1) and (2) are still the real fix.
4. Add a test next to `transcript/taskrecords_agents_test.go` with a tool_use whose
   `run_in_background` is the string `"true"`, plus a sibling tool_use in the same message.
