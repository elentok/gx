# gx status - commits in branch

I want to add another frame below the status frame, it will be read-only and
not focusable for now (I'll work on that later).

it should show the the commits of the current branch:

- all commits starting from the main branch
- top most commit is the most recent
- should be color coded:
  - commits diverged from remote - red
  - commits in remote - regular
  - new commits - green
- it should show the short hash, the title, and the relative date (X minutes ago)
- should probably look something like this:

  ```
  {title}
  X minutes ago, {hash}
  ```

  - Render the second line in italic, in a dim color
  - use word wrapping
