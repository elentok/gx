In the "gx push", "gx wt"'s push and "gx stage"'s push:

- Before pushing:
  - If there's a remote branch - run `git fetch origin`
  - If the current branch has diverged (and fast-forward push is not possible) tell it to the user:

    ```
    Branch {branch} has diverged from the remote branch:

      Last local commit: {hash} {message}
      Last remote commit: {hash} {message}
    ```

    give the user 3 options:
    1. Rebase
    2. Push --force
    3. Abort
