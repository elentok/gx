# gx stage - e2es

Please write E2Es for the `gx stage` command, use teatest like we did for the `gx wt` E2Es.

For each test:

- create a tmp repository and make some changes
- run `gx stage` and simulate user actions
- at the end of the test verify the what was supposed to staged is actually staged

Tests:

1. stage full new file (from the sidebar)
2. stage full modified file (from the sidebar)
3. stage 2 hunks in a new file from the diff view (hunk mode)
4. stage 2 hunks in a modified file from the diff view (hunk mode)
5. stage 1 line in a new file from the diff view (line mode)
6. stage 1 line in a modified file from the diff view (line mode)
7. stage 3 line in a new file from the diff view (line mode)
8. stage 3 line in a modified file from the diff view (line mode)

If you find bugs along the way please report and fix them.
