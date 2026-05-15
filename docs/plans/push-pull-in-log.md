# Push and Pull in Log View

Add `P` (push) and `p` (pull) to the log view, matching the status view experience.

## Decisions

- Full push flow: confirm → fetch (divergence check) → push → force-push if needed → PR URL prompt
- Pull flow: no confirm, just run (same as status)
- Both support credential prompting via `components.CommandRunner` with `CredentialPolicyPrompt`
- Spinner + result (no streaming); accumulated output stored so `g o` works
- New `ui/push` and `ui/pull` packages (same pattern as `ui/amend`)
- Status refactoring (to use the shared packages) is a separate follow-up

## Tasks

- [x] Create `ui/push/push.go` — self-contained push state machine
- [x] Create `ui/pull/pull.go` — self-contained pull state machine
- [x] Wire push + pull into log view (`model.go`, `model_keys.go`, `model_update.go`, `view.go`)
