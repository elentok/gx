# Log amend specific commit

In log view or commit view - when pressing "A" it will show a confirmation modal to the user:

```
Do you want to amend the following staged changes into this commit:

{list of staged files}
```

If the uses chooses yes - amend that specific commit.

Notes:

- After the amend refresh the view

Open questions:

- How do we amend the commit? via rebase? or is there a simpler way?
