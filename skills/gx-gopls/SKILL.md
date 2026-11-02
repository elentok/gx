---
name: gx-gopls
description: >-
  Navigate and refactor Go code with the built-in `LSP` tool and the `gopls` CLI instead of grep. Use
  when working on a Go codebase and you need definitions, references, implementations, call
  hierarchy, or a safe rename.
---

# Go code intelligence

Use semantic tools for symbols. Use `rg` for text (comments, strings, TODOs).

Never use grep to find definitions, references, or implementations.

## Built-in `LSP` tool

Every call needs `filePath`, `line`, and `character` (all 1-based). Put the cursor on the symbol name.
Get the position from `documentSymbol` or `rg -n`.

| Task                          | Operation                             |
| ----------------------------- | ------------------------------------- |
| List symbols in a file        | `documentSymbol`                      |
| Find a symbol by name         | `workspaceSymbol` (fuzzy, set `query`) |
| Jump to definition            | `goToDefinition`                      |
| Find all references           | `findReferences`                      |
| Find interface implementers   | `goToImplementation` (interfaces/types only) |
| Who calls this function       | `incomingCalls`                       |
| What this function calls      | `outgoingCalls`                       |
| Type and docs of a symbol     | `hover`                               |

- `workspaceSymbol` is fuzzy and returns up to 100 hits, many unrelated. Use an exact name.
- `incomingCalls` and `outgoingCalls` need a function or method position.
- If `LSP` returns an error or no server, fall back to the CLI below.

## `gopls` CLI (via Bash)

For what `LSP` cannot do. Positions are `file:line:col` (1-based).

- Rename: `gopls rename -d file.go:LINE:COL NewName` previews a diff. Use `-w` instead of `-d` to apply.
- Package symbols: `gopls symbols file.go`.
- Dependencies of a package: `go list -deps ./pkg` or `go list -f '{{.Imports}}' ./pkg`.
- Compile check: `go build ./...` or `go vet ./...`.

Each call starts a short-lived gopls, so it holds no open files afterwards.

## Fallbacks

Use `rg` + Read only when:

1. Two attempts at the semantic call fail or time out.
2. The result is empty but the symbol certainly exists.

Retry the semantic tool on the next question.
