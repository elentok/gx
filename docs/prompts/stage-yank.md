# gx stage - yank

Please add two yank mappings:

## "yc" Yank context for AI agents:

- in staged/unstaged - yank the filename and line number (or range) and the content of the hunk (without styling), e.g.

  ```
  @path/to/file.ts L10-14

  +added line 1
  +added line 2
  -removed line 3
  ```

- in status view - yank just the filename (`@path/to/file.ts`)

## "yf" Yank filename

Should work in all views
