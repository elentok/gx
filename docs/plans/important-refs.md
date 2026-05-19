# Log Ref Badge Redesign

## Context

The git log view shows all refs (branches, tags) as bright pill badges. In repos with many tags,
80%+ of rows have badges — some with many — making the UI feel visually noisy. The goal is to
introduce a two-tier badge system: dim (surface) badges for most refs, and bright color badges
for "important" refs defined by user-configurable regex rules.

## Decisions

- All refs default to dim surface badges
- Important refs are defined by `log.important-refs` in config: a list of `{ patterns, color }` rules
- Pattern matching: regex
- Ordering: important refs appear first (in rule order), unmatched refs after
- Colors: named Catppuccin names (`yellow`, `blue`, `mauve`, etc.) + hex strings (`#aeaeae`)
- Default preset (when config key absent): main/master rules with yellow
- Badge padding: add `padding bool` (default `true`) to Badge component; log view passes `false`

## Tasks

- [x] `config/log.go` — new file: LogConfig, ImportantRefRule, DefaultLogConfig
- [x] `config/config.go` — add Log field, update Default() and Load()
- [x] `ui/badge.go` — add padding param, add RenderBadgeWithColor
- [x] `ui/color_resolve.go` — new file: ResolveNamedColor helper
- [x] `ui/settings.go` — add LogConfig field
- [x] `cmd/cmd.go` — pass LogConfig to settings
- [x] `ui/log/view.go` — compiled rules, sort decorations, updated renderBadges, remove old helpers
- [x] `docs/config-schema.json` — schema for log.important-refs

## Config shape

```json
{
  "log": {
    "important-refs": [
      { "patterns": ["^main$", "^master$", "^origin/main$", "^origin/master$"], "color": "yellow" },
      { "patterns": ["^v\\d"], "color": "#aeaeae" }
    ]
  }
}
```

Default (when key absent): main/master rule with yellow.

## Verification

- [ ] `go build ./...` — no compile errors
- [ ] `go test ./...` — all tests pass
- [ ] `gx log` in a repo with many tags — most badges are dim surface
- [ ] `main`/`origin/main` appear first and are yellow
- [ ] Custom rule with hex color applies correctly
- [ ] Badge padding is visually tighter (no inner spaces)
- [ ] Works with `use-nerdfont-icons: false`
