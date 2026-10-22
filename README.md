<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="assets/logo-light.svg">
    <img src="assets/logo-light.svg" width="200" alt="v">
  </picture>
</p>

# v

`v` is an experimental terminal tool, written in Go, for vibe coding: you talk to a coding agent, read and edit the files it touches, and review its work as diffs without leaving the terminal.

It is a young project. Expect rough edges and changes in behavior.

## Main features

- **Chat with the agent.** Type to talk to [Claude Code](https://claude.com/claude-code); `/resume` picks up past sessions, model and effort are dropdowns in the bottom line.
- **Files and diffs in the same window.** Browse and fuzzy-search files with `/open`, edit them in place, and see changes against `HEAD` as flat, unified or side-by-side diffs.
- **Workdir and Git.** Switch the working directory (`/workdir`) and branches (`/switch`), read branch history (`/show`); every picker has a preview.
- **A PR screen.** `/pr <url>` checks out a pull request and shows it as a diff with its review threads: reply, resolve, comment on lines and ranges, keep everything in a pending review and submit it with a verdict.
- **Review agents (experimental).** `/review make` has an agent review the PR and post comments; `/apply` has an agent fix the open threads, commit, push and reply; `/converge` runs the two in turns until the reviewer has nothing blocking left. The agents work with GitHub only through tools `v` provides; their instructions are Markdown files in [`prompts/`](prompts/README.md) that you can override.
- **Themes and a tidy UI.** Dark themes, mouse support, a searchable `/config`, help on `?`.

## Limitations

For now `v` supports only Claude Code as the agent and only GitHub (through the `gh` CLI) for pull requests.

## Install

Requires Go 1.27+, a truecolor terminal (iTerm2, Ghostty, WezTerm, Kitty) and `git`. For the PR features also `gh` (signed in with `gh auth login`) and `claude` (Claude Code, signed in).

```sh
go install github.com/dasimagin/v@latest
```

or from a clone:

```sh
make build   # go build -o v .
./v /path/to/project
```

Quit with `/q` or `Ctrl-D` twice.

## Documentation

The full list of commands, screens and settings is in [docs/reference.md](docs/reference.md).
