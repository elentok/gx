## Planning the verify tickets (step 6)

Write a plan file (YAML). Per story with user-visible behaviour it lists
the story number, its scenario group, its items by kind (web flow, CLI check, Go test run, skill
quote) and its mock states by name, plus the fixture scenario. gx does the estimating and packing.

`gx tickets verify-plan` reads the spec at `docs/specs/<name>.md`, copies each story's Verification
entry and Mock states line word for word, packs the parts, and appends the verify entries and their
body files to the publish manifest.

gx gives one lone `verify` entry when every story fits one part. Otherwise it gives top-level
`verify-part-N` entries and one `verify-collect` entry blocked by every part. The body formats
follow [gx-verify-epic](../gx-verify-epic/SKILL.md) ("The verify part body"). Every entry sets
`expected_context_window`. Do not restate the formats here.
