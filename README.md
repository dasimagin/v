<div align="center">

```
██╗          ██╗
╚██╗        ██╔╝
 ╚██╗      ██╔╝
  ╚██╗    ██╔╝
   ╚██╗  ██╔╝
    ╚██╗██╔╝
     ╚███╔╝
      ╚══╝
```

*v is for vibing*

</div>

`v` is a terminal tool for coding together with an AI agent: you and the agent take turns editing the same code.

What works now:

- an omnibox with commands: type `/` for a menu that filters as you type; Tab, Shift-Tab and arrows move the selection, Enter picks it;
- `/open [path]` opens a file with syntax highlighting (One Dark Pro Darker) or lists a directory; without a path it lists the working directory; paths complete on Tab;
- screens stack up: Esc goes back one screen, down to the chat;
- `/reset` starts over with a fresh splash; `/q`, `/quit`, or Ctrl-D / Ctrl-C twice exits.

The plan:

- editing files in place;
- diff against git `HEAD`: unified and side-by-side;
- an agent on top of Claude Code that uses your Claude subscription login, where every edit it proposes can be reviewed and changed before it lands.

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
