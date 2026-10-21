# Prompts

The instructions v gives its agents live in this directory as plain Markdown, so they are easy to read and to change. They are built into the binary; to try a change without rebuilding, put a file with the same name in one of these places (the first one found wins):

1. `<working directory>/.v/prompts/<name>.md`: for one project.
2. `~/.v/prompts/<name>.md`: for you.
3. the built-in `prompts/<name>.md`.

| file | who reads it | when |
| --- | --- | --- |
| `chat.md` | the Claude Code agent in the chat, appended to its system prompt | every session |
| `reviewer.md` | the agent of `/review make`, appended to its system prompt | a review run (and later the reviewer of the review/fix loop) |
| `fixer.md` | the first message of the agent of `/apply`, before the list of threads | an apply run (and later the fixer of the loop) |

A file is a Go template (`text/template`): `{{.Name}}` inserts a value and `{{if .Flag}}…{{end}}` keeps a block only when the flag is set. A file that does not parse, or uses a name that does not exist, falls back to the built-in one and v says so in the chat.

| value | meaning |
| --- | --- |
| `.Workdir` | the working directory |
| `.Number`, `.Title`, `.Head`, `.Base` | the pull request |
| `.Branch` | the branch the working tree is on |
| `.Auto` | the run is in auto mode: nobody is watching |
| `.Loop`, `.Round`, `.MaxRounds`, `.Since` | the review/fix loop: on, which round, how many at most, the commit the reviewer looks from |

The tool names the prompts mention (`pr_comment`, `pr_reply`, …) are served by v; changing a prompt does not change what the tools can do.
