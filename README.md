<p align="center">
  <img src="assets/logo.svg" width="200" alt="v">
</p>

`v` is a terminal tool for coding together with an AI agent: you and the agent take turns editing the same code.

What works now:

- an omnibox with commands: type `/` for a menu that filters as you type; Tab, Shift-Tab and arrows move the selection, Enter picks it;
- workdir defaults to the directory where `v` was launched. Start with `v /path/to/project` or `v -C /path/to/project` (`--workdir` also works); `/workdir` opens a directory list: → enters the highlighted folder, ← goes up (and highlights the folder you left), Enter chooses it; `/workdir <directory>` changes it directly, and `/workdir` shows the current path. The agent and conversation are kept when changing workdir;
- `/open [path]` stays in the omnibox and lists workdir (or the path's directory) in the main area; typing fuzzy-filters names, ↑↓ select, → enters a folder or opens a file, ← goes up, Enter opens (a name with no match is created empty); files open with syntax highlighting;
- in windows at least 100 columns wide, `/open`, `/workdir`, `/switch` and `/show` add a read-only preview on the right: a highlighted file or a folder's contents, a branch's recent commits, or the selected commit's message and patch. Shift-↑↓ or the mouse wheel over it scroll the preview;
- `/show` opens the current branch's history (last 500 commits); once it is open, typing in the omnibox searches hash, subject and author
- the right side of the single bottom status line shows workdir, the Git branch and its PR, then the agent's model and effort; clicking the model or the effort opens a dropdown to change it (the agent itself is still picked in `/config`). Click the branch, press Ctrl-B or use `/branch [name]` to select a local or already-fetched remote branch; switching uses `git switch`. Git state refreshes in the background, including changes made outside `v`;
- PR discovery is optional and uses `gh` for GitHub remotes. One PR is displayed directly; multiple PRs get a dropdown (click it or press Ctrl-P). `/pr <URL>` links any forge's PR to the current branch through local Git config (`branch.<name>.v-pr`); `/pr <number>` works for GitHub remotes. `/pr auto` removes the explicit link and returns to automatic selection. Git and manual URL links work without `gh`; `/pr` on a GitHub PR opens a read-only screen: a Conversation entry (description, reviews, comments) and the PR's files on the left, the selected file's diff on the right with inline review comments under their lines; the list on the left switches between Files and Comments (Ctrl-L or a click on its first row); typing in the omnibox searches it, and `gh` is required. With several PRs and no explicit link, `/pr` first opens the dropdown to choose one
- `/review` has Claude Code review this branch's GitHub PR in the background: a one-shot, headless `claude -p` whose chat session is separate from yours. The reviewer has no access to GitHub; it works through the `pr_*` tools of a small MCP server that `v` runs on localhost for the length of the review (random port and token, `--strict-mcp-config`, so your other MCP servers are not loaded). Read tools: `pr_description`, `pr_list`, `pr_diff` (lines numbered as in the new file), `pr_file` (head or base), `pr_threads` (all, resolved, unresolved, pending or rejected, including its own proposals), `pr_checks` (all or failing). Write tools only *propose*: `pr_comment` and `pr_reply`. Its thinking and the files it reads stream into the chat, and each proposal arrives as a card: the diff around the line with the comment right under it, marked `!!` critical (bug, data loss, security), `!` strongly recommended (design, readability, maintainability) or unmarked (an optional remark). The reviewer is told to report only what matters and skip trivia. `/config` → Review mode: *Manual* holds the reviewer's tool call until you answer (`y` accepts, `n` skips: it is told not to repost or reword it, `a` accepts the rest, Esc drops the review) and then publishes the accepted ones as one review; *Auto* accepts everything and saves it as a **pending** review that only you can see. `/review submit` publishes your pending review, `/review discard` deletes it
- files are editable in every view mode: Enter or a click starts editing, Ctrl-S saves, Ctrl-Z / Ctrl-Y undo and redo, Esc stops; in diffs the working copy side is editable and the whole file is shown while editing;
- Flat, Unified diff and Side-by-side against git `HEAD` are always available in the top-right dropdown (Ctrl-T or a click; Tab also works in a file). Outside a file, the selection sets the next file's view. Files without Git history are shown flat;
- the single bottom status line keeps agent information and context controls. In a file it offers Edit, Save and Back; while editing, Save, Undo, Redo and Done;
- screens stack up: Esc goes back one screen, down to the chat;
- `/config` edits `~/.v/settings.json`; for now it picks the color theme. Built-in themes are One Dark Pro (+ Darker), One Light and GitHub Dark/Light; any VS Code theme JSON dropped into `~/.v/themes/` shows up too;
- `/config` also sets the diff context: how many unchanged lines show before and after each change (default 3 and 3, `diff_before` / `diff_after` in `settings.json`). It applies to every diff: file views, agent proposals, commits in `/show` and PRs in `/pr`; the PR patch comes from GitHub with 3 lines of context, so a PR can be trimmed below that but not widened;
- the agent is Claude Code or Codex, picked in `/config` with its model and effort (Codex lists its models itself). Codex runs through `codex app-server` on your ChatGPT login, in a read-only sandbox: reads go through, edits and writing commands ask;
- `v` connects to the agent at launch; anything typed without `/` goes to it (`claude -p`, signed in with your Claude subscription; `ANTHROPIC_API_KEY` is ignored). The model and effort are set in `/config` (Opus 5.5, high by default); a change applies from the next message and keeps the conversation. Replies stream into the chat as rendered markdown, and Ctrl-O shows the model's thinking; every file edit the agent proposes opens as a diff (Unified, Side-by-Side or Flat) with Yes / Edit first / No (↑↓ and Enter, or y / e / n); Tab adds a note that goes with the answer; with Edit first you change the proposal and Ctrl-S saves your version, and the agent is told what you changed. Other tool calls ask Yes / No. Esc interrupts; messages typed while the agent works are queued;
- ↑↓ in the omnibox walk through what you typed before (kept in `~/.v/history`); with some text typed, only lines starting with it;
- `/resume` lists past Claude Code sessions of this directory (yours from the terminal too) and continues the chosen one with its chat;
- `/reset` starts over with a fresh splash; `/q`, `/quit`, or Ctrl-D / Ctrl-C twice exits.

The plan:

- reviewing the agent's edits as diffs and changing them before they land.

## Build

Requires Go 1.27+ and a truecolor terminal (iTerm2, Ghostty, WezTerm, Kitty).

```sh
make build   # go build -o v .
make run     # build and start
make lint    # golangci-lint (brew install golangci-lint)
```

Or install into `$GOPATH/bin`:

```sh
go install github.com/dasimagin/v@latest
```

Quit with `/q` or press `Ctrl-D` twice.
